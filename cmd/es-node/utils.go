// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/urfave/cli"
)

const (
	fileName           = "shard-%d.dat"
	shardLenFlagName   = "shard_len"
	shardIndexFlagName = "shard_index"
)

func initStorageConfig(ctx context.Context, client *ethclient.Client, l1Contract, miner common.Address) (*storage.StorageConfig, error) {
	maxKvSizeBits, err := readUintFromContract(ctx, client, l1Contract, "maxKvSizeBits")
	if err != nil {
		return nil, err
	}
	chunkSizeBits := maxKvSizeBits
	shardEntryBits, err := readUintFromContract(ctx, client, l1Contract, "shardEntryBits")
	if err != nil {
		return nil, err
	}
	return &storage.StorageConfig{
		L1Contract:        l1Contract,
		Miner:             miner,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}, nil
}

func readSlotFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) ([]byte, error) {
	h := crypto.Keccak256Hash([]byte(fieldName + "()"))
	msg := ethereum.CallMsg{
		To:   &l1Contract,
		Data: h[0:4],
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to get %s from contract: %v", fieldName, err)
	}
	return bs, nil
}

func readUintFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) (uint64, error) {
	bs, err := readSlotFromContract(ctx, client, l1Contract, fieldName)
	if err != nil {
		return 0, err
	}
	value := new(big.Int).SetBytes(bs).Uint64()
	log.Info("Read uint from contract", "field", fieldName, "value", value)
	return value, nil
}

func readBigIntFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) (*big.Int, error) {
	bs, err := readSlotFromContract(ctx, client, l1Contract, fieldName)
	if err != nil {
		return nil, err
	}
	value := new(big.Int).SetBytes(bs)
	log.Info("Read big int from contract", "field", fieldName, "value", value)
	return new(big.Int).SetBytes(bs), nil
}

func getShardList(ctx context.Context, client *ethclient.Client, contract common.Address, shardLen int) ([]uint64, error) {
	var shardId uint64 = 0
	var diffs []*big.Int
	for {
		diff, err := getDifficulty(ctx, client, contract, shardId)
		if err != nil {
			log.Error("Query difficulty by shard", "error", err)
			break
		}
		if diff != nil && diff.Cmp(big.NewInt(0)) == 0 {
			// shardId not exist yet
			break
		}
		log.Info("Query difficulty by shard", "shard", shardId, "difficulty", diff)
		diffs = append(diffs, diff)
		shardId++
	}
	// get the shards with lowest difficulty
	sortedShardIds := sortBigIntSlice(diffs)
	var result []uint64
	if len(sortedShardIds) == 0 {
		// Will create at least one data file
		result = []uint64{0}
	} else {
		if len(sortedShardIds) < shardLen {
			shardLen = len(sortedShardIds)
		}
		for i := 0; i < shardLen; i++ {
			result = append(result, uint64(sortedShardIds[i]))
		}
	}
	log.Info("Get shard list", "shards", result)
	return result, nil
}

func getDifficulty(ctx context.Context, client *ethclient.Client, contract common.Address, shardIdx uint64) (*big.Int, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, _ := abi.Arguments{{Type: uint256Type}}.Pack(new(big.Int).SetUint64(shardIdx))
	h := crypto.Keccak256Hash([]byte(`infos(uint256)`))
	calldata := append(h[0:4], dataField...)
	msg := ethereum.CallMsg{
		To:   &contract,
		Data: calldata,
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		log.Error("Failed to call contract", "error", err.Error())
		return nil, err
	}
	res, _ := abi.Arguments{
		{Type: uint256Type},
		{Type: uint256Type},
		{Type: uint256Type},
	}.UnpackValues(bs)
	if res == nil || len(res) < 3 {
		log.Error("Query difficulty by shard", "error", "invalid result", "result", res)
		return nil, fmt.Errorf("invalid result: %v", res)
	}
	return res[1].(*big.Int), nil
}

func createDataFile(cfg *storage.StorageConfig, shardIdxList []uint64, datadir string) ([]string, error) {
	log.Info("Creating data files", "shardIdxList", shardIdxList, "dataDir", datadir)
	if _, err := os.Stat(datadir); os.IsNotExist(err) {
		if err := os.Mkdir(datadir, 0755); err != nil {
			log.Error("Creating data directory", "error", err)
			return nil, err
		}
	}
	var files []string
	for _, shardIdx := range shardIdxList {
		dataFile := filepath.Join(datadir, fmt.Sprintf(fileName, shardIdx))
		if _, err := os.Stat(dataFile); err == nil {
			log.Error("Creating data file", "error", "file already exists, will not overwrite", "file", dataFile)
			return nil, err
		}
		if cfg.ChunkSize == 0 {
			return nil, fmt.Errorf("chunk size should not be 0")
		}
		if cfg.KvSize%cfg.ChunkSize != 0 {
			return nil, fmt.Errorf("max kv size %% chunk size should be 0")
		}
		chunkPerKv := cfg.KvSize / cfg.ChunkSize
		startChunkId := shardIdx * cfg.KvEntriesPerShard * chunkPerKv
		chunkIdxLen := chunkPerKv * cfg.KvEntriesPerShard
		log.Info("Creating data file", "chunkIdxStart", startChunkId, "chunkIdxLen", chunkIdxLen, "chunkSize", cfg.ChunkSize, "miner", cfg.Miner, "encodeType", es.ENCODE_BLOB_POSEIDON)

		df, err := es.Create(dataFile, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, es.ENCODE_BLOB_POSEIDON, cfg.Miner, cfg.ChunkSize)
		if err != nil {
			log.Error("Creating data file", "error", err)
			return nil, err
		}
		log.Info("Data file created", "shard", shardIdx, "file", dataFile, "chunkIdxStart", df.KvIdxStart(), "ChunkIdxEnd", df.ChunkIdxEnd(), "miner", df.Miner())
		files = append(files, dataFile)
	}
	return files, nil
}

func sortBigIntSlice(slice []*big.Int) []int {
	indices := make([]int, len(slice))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return slice[indices[i]].Cmp(slice[indices[j]]) < 0
	})
	return indices
}

func readRequiredFlag(ctx *cli.Context, name string) string {
	if !ctx.IsSet(name) {
		log.Crit("Flag is required", "flag", name)
	}
	value := ctx.String(name)
	log.Info("Read flag", "name", name, "value", value)
	return value
}
