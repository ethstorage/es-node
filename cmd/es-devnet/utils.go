// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"math/big"
	"os"
	"path/filepath"
)

const fileName = "shard-%d.dat"

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

func createDataFile(cfg *storage.StorageConfig, shardIdxList []uint64, datadir string) ([]string, error) {
	log.Info("Creating data files", "shardIdxList", shardIdxList, "dataDir", datadir)
	if _, err := os.Stat(datadir); os.IsNotExist(err) {
		if err := os.Mkdir(datadir, 0755); err != nil {
			log.Error("Creating data directory", "error", err)
			return nil, err
		}
	}
	var files []string
	for index := range shardIdxList {
		shardIdx := uint64(index)
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

func initDataShard(shardIdx uint64, filename string, storageCfg *storage.StorageConfig) *es.DataShard {
	ds := es.NewDataShard(shardIdx, storageCfg.KvSize, storageCfg.KvEntriesPerShard, storageCfg.ChunkSize)
	var err error
	var df *es.DataFile
	df, err = es.OpenDataFile(filename)
	if err != nil {
		log.Crit("Open failed", "error", err)
	}
	err = ds.AddDataFile(df)
	if err != nil {
		log.Crit("Open failed", "error", err)
	}
	if !ds.IsComplete() {
		log.Warn("Shard is not completed")
	}
	return ds
}

func genRandomCanonicalScalar() [32]byte {
	maxCanonical := new(big.Int)
	_, success := maxCanonical.SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000000", 16)
	if !success {
		log.Crit("Error creating modulus")
	}
	randomNum, err := crand.Int(crand.Reader, maxCanonical)
	if err != nil {
		log.Crit("Error generate random number")
	}
	var res [32]byte
	randomNum.FillBytes(res[:])
	return res
}

func writeBlob(kvIdx uint64, blob kzg4844.Blob, ds *es.DataShard) common.Hash {
	commitments, _, versionedHashes, err := utils.ComputeBlobs([]kzg4844.Blob{blob})
	if err != nil {
		log.Crit("Compute versioned hash failed", "error", err)
	} else {
		log.Info(fmt.Sprintf("Versioned hash is %x and commitment is %x", versionedHashes[0][:], commitments[0][:]))
	}

	err = ds.Write(kvIdx, blob[:], versionedHashes[0])
	if err != nil {
		log.Crit("Write failed", "error", err)
	}
	log.Info("Write value", "kvIdx", kvIdx, "bytes", len(blob[:]))
	return versionedHashes[0]
}
