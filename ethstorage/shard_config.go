// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// TODO: move to config?
var ContractToShardManager = make(map[common.Address]*ShardManager)

type ShardInfo struct {
	Contract  common.Address
	KVSize    uint64
	KVEntries uint64
	ChunkSize uint64
}

// TODO: move to chain specific config?
var ShardInfos = []*ShardInfo{
	{common.HexToAddress("0x0000000000000000000000000000000003330001"), 128 * 1024, 256 * 1024, 4096},
}

func InitializeConfig() {
	for _, sinfo := range ShardInfos {
		ContractToShardManager[sinfo.Contract] = NewShardManager(sinfo.Contract, sinfo.KVSize, sinfo.KVEntries, sinfo.ChunkSize)
	}
}

func findShardManaager(kvSize uint64) *ShardManager {
	for _, v := range ContractToShardManager {
		if v.kvSize == kvSize {
			return v
		}
	}
	return nil
}

func parseKvSize(s string) (uint64, error) {
	if s[len(s)-1] == 'k' || s[len(s)-1] == 'K' {
		if v, err := strconv.Atoi(s[0 : len(s)-1]); err != nil {
			return 0, err
		} else {
			return uint64(v) * 1024, nil
		}
	} else {
		if v, err := strconv.Atoi(s); err != nil {
			return 0, err
		} else {
			return uint64(v), nil
		}
	}
}

func AddDataShardFromConfig(cfg string) error {
	// Format is kvSize,shardIdx
	ss := strings.Split(cfg, ",")
	if len(ss) != 2 || len(ss[0]) == 0 || len(ss[1]) == 0 {
		return fmt.Errorf("incorrect data shard cfg")
	}

	kvSize, err := parseKvSize(ss[0])
	if err != nil {
		return err
	}
	var shardIdx uint64

	sm := findShardManaager(kvSize)
	if sm == nil {
		return fmt.Errorf("shard with kv size %d not found", kvSize)
	}

	if v, err := strconv.Atoi(ss[1]); err != nil {
		return err
	} else {
		shardIdx = uint64(v)
	}
	return sm.AddDataShard(shardIdx)
}

func AddDataFileFromConfig(cfg string) error {
	df, err := OpenDataFile(cfg)
	if err != nil {
		return err
	}

	sm := findShardManaager(df.maxKvSize)
	if sm == nil {
		return fmt.Errorf("shard with kv size %d not found", df.maxKvSize)
	}

	return sm.AddDataFile(df)
}

func IsComplete() error {
	for _, sm := range ContractToShardManager {
		if err := sm.IsComplete(); err != nil {
			return err
		}
	}
	return nil
}

// Return a copy of the map of all shards hosted by the node. The method is thread-safe.
func Shards() map[common.Address][]uint64 {
	shardList := make(map[common.Address][]uint64, 0)
	for addr, sm := range ContractToShardManager {
		if sm != nil && len(sm.shardMap) > 0 {
			shardList[addr] = make([]uint64, 0, len(sm.shardMap))
			for idx := range sm.shardMap {
				shardList[addr] = append(shardList[addr], idx)
			}
		}
	}

	return shardList
}
