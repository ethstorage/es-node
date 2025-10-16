// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/leveldb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	ethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/archiver"
	"github.com/ethstorage/go-ethstorage/ethstorage/blobs"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
	"github.com/hashicorp/go-multierror"
)

type EsNode struct {
	lg         log.Logger
	appVersion string
	metrics    metrics.Metricer

	l1HeadsSub     ethereum.Subscription // Subscription to get L1 heads (automatically re-subscribes on error)
	l1SafeSub      ethereum.Subscription // Subscription to get L1 safe blocks, a.k.a. justified data (polling)
	l1FinalizedSub ethereum.Subscription // Subscription to get L1 Finalized blocks, a.k.a. justified data (polling)
	randaoHeadsSub ethereum.Subscription // Subscription to get randao heads (automatically re-subscribes on error)

	randaoSource *eth.RandaoClient      // RPC client to fetch randao from
	l1Source     *eth.PollingClient     // L1 Client to fetch data from
	l1Beacon     *eth.BeaconClient      // L1 Beacon Chain to fetch blobs from
	daClient     *eth.DAClient          // L1 Data Availability Client
	blobCache    downloader.BlobCache   // Cache for blobs
	downloader   *downloader.Downloader // L2 Engine to Sync
	// l2Source  *sources.EngineClient // L2 Execution Engine RPC bindings
	// rpcSync   *sources.SyncClient   // Alt-sync RPC client, optional (may be nil)
	server  *rpcServer   // RPC server hosting the rollup-node API
	p2pNode *p2p.NodeP2P // P2P node functionality
	// p2pSigner p2p.Signer            // p2p gogssip application messages will be signed with this signer
	// tracer    Tracer                // tracer to get events for testing/debugging
	// runCfg    *RuntimeConfig        // runtime configurables
	storageManager *ethstorage.StorageManager
	db             ethdb.Database

	// some resources cannot be stopped directly, like the p2p gossipsub router (not our design),
	// and depend on this ctx to be closed.
	resourcesCtx   context.Context
	resourcesClose context.CancelFunc
	miner          *miner.Miner
	// feed to notify miner of the sync done event to start mining
	feed *event.Feed
	// long term blob provider API for rollups
	archiverAPI *archiver.APIService
	// data integrity check on a regular basis
	scanner *scanner.Scanner
}

func New(ctx context.Context, cfg *Config, lg log.Logger, appVersion string, m metrics.Metricer) (*EsNode, error) {
	if err := cfg.Check(); err != nil {
		return nil, err
	}

	n := &EsNode{
		lg:         lg,
		appVersion: appVersion,
		metrics:    m,
	}
	// not a context leak, gossipsub is closed with a context.
	n.resourcesCtx, n.resourcesClose = context.WithCancel(context.Background())
	n.feed = new(event.Feed)
	err := n.init(ctx, cfg)
	if err != nil {
		n.lg.Error("Error initializing the rollup node", "err", err)
		// ensure we always close the node resources if we fail to initialize the node.
		if closeErr := n.Close(); closeErr != nil {
			return nil, multierror.Append(err, closeErr)
		}
		return nil, err
	}
	return n, nil
}

func (n *EsNode) init(ctx context.Context, cfg *Config) error {
	if err := n.initL1(ctx, cfg); err != nil {
		return err
	}
	if err := n.initDatabase(cfg); err != nil {
		return err
	}
	if err := n.initStorageManager(ctx, cfg); err != nil {
		return err
	}
	if err := n.initP2P(ctx, cfg); err != nil {
		return err
	}
	if err := n.initL2(ctx, cfg); err != nil {
		return err
	}
	if err := n.initMiner(ctx, cfg); err != nil {
		return err
	}

	// Only expose the server at the end, ensuring all RPC backend components are initialized.
	if err := n.initRPCServer(ctx, cfg); err != nil {
		return err
	}
	if err := n.initMetricsServer(ctx, cfg); err != nil {
		return err
	}
	if err := n.initArchiver(ctx, cfg); err != nil {
		return err
	}
	return nil
}

