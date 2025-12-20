// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package downloader

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

const (
	TrackLatest    = iota // 0
	TrackSafe             // 1
	TrackFinalized        // 2

	downloadBatchSize = 64 // 2 epoch
)

var (
	downloaderPrefix   = []byte("dl-")
	downloadedBlobsKey = []byte("downloaded-blobs")
	lastDownloadKey    = []byte("last-download-block")
)

type BlobCache interface {
	SetBlockBlobs(block *blockBlobs) error
	Blobs(number uint64) []blob
	GetKeyValueByIndex(idx uint64, hash common.Hash) []byte
	GetSampleData(idx uint64, sampleIdx uint64) []byte
	Cleanup(finalized uint64)
	Close() error
}

type Downloader struct {
	Cache BlobCache

	// latestHead and finalizedHead are shared among multiple threads and thus locks must be required when being accessed
	// others are only accessed by the downloader thread so it is safe to access them in DL thread without locks
	l1Source                   *eth.PollingClient
	l1Beacon                   *eth.BeaconClient
	daClient                   *eth.DAClient
	db                         ethdb.Database
	sm                         *ethstorage.StorageManager
	lastDownloadBlock          int64
	lastCacheBlock             int64
	finalizedHead              int64
	latestHead                 int64
	dumpDir                    string
	minDurationForBlobsRequest uint64
	downloadedBlobs            uint64

	// Request to download new blobs
	dlLatestReq    chan struct{}
	dlFinalizedReq chan struct{}

	emailConfig *email.EmailConfig
	lg          log.Logger
	done        chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
}

type blob struct {
	kvIndex *big.Int
	kvSize  *big.Int
	hash    common.Hash
	data    []byte
	dataId  uint64
}

func (b *blob) String() string {
	return fmt.Sprintf("blob{kvIndex: %d, hash: %x, data: %s}", b.kvIndex, b.hash, b.data)
}

type blockBlobs struct {
	timestamp uint64
	number    uint64
	blobs     []*blob
}

func (b *blockBlobs) String() string {
	return fmt.Sprintf("blockBlobs{number: %d, timestamp: %d, blobs: %d}", b.number, b.timestamp, len(b.blobs))
}

func NewDownloader(
	l1Source *eth.PollingClient,
	l1Beacon *eth.BeaconClient,
	daClient *eth.DAClient,
	db ethdb.Database,
	sm *ethstorage.StorageManager,
	cache BlobCache,
	minDurationForBlobsRequest uint64,
	downloadConfig Config,
	lg log.Logger,
) *Downloader {
	sm.DownloadThreadNum = downloadConfig.DownloadThreadNum
	return &Downloader{
		Cache:                      cache,
		l1Source:                   l1Source,
		l1Beacon:                   l1Beacon,
		daClient:                   daClient,
		db:                         db,
		sm:                         sm,
		dumpDir:                    downloadConfig.DownloadDump,
		minDurationForBlobsRequest: minDurationForBlobsRequest,
		dlLatestReq:                make(chan struct{}, 1),
		dlFinalizedReq:             make(chan struct{}, 1),
		lg:                         lg,
		done:                       make(chan struct{}),
		lastDownloadBlock:          downloadConfig.DownloadStart,
		downloadedBlobs:            0,
		emailConfig:                downloadConfig.EmailConfig,
	}
}

// Start starts up the state loop.
func (s *Downloader) Start() error {
	// user does NOT specify a download start in the flag
	if s.lastDownloadBlock == 0 {
		bs, err := s.db.Get(append(downloaderPrefix, lastDownloadKey...))
		if err != nil {
			// first-time start
			header, err := s.l1Source.HeaderByNumber(context.Background(), big.NewInt(rpc.FinalizedBlockNumber.Int64()))
			if err != nil {
				return err
			} else {
				s.lastDownloadBlock = header.Number.Int64()
				s.lg.Info("Downloader will use the latest finalized block to start for the first time", "block", s.lastDownloadBlock)
			}
		} else {
			s.lastDownloadBlock = int64(binary.LittleEndian.Uint64(bs))
			s.lg.Info("Downloader will use the last download block to start", "block", s.lastDownloadBlock)
		}
	} else if s.lastDownloadBlock < 0 {
		if s.lastDownloadBlock == rpc.FinalizedBlockNumber.Int64() {
			header, err := s.l1Source.HeaderByNumber(context.Background(), big.NewInt(s.lastDownloadBlock))
			if err != nil {
				return err
			} else {
				s.lastDownloadBlock = header.Number.Int64()
			}
		} else {
			return fmt.Errorf("please use a positive number or latest finalized block (-3) as the download start point")
		}
	}

	bs, err := s.db.Get(append(downloaderPrefix, downloadedBlobsKey...))
	if err == nil && len(bs) == 8 {
		s.downloadedBlobs = binary.LittleEndian.Uint64(bs)
	}

	err = s.sm.Reset(s.lastDownloadBlock)
	if err != nil {
		return err
	}

	s.wg.Add(1)
	go s.eventLoop()
	return nil
}

