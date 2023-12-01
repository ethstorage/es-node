// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethstorage/go-ethstorage/ethstorage/encoder"
	"github.com/ethstorage/go-ethstorage/ethstorage/pora"
	"github.com/protolambda/go-kzg/eth"
)

// A DataShard is a logical shard that manages multiple DataFiles.
// It also manages the encoding/decoding, tranlation from KV read/write to chunk read/write,
// and sanity check of the data files.
type DataShard struct {
	shardIdx     uint64
	kvSize      uint64
	chunksPerKv  uint64
	kvEntries   uint64
	dataFiles    []*DataFile
	chunkSize       uint64
}

func NewDataShard(shardIdx uint64, kvSize uint64, kvEntries uint64, chunkSize uint64) *DataShard {
	if kvSize%chunkSize != 0 {
		panic("kvSize must be CHUNK_SIZE at the moment")
	}

	return &DataShard{shardIdx: shardIdx, kvSize: kvSize, chunksPerKv: kvSize / chunkSize, kvEntries: kvEntries, chunkSize: chunkSize}
}

func (ds *DataShard) AddDataFile(df *DataFile) error {
	if len(ds.dataFiles) != 0 {
		// Perform sanity check
		if ds.dataFiles[0].miner != df.miner {
			return fmt.Errorf("mismatched data file SP")
		}
		if ds.dataFiles[0].encodeType != df.encodeType {
			return fmt.Errorf("mismatched data file encode type")
		}
		if ds.dataFiles[0].maxKvSize != df.maxKvSize {
			return fmt.Errorf("mismatched data file max kv size")
		}
		// TODO: May check if not overlapped?
	}
	ds.dataFiles = append(ds.dataFiles, df)
	return nil
}