func (n *EsNode) initL2(ctx context.Context, cfg *Config) error {
	n.blobCache = downloader.NewBlobDiskCache(cfg.DataDir, n.lg)
	n.downloader = downloader.NewDownloader(
		n.l1Source,
		n.l1Beacon,
		n.daClient,
		n.db,
		n.storageManager,
		n.blobCache,
		cfg.Downloader.DownloadStart,
		cfg.Downloader.DownloadDump,
		cfg.L1.L1MinDurationForBlobsRequest,
		cfg.Downloader.DownloadThreadNum,
		n.lg,
	)
	return nil
}

func (n *EsNode) initL1(ctx context.Context, cfg *Config) error {
	client, err := eth.Dial(cfg.L1.L1NodeAddr, cfg.Storage.L1Contract, cfg.L1.L1BlockTime, n.lg)
	if err != nil {
		return fmt.Errorf("failed to create L1 source: %w", err)
	}
	n.l1Source = client

	if cfg.L1.DAURL != "" {
		n.daClient = eth.NewDAClient(cfg.L1.DAURL)
		n.lg.Info("Using DA URL", "url", cfg.L1.DAURL)
	} else if cfg.L1.L1BeaconURL != "" {
		n.l1Beacon, err = eth.NewBeaconClient(cfg.L1.L1BeaconURL, cfg.L1.L1BeaconSlotTime)
		if err != nil {
			return fmt.Errorf("failed to create L1 beacon source: %w", err)
		}
		n.lg.Info("Using L1 Beacon URL", "url", cfg.L1.L1BeaconURL)
	} else {
		return fmt.Errorf("no L1 beacon or DA URL provided")
	}
	if cfg.RandaoSourceURL != "" {
		rc, err := eth.DialRandaoSource(ctx, cfg.RandaoSourceURL, cfg.L1.L1NodeAddr, cfg.L1.L1BlockTime, n.lg)
		if err != nil {
			return fmt.Errorf("failed to create randao source: %w", err)
		}
		n.randaoSource = rc
		n.lg.Info("Using randao source", "url", cfg.RandaoSourceURL)
	}
	return nil
}

func (n *EsNode) startL1(cfg *Config) {
	// Keep subscribed to the L1 heads, which keeps the L1 maintainer pointing to the best headers to sync
	n.l1HeadsSub = event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			n.lg.Warn("Resubscribing after failed L1 subscription", "err", err)
		}
		return eth.WatchHeadChanges(n.resourcesCtx, n.l1Source, n.OnNewL1Head)
	})
	go func() {
		err, ok := <-n.l1HeadsSub.Err()
		if !ok {
			return
		}
		n.lg.Error("L1 heads subscription error", "err", err)
	}()

	// Keep subscribed to the randao heads, which helps miner to get proper random seeds
	n.randaoHeadsSub = event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			n.lg.Warn("Resubscribing after failed randao head subscription", "err", err)
		}
		if n.randaoSource != nil {
			return eth.WatchHeadChanges(n.resourcesCtx, n.randaoSource, n.OnNewRandaoSourceHead)
		} else {
			return eth.WatchHeadChanges(n.resourcesCtx, n.l1Source, n.OnNewRandaoSourceHead)
		}
	})
	go func() {
		err, ok := <-n.randaoHeadsSub.Err()
		if !ok {
			return
		}
		n.lg.Error("Randao heads subscription error", "err", err)
	}()

	// Poll for the safe L1 block and finalized block,
	// which only change once per epoch at most and may be delayed.
	n.l1SafeSub = eth.PollBlockChanges(n.resourcesCtx, n.lg, n.l1Source, n.OnNewL1Safe, ethRPC.SafeBlockNumber,
		cfg.L1EpochPollInterval, time.Second*10)
	n.l1FinalizedSub = eth.PollBlockChanges(n.resourcesCtx, n.lg, n.l1Source, n.OnNewL1Finalized, ethRPC.FinalizedBlockNumber,
		cfg.L1EpochPollInterval, time.Second*10)
}

func (n *EsNode) initP2P(ctx context.Context, cfg *Config) error {
	if cfg.P2P != nil {
		p2pNode, err := p2p.NewNodeP2P(n.resourcesCtx, cfg.ChainID, n.lg, cfg.P2P, n.storageManager, n.db, n.metrics, n.feed)
		if err != nil || p2pNode == nil {
			return err
		}
		n.p2pNode = p2pNode
		if n.p2pNode.Dv5Udp() != nil {
			go n.p2pNode.DiscoveryProcess(n.resourcesCtx, n.lg, cfg.ChainID.Uint64(), cfg.P2P.TargetPeers())
		}
	}
	return nil
}