func (s *Downloader) Close() error {
	s.done <- struct{}{}
	s.wg.Wait()
	return nil
}

func (s *Downloader) OnL1Finalized(finalized uint64) {
	s.mu.Lock()
	if s.finalizedHead > int64(finalized) {
		s.lg.Warn("The tracking head is greater than new finalized", "tracking", s.finalizedHead, "new", finalized)
	}
	s.finalizedHead = int64(finalized)
	s.mu.Unlock()

	select {
	case s.dlFinalizedReq <- struct{}{}:
		return
	default:
		// if there is already a download request in the channel, then do nothing
		return
	}
}

func (s *Downloader) OnNewL1Head(head eth.L1BlockRef) {
	s.mu.Lock()
	if s.latestHead > int64(head.Number) {
		s.lg.Info("The tracking head is greater than new one, a reorg may happen", "tracking", s.latestHead, "new", head)
	}
	s.latestHead = int64(head.Number)
	s.mu.Unlock()

	select {
	case s.dlLatestReq <- struct{}{}:
		return
	default:
		// if there is already a download request in the channel, then do nothing
		return
	}
}

func (s *Downloader) GetState() uint64 {
	return s.downloadedBlobs
}

func (s *Downloader) eventLoop() {
	defer s.wg.Done()
	s.lg.Info("Download loop started")

	for {
		select {
		case <-s.dlFinalizedReq:
			s.download()
		case <-s.dlLatestReq:
			s.downloadToCache()
		case <-s.done:
			return
		}
	}
}

func (s *Downloader) downloadToCache() {
	s.mu.Lock()
	if s.finalizedHead == 0 {
		// we need the finalized head to trigger the first cache download
		s.mu.Unlock()
		return
	}
	end := s.latestHead
	start := s.lastCacheBlock
	if start == 0 {
		start = s.finalizedHead
	}
	s.mu.Unlock()

	for start < end {
		rangeEnd := min(start+downloadBatchSize, end)
		_, err := s.downloadRange(start+1, rangeEnd, true)

		if err != nil {
			s.lg.Error("DownloadRange failed", "err", err)
			return
		}

		s.lastCacheBlock = rangeEnd
		start = rangeEnd
	}
}

