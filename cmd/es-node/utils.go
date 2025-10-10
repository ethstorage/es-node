// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/urfave/cli"
)

const (
	fileName             = "shard-%d.dat"
	shardLenFlagName     = "shard_len"
	shardIndexFlagName   = "shard_index"
	encodingTypeFlagName = "encoding_type"
	kvIndexFlagName      = "kv_index"
	esRpcFlagName        = "es_rpc"
)

func getTopNShardListSortByDiff(ctx context.Context, pClient *eth.PollingClient, n int) ([]uint64, error) {
	var shardId uint64 = 0
	var diffs []*big.Int
	api := miner.NewL1MiningAPI(pClient, nil, nil)
	for {
		info, err := api.GetMiningInfo(ctx, pClient.ContractAddress(), shardId)
		if err != nil {
			lg.Error("Query difficulty by shard", "error", err)
			break
		}
		if info.Difficulty != nil && info.Difficulty.Cmp(big.NewInt(0)) == 0 {
			// shardId not exist yet
			break
		}
		lg.Info("Query difficulty by shard", "shard", shardId, "difficulty", info.Difficulty)
		diffs = append(diffs, info.Difficulty)
		shardId++
	}
	// get the shards with lowest difficulty
	sortedShardIds := sortBigIntSlice(diffs)
	var result []uint64
	if len(sortedShardIds) == 0 {
		// Will create at least one data file
		result = []uint64{0}
	} else {
		if len(sortedShardIds) < n {
			n = len(sortedShardIds)
		}
		for i := 0; i < n; i++ {
			result = append(result, uint64(sortedShardIds[i]))
		}
	}
	lg.Info("Get shard list", "shards", result)
	return result, nil
}

func createDataFile(cfg *storage.StorageConfig, shardIdxList []uint64, datadir string, encodingType int) ([]string, error) {
	lg.Info("Creating data files", "shardIdxList", shardIdxList, "dataDir", datadir)
	if _, err := os.Stat(datadir); os.IsNotExist(err) {
		if err := os.Mkdir(datadir, 0755); err != nil {
			lg.Error("Creating data directory", "error", err)
			return nil, err
		}
	}
	var files []string
	for _, shardIdx := range shardIdxList {
		dataFile := filepath.Join(datadir, fmt.Sprintf(fileName, shardIdx))
		if _, err := os.Stat(dataFile); err == nil {
			lg.Warn("Creating data file", "error", "file already exists, will not overwrite", "file", dataFile)
			continue
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
		lg.Info("Creating data file", "chunkIdxStart", startChunkId, "chunkIdxLen", chunkIdxLen, "chunkSize", cfg.ChunkSize, "miner", cfg.Miner, "encodeType", encodingType)

		df, err := es.Create(dataFile, startChunkId, chunkPerKv*cfg.KvEntriesPerShard, 0, cfg.KvSize, uint64(encodingType), cfg.Miner, cfg.ChunkSize)
		if err != nil {
			lg.Error("Creating data file", "error", err)
			return nil, err
		}
		lg.Info("Data file created", "shard", shardIdx, "file", dataFile, "kvIdxStart", df.KvIdxStart(), "kvIdxEnd", df.KvIdxEnd(), "miner", df.Miner())
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

func readRequiredFlag(ctx *cli.Context, flag cli.StringFlag) string {
	name := flag.GetName()
	if !ctx.IsSet(name) {
		lg.Crit("Flag or environment variable is required", "flag", name, "envVar", flag.EnvVar)
	}
	value := ctx.String(name)
	lg.Info("Read flag", "name", name, "value", value)
	return value
}

func downloadBlobFromRPC(endpoint string, kvIndex uint64, hash common.Hash) ([]byte, error) {
	rpcClient, err := rpc.DialOptions(context.Background(), endpoint, rpc.WithHTTPClient(http.DefaultClient))
	if err != nil {
		return nil, err
	}

	var result hexutil.Bytes
	// kvIndex, blobHash, encodeType, offset, length
	if err := rpcClient.Call(&result, "es_getBlob", kvIndex, hash, 0, 0, 4096*32); err != nil {
		return nil, err
	}

	var blob kzg4844.Blob
	copy(blob[:], result)
	commitment, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return nil, fmt.Errorf("blobToCommitment failed: %w", err)
	}
	blobhash := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commitment))
	fmt.Printf("blobhash from blob: %x\n", blobhash)
	if bytes.Compare(blobhash[:es.HashSizeInContract], hash[:es.HashSizeInContract]) != 0 {
		return nil, fmt.Errorf("invalid blobhash for %d want: %x, got: %x", kvIndex, hash, blobhash)
	}

	return result, nil
}

func initShardManager(ctx *cli.Context, l1Rpc string, l1contract common.Address, lg log.Logger) (*es.ShardManager, error) {
	miner := readRequiredFlag(ctx, flags.StorageMiner)
	if !common.IsHexAddress(miner) {
		return nil, fmt.Errorf("invalid miner address %s", miner)
	}
	client, err := eth.Dial(l1Rpc, l1contract, 0, lg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the Ethereum client: %w", err)
	}
	defer client.Close()

	storageCfg, err := client.LoadStorageConfigFromContract(common.HexToAddress(miner))
	if err != nil {
		return nil, fmt.Errorf("failed to load storage config: %w", err)
	}
	if !ctx.IsSet(flags.StorageFiles.Name) {
		return nil, fmt.Errorf("flag is required: %s", flags.StorageFiles.Name)
	}
	storageCfg.Filenames = ctx.StringSlice(flags.StorageFiles.Name)
	shardManager := es.NewShardManager(storageCfg.L1Contract, storageCfg.KvSize, storageCfg.KvEntriesPerShard, storageCfg.ChunkSize)
	for _, filename := range storageCfg.Filenames {
		fmt.Printf("Adding data file %s\n", filename)
		df, err := es.OpenDataFile(filename)
		if err != nil {
			return nil, fmt.Errorf("open data file failed: %w", err)
		}
		if df.Miner() != storageCfg.Miner {
			return nil, fmt.Errorf("miner mismatches data file")
		}
		shardManager.AddDataFileAndShard(df)
	}
	if shardManager.IsComplete() != nil {
		return nil, fmt.Errorf("data files are not completed")
	}
	return shardManager, nil
}