func (n *EsNode) initStorageManager(ctx context.Context, cfg *Config) error {
	shardManager := ethstorage.NewShardManager(cfg.Storage.L1Contract, cfg.Storage.KvSize, cfg.Storage.KvEntriesPerShard, cfg.Storage.ChunkSize)
	for _, filename := range cfg.Storage.Filenames {
		var err error
		var df *ethstorage.DataFile
		df, err = ethstorage.OpenDataFile(filename)
		if err != nil {
			return fmt.Errorf("open failed: %w", err)
		}
		if df.Miner() != cfg.Storage.Miner {
			n.lg.Error("Miners mismatch", "fromDataFile", df.Miner(), "fromConfig", cfg.Storage.Miner)
			return fmt.Errorf("miner mismatches datafile")
		}
		shardManager.AddDataFileAndShard(df)
	}

	if shardManager.IsComplete() != nil {
		return fmt.Errorf("shard is not completed")
	}

	n.lg.Info("Initialized storage",
		"miner", cfg.Storage.Miner,
		"l1contract", cfg.Storage.L1Contract,
		"kvSize", shardManager.MaxKvSize(),
		"chunkSize", shardManager.ChunkSize(),
		"kvsPerShard", shardManager.KvEntries(),
		"shards", shardManager.ShardIds())

	n.storageManager = ethstorage.NewStorageManager(shardManager, n.l1Source, n.lg)
	return nil
}

func (n *EsNode) initRPCServer(ctx context.Context, cfg *Config) error {
	server, err := newRPCServer(ctx, &cfg.RPC, cfg.ChainID, n.storageManager, n.downloader, n.lg, n.appVersion)
	if err != nil {
		return err
	}
	n.lg.Info("Starting JSON-RPC server")
	if err := server.Start(); err != nil {
		return fmt.Errorf("unable to start RPC server: %w", err)
	}
	n.server = server
	return nil
}

func (n *EsNode) initMetricsServer(ctx context.Context, cfg *Config) error {
	if !cfg.Metrics.Enabled {
		n.lg.Info("Metrics disabled")
		return nil
	}
	n.lg.Info("Starting metrics server", "addr", cfg.Metrics.ListenAddr, "port", cfg.Metrics.ListenPort)
	go func() {
		if err := n.metrics.Serve(ctx, cfg.Metrics.ListenAddr, cfg.Metrics.ListenPort); err != nil {
			n.lg.Crit("Error starting metrics server", "err", err)
		}
	}()
	return nil
}

func (n *EsNode) initMiner(ctx context.Context, cfg *Config) error {
	if cfg.Mining == nil {
		// not enabled
		return nil
	}
	l1api := miner.NewL1MiningAPI(n.l1Source, n.randaoSource, n.lg)
	if err := l1api.CheckMinerRole(ctx, cfg.Storage.L1Contract, cfg.Storage.Miner); err != nil {
		n.lg.Crit("Check miner role", "err", err)
	}
	pvr := prover.NewKZGPoseidonProver(
		cfg.Mining.ZKWorkingDir,
		cfg.Mining.ZKeyFile,
		cfg.Mining.ZKProverMode,
		cfg.Mining.ZKProverImpl,
		n.lg,
	)
	br := blobs.NewBlobReader(n.blobCache, n.storageManager, n.lg)
	n.miner = miner.New(cfg.Mining, n.db, n.storageManager, l1api, br, &pvr, n.feed, n.lg)
	n.lg.Info("Initialized miner")
	return nil
}

func (n *EsNode) initArchiver(ctx context.Context, cfg *Config) error {
	if cfg.Archiver == nil {
		// not enabled
		return nil
	}
	n.archiverAPI = archiver.NewService(*cfg.Archiver, n.storageManager, n.l1Beacon, n.l1Source, n.lg)
	n.lg.Info("Initialized blob archiver API server")
	if err := n.archiverAPI.Start(ctx); err != nil {
		return fmt.Errorf("unable to start blob archiver API server: %w", err)
	}
	return nil
}