// Returns whether the shard has all data files to cover all entries
func (ds *DataShard) IsComplete() bool {
	chunkIdx := ds.StartChunkIdx()
	chunkIdxEnd := (ds.shardIdx + 1) * ds.chunksPerKv * ds.kvEntries
	for chunkIdx < chunkIdxEnd {
		found := false
		for _, df := range ds.dataFiles {
			if df.Contains(chunkIdx) {
				chunkIdx = df.ChunkIdxEnd()
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Return the storage provider address (i.e., miner) of the shard.
func (ds *DataShard) Miner() common.Address {
	if len(ds.dataFiles) == 0 {
		return common.Address{}
	} else {
		return ds.dataFiles[0].miner
	}
}

func (ds *DataShard) EncodeType() uint64 {
	if len(ds.dataFiles) == 0 {
		return NO_ENCODE
	} else {
		return ds.dataFiles[0].encodeType
	}
}

func (ds *DataShard) Contains(kvIdx uint64) bool {
	return kvIdx >= ds.shardIdx*ds.kvEntries && kvIdx < (ds.shardIdx+1)*ds.kvEntries
}

func (ds *DataShard) ContainsSample(sampleIdx uint64) bool {
	return ds.Contains(sampleIdx * 32 / ds.kvSize)
}

func (ds *DataShard) StartChunkIdx() uint64 {
	return ds.shardIdx * ds.chunksPerKv * ds.kvEntries
}

func (ds *DataShard) GetStorageFile(chunkIdx uint64) *DataFile {
	for _, df := range ds.dataFiles {
		if df.Contains(chunkIdx) {
			return df
		}
	}
	return nil
}

// ReadChunkEncoded read the encoded data from storage and return it.
func (ds *DataShard) ReadChunkEncoded(kvIdx uint64, chunkIdx uint64) ([]byte, error) {
	return ds.readChunkWith(kvIdx, chunkIdx, func(cdata []byte, chunkIdx uint64) []byte {
		return cdata
	})
}

// ReadChunk read the encoded data from storage and decode it.
func (ds *DataShard) ReadChunk(kvIdx uint64, chunkIdx uint64, commit common.Hash) ([]byte, error) {
	return ds.readChunkWith(kvIdx, chunkIdx, func(cdata []byte, chunkIdx uint64) []byte {
		encodeKey := calcEncodeKey(commit, chunkIdx, ds.dataFiles[0].miner)
		return decodeChunk(ds.chunkSize, cdata, ds.dataFiles[0].encodeType, encodeKey)
	})
}

// readChunkWith read the encoded chunk from storage with a decoder.
func (ds *DataShard) readChunkWith(kvIdx uint64, chunkIdx uint64, decoder func([]byte, uint64) []byte) ([]byte, error) {
	if !ds.Contains(kvIdx) {
		return nil, fmt.Errorf("kv not found")
	}
	if chunkIdx >= ds.chunksPerKv {
		return nil, fmt.Errorf("chunkIdx out of range, chunkIdx： %d vs chunksPerKv %d", chunkIdx, ds.chunksPerKv)
	}
	idx := kvIdx*ds.chunksPerKv + chunkIdx
	data, err := ds.readChunk(idx, int(ds.chunkSize))
	if err != nil {
		return nil, err
	}
	data = decoder(data, idx)
	return data, nil
}

// ReadEncoded read the encoded data from storage and return it.
func (ds *DataShard) ReadEncoded(kvIdx uint64, readLen int) ([]byte, error) {
	return ds.readWith(kvIdx, readLen, func(cdata []byte, chunkIdx uint64) []byte {
		return cdata
	})
}

// Read the encoded data from storage and decode it.
func (ds *DataShard) Read(kvIdx uint64, readLen int, commit common.Hash) ([]byte, error) {
	bs, err := ds.readWith(kvIdx, int(ds.kvSize), func(cdata []byte, chunkIdx uint64) []byte {
		encodeKey := calcEncodeKey(commit, chunkIdx, ds.dataFiles[0].miner)
		return decodeChunk(ds.chunkSize, cdata, ds.dataFiles[0].encodeType, encodeKey)
	})
	if err != nil {
		return nil, err
	}

	if err = checkCommit(commit, bs); err != nil {
		return nil, err
	}
	return bs[0:readLen], nil
}

// Read the encoded data from storage and decode it.
func (ds *DataShard) ReadWithMeta(kvIdx uint64, readLen int) ([]byte, []byte, error) {
	commit, err := ds.ReadMeta(kvIdx)
	if err != nil {
		return nil, nil, err
	}
	bs, err := ds.readWith(kvIdx, int(ds.kvSize), func(cdata []byte, chunkIdx uint64) []byte {
		encodeKey := calcEncodeKey(common.BytesToHash(commit), chunkIdx, ds.dataFiles[0].miner)
		return decodeChunk(ds.chunkSize, cdata, ds.dataFiles[0].encodeType, encodeKey)
	})
	if err != nil {
		return nil, nil, err
	}

	if err = checkCommit(common.BytesToHash(commit), bs); err != nil {
		return nil, nil, err
	}
	return bs[0:readLen], commit, nil
}

// readWith read the encoded data from storage with a decoder.
func (ds *DataShard) readWith(kvIdx uint64, readLen int, decoder func([]byte, uint64) []byte) ([]byte, error) {
	if !ds.Contains(kvIdx) {
		return nil, fmt.Errorf("kv not found")
	}
	if readLen > int(ds.kvSize) {
		return nil, fmt.Errorf("read len too large")
	}
	var data []byte
	for i := uint64(0); i < ds.chunksPerKv; i++ {
		if readLen == 0 {
			break
		}

		chunkReadLen := readLen
		if chunkReadLen > int(ds.chunkSize) {
			chunkReadLen = int(ds.chunkSize)
		}
		readLen = readLen - chunkReadLen

		chunkIdx := kvIdx*ds.chunksPerKv + i
		cdata, err := ds.readChunk(chunkIdx, chunkReadLen)
		if err != nil {
			return nil, err
		}

		cdata = decoder(cdata, chunkIdx)
		data = append(data, cdata...)
	}
	return data, nil
}

func (ds *DataShard) ReadSample(sampleIdx uint64) (common.Hash, error) {

	for _, df := range ds.dataFiles {
		if df.ContainsSample(sampleIdx) {
			return df.ReadSample(sampleIdx)
		}
	}
	return common.Hash{}, fmt.Errorf("chunk not found: the shard is not completed?")
}

func CalcEncodeKey(commit common.Hash, chunkIdx uint64, miner common.Address) common.Hash {
	return calcEncodeKey(commit, chunkIdx, miner)
}

// Obtain a unique encoding key with keccak256(chunkIdx || commit || miner).
// This will make sure the encoded data will be unique in terms of idx, storage provider, and data
func calcEncodeKey(commit common.Hash, chunkIdx uint64, miner common.Address) common.Hash {
	// keep the datahash and remove all the other metas
	c := common.Hash{}
	copy(c[0:HashSizeInContract], commit[0:HashSizeInContract])

	bb := make([]byte, 0)
	bb = append(bb, c.Bytes()...)
	bb = append(bb, common.BytesToHash(miner.Bytes()).Bytes()...)
	bb = append(bb, common.BigToHash(big.NewInt(int64(chunkIdx))).Bytes()...)

	return crypto.Keccak256Hash(bb)
}

// EncodeChunks encodes bs and returned a chunk-sized encoded one
func EncodeChunk(chunkSize uint64, bs []byte, encodeType uint64, encodeKey common.Hash) []byte {
	return encodeChunk(chunkSize, bs, encodeType, encodeKey)
}

func encodeChunk(chunkSize uint64, bs []byte, encodeType uint64, encodeKey common.Hash) []byte {
	if len(bs) > int(chunkSize) {
		panic("cannot encode chunk with size > CHUNK_SIZE")
	}
	if encodeType == ENCODE_KECCAK_256 {
		output := make([]byte, chunkSize)
		j := 0
		for i := 0; i < int(chunkSize); i++ {
			b := byte(0)
			if i < len(bs) {
				b = bs[i]
			}
			output[i] = b ^ encodeKey[j]
			j = j + 1
			if j >= len(encodeKey) {
				j = 0
			}
		}
		return output
	} else if encodeType == NO_ENCODE {
		output := make([]byte, chunkSize)
		copy(output, bs)
		return output
	} else if encodeType == ENCODE_BLOB_POSEIDON {
		mask, _ := encoder.Encode(encodeKey, int(chunkSize))
		return MaskDataInPlace(mask, bs)
	} else if encodeType == ENCODE_ETHASH {
		return MaskDataInPlace(pora.GetMaskData(0, encodeKey, int(chunkSize), nil), bs)
	} else {
		panic("unsupported encode type")
	}
}

func IsValidEncodeType(encodeType uint64) bool {
	switch encodeType {
	case ENCODE_KECCAK_256:
		return true
	case NO_ENCODE:
		return true
	case ENCODE_ETHASH:
		return true
	default:
		return false
	}
}

// DecodeChunk decodes bs and returns a new decoded bytes
func DecodeChunk(chunkSize uint64, bs []byte, encodeType uint64, encodeKey common.Hash) []byte {
	return decodeChunk(chunkSize, bs, encodeType, encodeKey)
}

func decodeChunk(chunkSize uint64, bs []byte, encodeType uint64, encodeKey common.Hash) []byte {
	if len(bs) > int(chunkSize) {
		panic("cannot encode chunk with size > CHUNK_SIZE")
	}
	if encodeType == ENCODE_KECCAK_256 {
		output := make([]byte, len(bs))
		j := 0
		for i := 0; i < len(bs); i++ {
			output[i] = bs[i] ^ encodeKey[j]
			j = j + 1
			if j >= len(encodeKey) {
				j = 0
			}
		}
		return output
	} else if encodeType == NO_ENCODE {
		output := make([]byte, len(bs))
		copy(output, bs)
		return output
	} else if encodeType == ENCODE_BLOB_POSEIDON {
		mask, _ := encoder.Encode(encodeKey, int(chunkSize))
		return UnmaskDataInPlace(bs, mask)
	} else if encodeType == ENCODE_ETHASH {
		return UnmaskDataInPlace(bs, pora.GetMaskData(0, encodeKey, len(bs), nil))
	} else {
		panic("unsupported encode type")
	}
}

var EmptyBlobCommit = make([]byte, HashSizeInContract)

func checkCommit(commit common.Hash, blobData []byte) error {
	// commit is empty 0x00..00
	if bytes.Equal(commit[0:HashSizeInContract], EmptyBlobCommit) {
		// blob is empty 0x00...00
		for _, b := range blobData {
			if b != 0 {
				return fmt.Errorf("commit does not match")
			}
		}
		return nil
	}

	// kzg blob
	blob := kzg4844.Blob{}
	copy(blob[:], blobData)
	// Generate VersionedHash
	commitment, err := kzg4844.BlobToCommitment(blob)
	if err != nil {
		return fmt.Errorf("could not convert blob to commitment: %v", err)
	}
	versionedHash := eth.KZGToVersionedHash(eth.KZGCommitment(commitment))
	// Get the hash and only take 24 bits
	if !bytes.Equal(versionedHash[0:HashSizeInContract], commit[0:HashSizeInContract]) {
		return fmt.Errorf("commit does not match")
	}
	return nil
}

// Write a value of the KV to the store using a customized encoder.
func (ds *DataShard) WriteWith(kvIdx uint64, b []byte, commit common.Hash, encoder func([]byte, uint64) []byte) error {
	if !ds.Contains(kvIdx) {
		return fmt.Errorf("kv not found")
	}

	if uint64(len(b)) > ds.kvSize {
		return fmt.Errorf("write data too large")
	}
	cb := make([]byte, ds.kvSize)
	copy(cb, b)
	for i := uint64(0); i < ds.chunksPerKv; i++ {
		chunkIdx := kvIdx*ds.chunksPerKv + i
		encodedChunk := encoder(cb[int(i*ds.chunkSize):int((i+1)*ds.chunkSize)], chunkIdx)
		err := ds.writeChunk(chunkIdx, encodedChunk)

		if err != nil {
			return err
		}
	}
	// This is not atomic, but we should get error since we already pre-allocate the space
	return ds.WriteMeta(kvIdx, commit[:])
}

// Write a value of the KV to the store.  The value will be encoded with kvIdx and SP address.
func (ds *DataShard) Write(kvIdx uint64, b []byte, commit common.Hash) error {
	return ds.WriteWith(kvIdx, b, commit, func(cdata []byte, chunkIdx uint64) []byte {
		encodeKey := calcEncodeKey(commit, chunkIdx, ds.Miner())
		encodedChunk := encodeChunk(ds.chunkSize, cdata, ds.EncodeType(), encodeKey)
		return encodedChunk
	})
}

func (ds *DataShard) readChunk(chunkIdx uint64, readLen int) ([]byte, error) {
	for _, df := range ds.dataFiles {
		if df.Contains(chunkIdx) {
			return df.Read(chunkIdx, readLen)
		}
	}
	return nil, fmt.Errorf("chunk not found: the shard is not completed?")
}

func (ds *DataShard) writeChunk(chunkIdx uint64, b []byte) error {
	for _, df := range ds.dataFiles {
		if df.Contains(chunkIdx) {
			return df.Write(chunkIdx, b)
		}
	}
	return fmt.Errorf("chunk not found: the shard is not completed?")
}

func (ds *DataShard) WriteMeta(kvIdx uint64, b []byte) error {
	for _, df := range ds.dataFiles {
		if df.ContainsKv(kvIdx) {
			return df.WriteMeta(kvIdx, b)
		}
	}
	return fmt.Errorf("kv not found: the shard is not completed?")
}

func (ds *DataShard) ReadMeta(kvIdx uint64) ([]byte, error) {
	for _, df := range ds.dataFiles {
		if df.ContainsKv(kvIdx) {
			return df.ReadMeta(kvIdx)
		}
	}
	return nil, fmt.Errorf("kv not found: the shard is not completed?")
}

func (ds *DataShard) Close() error {
	for _, df := range ds.dataFiles {
		if err := df.Close(); err != nil {
			return err
		}
	}
	return nil
}