func (s *Downloader) download() {
	s.mu.Lock()
	trackHead := s.finalizedHead
	s.mu.Unlock()

	if (s.lastDownloadBlock > 0) && (trackHead-s.lastDownloadBlock > int64(s.minDurationForBlobsRequest)) {
		// TODO: @Qiang we can also enter into an recovery mode (e.g., scan local blobs to obtain a heal list, more complicated, will do later)
		prompt := "Ethereum only keep blobs for one month, but it has been over one month since last blob download." +
			"You may need to restart this node with full re-sync"
		s.lg.Error(prompt)
		return
	}

	for s.lastDownloadBlock < trackHead {
		start := s.lastDownloadBlock + 1
		end := min(s.lastDownloadBlock+downloadBatchSize, trackHead)
		// If downloadRange fails, then lastDownloadedBlock will keep the same as before. so when the next
		// upload task starts, it will still try to download the blobs from the last failed block number
		if blobs, err := s.downloadRange(start, end, false); err == nil {
			// need to prepare kvIndices, dataBlobs, metas for the downloadFinished
			// note that there will be parallel key-value writes in downloadFinished
			// so we need to make sure that the kvIndices are unique, otherwise, the one that is written later will overwrite the previous one
			// we will use a map named 'kvIdxToArrayIdx' to store the kvIndex to array index mapping
			kvIndices := make([]uint64, 0)
			dataBlobs := make([][]byte, 0)
			metas := make([]common.Hash, 0)

			kvIdxToArrayIdx := make(map[uint64]int)

			for _, blob := range blobs {
				s.lg.Info("Blob will be saved into disk", "kvIndex", blob.kvIndex.Uint64(), "hash", hex.EncodeToString(blob.hash[:]))
				if arrIdx, exists := kvIdxToArrayIdx[blob.kvIndex.Uint64()]; exists {
					s.lg.Info("Duplicate kvIndex found, will replace the previous one", "previous", kvIndices[arrIdx], "new", blob.kvIndex.Uint64())
					dataBlobs[arrIdx] = blob.data
					copy(metas[arrIdx][0:ethstorage.HashSizeInContract], blob.hash[0:ethstorage.HashSizeInContract])
				} else {
					kvIndices = append(kvIndices, blob.kvIndex.Uint64())
					kvIdxToArrayIdx[blob.kvIndex.Uint64()] = len(kvIndices) - 1

					dataBlobs = append(dataBlobs, blob.data)
					meta := common.Hash{}
					copy(meta[0:ethstorage.HashSizeInContract], blob.hash[0:ethstorage.HashSizeInContract])
					metas = append(metas, meta)
				}
			}

			ts := time.Now()
			err := s.sm.DownloadFinished(end, kvIndices, dataBlobs, metas)
			if err != nil {
				s.lg.Error("Save blobs error", "err", err)
				return
			}
			if len(blobs) > 0 {
				s.downloadedBlobs += uint64(len(blobs))
				s.lg.Info("DownloadFinished", "duration(ms)", time.Since(ts).Milliseconds(), "blobs", len(blobs))
			}

			// save lastDownloadedBlock into database
			bs := make([]byte, 8)
			binary.LittleEndian.PutUint64(bs, uint64(end))

			err = s.db.Put(append(downloaderPrefix, lastDownloadKey...), bs)
			if err != nil {
				s.lg.Error("Save lastDownloadedBlock into db error", "err", err)
				return
			}
			s.lg.Debug("LastDownloadedBlock saved into db", "lastDownloadedBlock", end)

			binary.LittleEndian.PutUint64(bs, s.downloadedBlobs)
			err = s.db.Put(append(downloaderPrefix, downloadedBlobsKey...), bs)
			if err != nil {
				s.lg.Error("Save downloadedBlobs into db error", "err", err)
				return
			}

			s.dumpBlobsIfNeeded(blobs)

			s.lastDownloadBlock = end
		}
	}

	// clear the cache
	s.Cache.Cleanup(uint64(trackHead))
}

// The entire downloading process consists of two phases:
// 1. Downloading the blobs into the cache when they are not finalized, with the option toCache set to true.
// 2. Writing the blobs into the shard file when they are finalized, with the option toCache set to false.
// we will attempt to read the blobs from the cache initially. If they don't exist in the cache, we will download them instead.
func (s *Downloader) downloadRange(start int64, end int64, toCache bool) ([]blob, error) {
	ts := time.Now()

	if end < start {
		end = start
	}

	events, err := s.l1Source.FilterLogsByBlockRange(big.NewInt(int64(start)), big.NewInt(int64(end)), eth.PutBlobEvent)
	if err != nil {
		return nil, err
	}
	elBlocks, err := s.eventsToBlocks(events)
	if err != nil {
		return nil, err
	}
	blobs := []blob{}
	for _, elBlock := range elBlocks {
		// attempt to read the blobs from the cache first
		res := s.Cache.Blobs(elBlock.number)
		if res != nil {
			blobs = append(blobs, res...)
			s.lg.Info("Blob found in the cache, continue to the next block", "blockNumber", elBlock.number)
			continue
		} else {
			s.lg.Info(
				"Don't find blob in the cache, will try to download directly",
				"blockNumber", elBlock.number,
				"start", start,
				"end", end,
				"toCache", toCache,
			)
		}

		clBlobs, err := s.downloadBlobsWithRetry(elBlock, 3)
		if err != nil {
			s.lg.Error("Failed to download blobs for the block after 3 attempts", "block", elBlock.number, "err", err)
			// Empty CL blob will be handled later in the EL blob loop
		}

		for _, elBlob := range elBlock.blobs {
			shard := elBlob.kvIndex.Uint64() >> s.sm.KvEntriesBits()
			if !slices.Contains(s.sm.Shards(), shard) {
				s.lg.Warn("Shard not initialized locally for the kvIndex, skip this blob", "kvIndex", elBlob.kvIndex.Uint64(), "shard", shard)
				continue
			}
			clBlob, exists := clBlobs[elBlob.hash]
			if !exists {
				s.notifyBlobMissing(elBlock.number, elBlob.kvIndex.Uint64(), elBlob.hash)
				s.lg.Crit("Did not find the event specified blob in the CL", "blockNumber", elBlock.number, "kvIndex", elBlob.kvIndex)
			}
			// encode blobs so that miner can do sampling directly from cache
			elBlob.data = s.sm.EncodeBlob(clBlob.Data, elBlob.hash, elBlob.kvIndex.Uint64(), s.sm.MaxKvSize())
			blobs = append(blobs, *elBlob)
			s.lg.Info("Downloaded and encoded", "blockNumber", elBlock.number, "kvIdx", elBlob.kvIndex)
		}
		if toCache {
			if err := s.Cache.SetBlockBlobs(elBlock); err != nil {
				s.lg.Error("Failed to cache blobs", "block", elBlock.number, "err", err)
				return nil, err
			}
		}
	}

	if len(blobs) > 0 {
		s.lg.Info("Download range", "cache", toCache, "start", start, "end", end, "blobNumber", len(blobs), "duration(ms)", time.Since(ts).Milliseconds())
	}

	return blobs, nil
}