func (n *EsNode) initScanner(ctx context.Context, cfg *Config) {
	if cfg.Scanner == nil {
		// not enabled
		return
	}
	n.scanner = scanner.New(
		ctx,
		*cfg.Scanner,
		n.storageManager,
		n.p2pNode.FetchBlob,
		n.l1Source,
		n.feed,
		n.lg,
	)
}

func (n *EsNode) Start(ctx context.Context, cfg *Config) error {
	n.startL1(cfg)

	// miner must be started before p2p sync
	if n.miner != nil {
		n.miner.Start()
	}

	if err := n.downloader.Start(); err != nil {
		n.lg.Error("Could not start a downloader", "err", err)
		return err
	}
	// Scanner must be started after downloader resets storage manager
	n.initScanner(ctx, cfg)

	if n.p2pNode != nil {
		if err := n.p2pNode.Start(); err != nil {
			n.lg.Error("Could not start a p2pNode", "err", err)
			return err
		}
	}

	if cfg.StateUploadURL != "" {
		n.lg.Info("Start upload node state")
		go n.UploadNodeState(cfg.StateUploadURL)
	}

	return nil
}

func (n *EsNode) UploadNodeState(url string) {
	<-time.After(2 * time.Minute)
	localNode := n.p2pNode.Dv5Local().Node()
	id := localNode.ID().String()
	helloUrl := fmt.Sprintf("%s/hello", url)
	stateUrl := fmt.Sprintf("%s/reportstate", url)
	_, err := sendMessage(helloUrl, id)
	if err != nil {
		n.lg.Warn("Send message to resp", "err", err.Error())
		return
	}
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			state := NodeState{
				Id:              id,
				Contract:        n.storageManager.ContractAddress().Hex(),
				Version:         n.appVersion,
				Address:         fmt.Sprintf("%s:%d", localNode.IP().String(), localNode.TCP()),
				SavedBlobs:      n.storageManager.KvEntryCount(),
				DownloadedBlobs: n.downloader.GetState(),
				ScanStats:       n.scanner.GetScanState(),
			}

			var submissionStates map[uint64]*miner.SubmissionState
			if status, _ := n.db.Get(miner.SubmissionStatusKey); status != nil {
				if err := json.Unmarshal(status, &submissionStates); err != nil {
					n.lg.Error("Failed to decode submission states", "err", err)
					continue
				}
			}
			var miningStates map[uint64]*miner.MiningState
			if status, _ := n.db.Get(miner.MiningStatusKey); status != nil {
				if err := json.Unmarshal(status, &miningStates); err != nil {
					n.lg.Error("Failed to decode submission states", "err", err)
					continue
				}
			}
			var providedBlobs map[uint64]uint64
			if status, _ := n.db.Get(protocol.ProvidedBlobsKey); status != nil {
				if err := json.Unmarshal(status, &providedBlobs); err != nil {
					n.lg.Error("Failed to decode provided Blobs count", "err", err)
					continue
				}
			}
			var syncStates map[uint64]*protocol.SyncState
			if status, _ := n.db.Get(protocol.SyncStatusKey); status != nil {
				if err := json.Unmarshal(status, &syncStates); err != nil {
					n.lg.Error("Failed to decode sync states", "err", err)
					continue
				}
			}

			shards := make([]*ShardState, 0)
			for _, shardId := range n.storageManager.Shards() {
				miner, _ := n.storageManager.GetShardMiner(shardId)
				providedBlob, _ := providedBlobs[shardId]
				syncState, _ := syncStates[shardId]
				miningState, _ := miningStates[shardId]
				submissionState, _ := submissionStates[shardId]
				s := ShardState{
					ShardId:         shardId,
					Miner:           miner,
					ProvidedBlob:    providedBlob,
					SyncState:       syncState,
					MiningState:     miningState,
					SubmissionState: submissionState,
				}
				shards = append(shards, &s)
			}
			state.Shards = shards

			data, err := json.Marshal(state)
			if err != nil {
				n.lg.Info("Fail to Marshal node state", "error", err.Error())
				continue
			}
			_, err = sendMessage(stateUrl, string(data))
			if err != nil {
				n.lg.Info("Fail to upload node state", "error", err.Error())
			}
		case <-n.resourcesCtx.Done():
			return
		}
	}
}

