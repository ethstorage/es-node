// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"fmt"
	"math/bits"

	"github.com/ethereum/go-ethereum/common"
)

type ShardManager struct {
	shardMap        map[uint64]*DataShard
	contractAddress common.Address
	kvSizeBits      uint64
	kvSize          uint64
	chunksPerKvBits uint64
	chunksPerKv     uint64
	kvEntriesBits   uint64
	kvEntries       uint64
	chunkSize       uint64
	chunkSizeBits   uint64
}

// if v is not 2^n, panic; otherwise return n
func checkAndGetBits(v uint64) uint64 {
	if !isPow2n(v) {
		panic("v must be 2^n")
	}
	return uint64(63 - bits.LeadingZeros64(v))
}

func isPow2n(v uint64) bool {
	if v == 0 {
		return false
	}
	return v&(v-1) == 0
}

func NewShardManager(contractAddress common.Address, kvSize uint64, kvEntries uint64, chunkSize uint64) *ShardManager {
	if chunkSize > kvSize {
		panic("kvSize must > chunkSize")
	}

	kvSizeBits := checkAndGetBits(kvSize)
	chunkSizeBits := checkAndGetBits(chunkSize)
	kvEntriesBits := checkAndGetBits(kvEntries)

	sm := &ShardManager{
		shardMap:        make(map[uint64]*DataShard),
		contractAddress: contractAddress,
		kvSizeBits:      kvSizeBits,
		kvSize:          kvSize,
		kvEntriesBits:   kvEntriesBits,
		kvEntries:       kvEntries,
		chunksPerKvBits: kvSizeBits - chunkSizeBits,
		chunksPerKv:     kvSize / chunkSize,
		chunkSize:       chunkSize,
		chunkSizeBits:   chunkSizeBits,
	}

	ContractToShardManager[contractAddress] = sm
	return sm
}

func (sm *ShardManager) ContractAddress() common.Address {
	return sm.contractAddress
}

func (sm *ShardManager) ShardMap() map[uint64]*DataShard {
	return sm.shardMap
}

func (sm *ShardManager) ShardIds() []uint64 {
	shardIds := make([]uint64, 0)
	for id := range sm.shardMap {
		shardIds = append(shardIds, id)
	}
	return shardIds
}

func (sm *ShardManager) ChunkSize() uint64 {
	return sm.chunkSize
}

func (sm *ShardManager) ChunksPerKv() uint64 {
	return sm.chunksPerKv
}

func (sm *ShardManager) ChunksPerKvBits() uint64 {
	return sm.chunksPerKvBits
}

func (sm *ShardManager) KvEntries() uint64 {
	return sm.kvEntries
}

func (sm *ShardManager) KvEntriesBits() uint64 {
	return sm.kvEntriesBits
}

func (sm *ShardManager) MaxKvSize() uint64 {
	return sm.kvSize
}

func (sm *ShardManager) MaxKvSizeBits() uint64 {
	return sm.kvSizeBits
}

func (sm *ShardManager) AddDataShard(shardIdx uint64) error {
	if _, ok := sm.shardMap[shardIdx]; !ok {
		ds := NewDataShard(shardIdx, sm.kvSize, sm.kvEntries, sm.chunkSize)
		sm.shardMap[shardIdx] = ds
		return nil
	} else {
		return fmt.Errorf("data shard already exists")
	}
}

func (sm *ShardManager) AddDataFile(df *DataFile) error {
	shardIdx := df.chunkIdxStart / sm.chunksPerKv / sm.kvEntries
	var ds *DataShard
	var ok bool
	if ds, ok = sm.shardMap[shardIdx]; !ok {
		return fmt.Errorf("data shard not found")
	}

	return ds.AddDataFile(df)
}

func (sm *ShardManager) AddDataFileAndShard(df *DataFile) error {
	shardIdx := df.chunkIdxStart / sm.chunksPerKv / sm.kvEntries
	var ds *DataShard
	var ok bool
	if ds, ok = sm.shardMap[shardIdx]; !ok {
		ds = NewDataShard(shardIdx, sm.kvSize, sm.kvEntries, sm.chunkSize)
		sm.shardMap[shardIdx] = ds
	}

	return ds.AddDataFile(df)
}

// TryWrite Encode a raw KV data, and write it to the underly storage file.
// Return error if the write IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryWrite(kvIdx uint64, b []byte, commit common.Hash) (bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		return true, ds.Write(kvIdx, b, commit)
	} else {
		return false, nil
	}
}

// TryWriteEncoded write the encoded data to the underly storage file directly.
// Return error if the write IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryWriteEncoded(kvIdx uint64, b []byte, commit common.Hash) (bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		err := ds.WriteWith(kvIdx, b, commit, func(cdata []byte, chunkIdx uint64) []byte {
			return cdata
		})
		return true, err
	} else {
		return false, nil
	}
}

// TryRead Read the encoded KV data from storage file and decode it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryRead(kvIdx uint64, readLen int, commit common.Hash) ([]byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, err := ds.Read(kvIdx, readLen, commit)
		return b, true, err
	} else {
		return nil, false, nil
	}
}