func (s *Downloader) downloadBlobsWithRetry(elBlock *blockBlobs, maxAttempts int) (map[common.Hash]eth.Blob, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		clBlobs, err := s.downloadBlobs(elBlock)
		if err == nil {
			return clBlobs, nil
		}
		lastErr = err
		if attempt < maxAttempts {
			time.Sleep(3 * time.Second)
		}
	}
	return nil, lastErr
}

func (s *Downloader) downloadBlobs(elBlock *blockBlobs) (map[common.Hash]eth.Blob, error) {
	if s.l1Beacon != nil {
		slot := s.l1Beacon.Timestamp2Slot(elBlock.timestamp)
		clBlobs, err := s.l1Beacon.DownloadBlobs(slot)
		if err != nil {
			s.lg.Error("L1 beacon download blob error", "block", elBlock.number, "slot", slot, "err", err)
			return nil, err
		}
		return clBlobs, nil
	}

	if s.daClient != nil {
		hashes := make([]common.Hash, 0, len(elBlock.blobs))
		for _, b := range elBlock.blobs {
			hashes = append(hashes, b.hash)
		}

		clBlobs, err := s.daClient.DownloadBlobs(hashes)
		if err != nil {
			s.lg.Error("DA client download blob error", "err", err)
			return nil, err
		}
		return clBlobs, nil
	}

	return nil, fmt.Errorf("no beacon client or DA client is available")
}

func (s *Downloader) dumpBlobsIfNeeded(blobs []blob) {
	if s.dumpDir != "" {
		for _, blob := range blobs {
			fileName := filepath.Join(s.dumpDir, fmt.Sprintf("%s.dat", hex.EncodeToString(blob.data[:5])))
			f, err := os.Create(fileName)
			if err != nil {
				s.lg.Warn("Error creating file", "filename", fileName, "err", err)
				return
			}
			defer f.Close()

			writer := bufio.NewWriter(f)
			writer.WriteString(string(blob.data))
			writer.Flush()
		}
	}
}

func (s *Downloader) eventsToBlocks(events []types.Log) ([]*blockBlobs, error) {
	blocks := []*blockBlobs{}
	lastBlockNumber := uint64(0)

	for _, event := range events {
		if lastBlockNumber != event.BlockNumber {
			res, err := s.l1Source.HeaderByNumber(context.Background(), big.NewInt(int64(event.BlockNumber)))
			if err != nil {
				return nil, err
			}
			lastBlockNumber = event.BlockNumber
			blocks = append(blocks, &blockBlobs{
				timestamp: res.Time,
				number:    event.BlockNumber,
				blobs:     []*blob{},
			})
		}

		block := blocks[len(blocks)-1]
		hash := common.Hash{}
		copy(hash[:], event.Topics[3][:])

		blob := blob{
			kvIndex: big.NewInt(0).SetBytes(event.Topics[1][:]),
			kvSize:  big.NewInt(0).SetBytes(event.Topics[2][:]),
			hash:    hash,
		}
		block.blobs = append(block.blobs, &blob)
	}

	return blocks, nil
}

func (s *Downloader) notifyBlobMissing(blockNumber uint64, kvIndex uint64, hash common.Hash) {
	if s.emailConfig == nil {
		return
	}

	msg := "The downloader couldn't locate the specified blob in the consensus layer. The node is stopped pending resolution. "
	msg += "Details from the EL event: \n"
	msg += fmt.Sprintf(" - blockNumber: %d\n", blockNumber)
	msg += fmt.Sprintf(" - kvIndex: %d\n", kvIndex)
	msg += fmt.Sprintf(" - hash: %s\n", hash.Hex())
	msg += "This may indicate a potential issue with blob availability on the consensus layer. \n"
	email.SendEmail(
		"🛑 Fatal Error from es-node: Downloader Failed to Locate Blob in CL",
		msg,
		*s.emailConfig,
		s.lg,
	)
}