func (n *EsNode) OnNewL1Head(ctx context.Context, sig eth.L1BlockRef) {
	n.lg.Debug("OnNewL1Head", "blockNumber", sig.Number)
	if n.downloader != nil {
		n.downloader.OnNewL1Head(sig)
	}
}

func (n *EsNode) OnNewRandaoSourceHead(ctx context.Context, sig eth.L1BlockRef) {
	n.lg.Debug("OnNewRandaoSourceHead", "blockNumber", sig.Number)
	if n.miner != nil {
		select {
		case n.miner.ChainHeadCh <- sig:
		default:
			// Channel is full, skipping
		}
	}
}

func (n *EsNode) OnNewL1Safe(ctx context.Context, sig eth.L1BlockRef) {
	n.lg.Debug("OnNewL1Safe", "blockNumber", sig.Number)
}

func (n *EsNode) OnNewL1Finalized(ctx context.Context, sig eth.L1BlockRef) {
	n.lg.Debug("OnNewL1Finalized", "blockNumber", sig.Number)
	if n.downloader != nil {
		n.downloader.OnL1Finalized(sig.Number)
	}
}

func (n *EsNode) RequestL2Range(ctx context.Context, start, end uint64) (uint64, error) {
	if n.p2pNode != nil {
		return n.p2pNode.RequestL2Range(ctx, start, end)
	}
	n.lg.Debug("Ignoring request to sync L2 range, no sync method available", "start", start, "end", end)
	return 0, nil
}

func (n *EsNode) Close() error {
	var result *multierror.Error

	if n.server != nil {
		n.server.Stop()
	}
	if n.p2pNode != nil {
		if err := n.p2pNode.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close p2p node: %w", err))
		}
	}
	if n.downloader != nil {
		if err := n.downloader.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close downloader: %w", err))
		}
	}
	// if n.p2pSigner != nil {
	// 	if err := n.p2pSigner.Close(); err != nil {
	// 		result = multierror.Append(result, fmt.Errorf("failed to close p2p signer: %w", err))
	// 	}
	// }

	if n.resourcesClose != nil {
		n.resourcesClose()
	}

	// stop L1 heads feed
	if n.l1HeadsSub != nil {
		n.l1HeadsSub.Unsubscribe()
	}
	if n.randaoHeadsSub != nil {
		n.randaoHeadsSub.Unsubscribe()
	}
	if n.miner != nil {
		n.miner.Close()
	}
	if n.blobCache != nil {
		if err := n.blobCache.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close blob cache: %w", err))
		}
	}

	if n.archiverAPI != nil {
		n.archiverAPI.Stop(context.Background())
	}

	if n.scanner != nil {
		n.scanner.Close()
	}
	// close L2 driver
	// if n.l2Driver != nil {
	// 	if err := n.l2Driver.Close(); err != nil {
	// 		result = multierror.Append(result, fmt.Errorf("failed to close L2 engine driver cleanly: %w", err))
	// 	}

	// 	// If the L2 sync client is present & running, close it.
	// 	if n.rpcSync != nil {
	// 		if err := n.rpcSync.Close(); err != nil {
	// 			result = multierror.Append(result, fmt.Errorf("failed to close L2 engine backup sync client cleanly: %w", err))
	// 		}
	// 	}
	// }

	// // close L2 engine RPC client
	// if n.l2Source != nil {
	// 	n.l2Source.Close()
	// }

	// close L1 data source
	if n.l1Source != nil {
		n.l1Source.Close()
	}
	if n.randaoSource != nil {
		n.randaoSource.Close()
	}
	if n.storageManager != nil {
		n.storageManager.Close()
	}
	return result.ErrorOrNil()
}

func (n *EsNode) initDatabase(cfg *Config) error {
	var db ethdb.Database
	if cfg.DataDir == "" {
		db = rawdb.NewMemoryDatabase()
	} else {
		rdb, err := leveldb.New(
			cfg.ResolvePath(cfg.DBConfig.Name),
			cfg.DBConfig.DatabaseCache,
			cfg.DBConfig.DatabaseHandles,
			cfg.DBConfig.NameSpace,
			false,
		)
		if err != nil {
			return err
		}
		db = rawdb.NewDatabase(rdb)
	}
	n.db = db
	return nil
}

func sendMessage(url string, data string) (string, error) {
	contentType := "application/json"
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