// TryEncodeKV encode the KV data using the miner and encodeType specified by the data shard.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryEncodeKV(kvIdx uint64, b []byte, hash common.Hash) ([]byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		cb := make([]byte, ds.kvSize)
		copy(cb, b)
		return sm.EncodeKV(kvIdx, cb, hash, ds.Miner(), ds.EncodeType())
	} else {
		return nil, false, nil
	}
}

// TryReadWithMeta Read the encoded KV data and meta from storage file and decode it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryReadWithMeta(kvIdx uint64, readLen int) ([]byte, []byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, commit, err := ds.ReadWithMeta(kvIdx, readLen)
		return b, commit, true, err
	} else {
		return nil, nil, false, nil
	}
}

func (sm *ShardManager) GetShardMiner(shardIdx uint64) (common.Address, bool) {
	if ds, ok := sm.shardMap[shardIdx]; ok {
		return ds.Miner(), true
	}
	return common.Address{}, false
}

func (sm *ShardManager) GetShardEncodeType(shardIdx uint64) (uint64, bool) {
	if ds, ok := sm.shardMap[shardIdx]; ok {
		return ds.EncodeType(), true
	}
	return NO_ENCODE, false
}

// DecodeKV Decode the encoded KV data.
func (sm *ShardManager) DecodeKV(kvIdx uint64, b []byte, hash common.Hash, providerAddr common.Address, encodeType uint64) ([]byte, bool, error) {
	return sm.DecodeOrEncodeKV(kvIdx, b, hash, providerAddr, false, encodeType)
}

// EncodeKV Encode the raw KV data.
func (sm *ShardManager) EncodeKV(kvIdx uint64, b []byte, hash common.Hash, providerAddr common.Address, encodeType uint64) ([]byte, bool, error) {
	return sm.DecodeOrEncodeKV(kvIdx, b, hash, providerAddr, true, encodeType)
}

func (sm *ShardManager) DecodeOrEncodeKV(kvIdx uint64, b []byte, hash common.Hash, providerAddr common.Address, encode bool, encodeType uint64) ([]byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	var data []byte
	if ds, ok := sm.shardMap[shardIdx]; ok {
		datalen := len(b)
		for i := uint64(0); i < ds.chunksPerKv; i++ {
			if datalen == 0 {
				break
			}

			chunkReadLen := datalen
			if chunkReadLen > int(sm.chunkSize) {
				chunkReadLen = int(sm.chunkSize)
			}
			datalen = datalen - chunkReadLen

			chunkIdx := kvIdx*ds.chunksPerKv + i
			encodeKey := calcEncodeKey(hash, chunkIdx, providerAddr)
			var cdata []byte
			if encode {
				cdata = encodeChunk(sm.chunkSize, b[i*sm.chunkSize:i*sm.chunkSize+uint64(chunkReadLen)], encodeType, encodeKey)
			} else {
				cdata = decodeChunk(sm.chunkSize, b[i*sm.chunkSize:i*sm.chunkSize+uint64(chunkReadLen)], encodeType, encodeKey)
			}
			data = append(data, cdata...)
		}
		return data, true, nil
	}
	return nil, false, nil
}

// TryReadEncoded Read the encoded KV data from storage file and return it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryReadEncoded(kvIdx uint64, readLen int) ([]byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, err := ds.ReadEncoded(kvIdx, readLen) // read all the data
		return b[:readLen], true, err
	} else {
		return nil, false, nil
	}
}

// TryReadMeta Read the KV meta data from storage file and return it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryReadMeta(kvIdx uint64) ([]byte, bool, error) {
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, err := ds.ReadMeta(kvIdx) // read all the data
		return b, true, err
	} else {
		return nil, false, nil
	}
}

// TryReadChunk Read the encoded KV data using chunkIdx from storage file and decode it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryReadChunk(chunkIdx uint64, commit common.Hash) ([]byte, bool, error) {
	kvIdx := chunkIdx / sm.chunksPerKv
	cIdx := chunkIdx % sm.chunksPerKv
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, err := ds.ReadChunk(kvIdx, cIdx, commit) // read all the data
		return b, true, err
	} else {
		return nil, false, nil
	}
}

// TryReadChunkEncoded Read the encoded KV data using chunkIdx from storage file and return it.
// Return error if the read IO fails.
// Return false if the data is not managed by the ShardManager.
func (sm *ShardManager) TryReadChunkEncoded(chunkIdx uint64) ([]byte, bool, error) {
	kvIdx := chunkIdx / sm.chunksPerKv
	cIdx := chunkIdx % sm.chunksPerKv
	shardIdx := kvIdx / sm.kvEntries
	if ds, ok := sm.shardMap[shardIdx]; ok {
		b, err := ds.ReadChunkEncoded(kvIdx, cIdx) // read all the data
		return b, true, err
	} else {
		return nil, false, nil
	}
}

func (sm *ShardManager) IsComplete() error {
	for _, ds := range sm.shardMap {
		if !ds.IsComplete() {
			return fmt.Errorf("shard %d is not complete", ds.shardIdx)
		}
	}
	return nil
}

func (sm *ShardManager) Close() error {
	for _, ds := range sm.shardMap {
		if err := ds.Close(); err != nil {
			return err
		}
	}
	return nil
}
