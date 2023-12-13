// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

func TestCreateDataFile(t *testing.T) {
	type args struct {
		sLen int
		cfg  *storage.StorageConfig
	}
	type rslt struct {
		ChunkIdxEnd, KvIdxEnd []uint64
		Miner                 common.Address
	}
	tests := []struct {
		name     string
		args     args
		expected rslt
		wantErr  bool
	}{
		{
			"",
			args{
				2,
				&storage.StorageConfig{
					L1Contract:        common.HexToAddress("0xeeca1001388a5d554D8935E7eaDa08C103E31337"),
					Miner:             common.HexToAddress("0x04580493117292ba13361D8e9e28609ec112264D"),
					KvSize:            uint64(131072),
					ChunkSize:         uint64(131072),
					KvEntriesPerShard: uint64(16),
				},
			},
			rslt{
				ChunkIdxEnd: []uint64{16, 32},
				KvIdxEnd:    []uint64{16, 32},
				Miner:       common.HexToAddress("0x04580493117292ba13361D8e9e28609ec112264D"),
			},
			false,
		},
	}
	client, err := ethclient.DialContext(context.Background(), "http://65.109.115.36:8545")
	if err != nil {
		t.Fatalf("connect to L1 error: %v ", err)
	}
	defer client.Close()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shardList, err := getShardList(context.Background(), client, tt.args.cfg.L1Contract, tt.args.sLen)
			if err != nil {
				t.Fatalf("getShardList() error: %v ", err)
			}
			files, err := createDataFile(tt.args.cfg, shardList, ".", ethstorage.ENCODE_BLOB_POSEIDON)
			if err != nil {
				t.Fatalf("createDataFile() error: %v ", err)
			}
			defer func() {
				for _, fileName := range files {
					os.Remove(fileName)
				}
			}()
			for i, fileName := range files {
				df, err := ethstorage.OpenDataFile(fileName)
				if err != nil {
					t.Fatalf("open data file failed: name=%s, error=%v", fileName, err)
				}
				chunkIndxEnd := df.ChunkIdxEnd()
				if chunkIndxEnd != tt.expected.ChunkIdxEnd[i] {
					t.Errorf("invalid ChunkIdxEnd expected: %d; actual: %d", tt.expected.ChunkIdxEnd, chunkIndxEnd)
				}
				kvIdxEnd := df.KvIdxEnd()
				if kvIdxEnd != tt.expected.KvIdxEnd[i] {
					t.Errorf("invalid KvIdxEnd expected: %d; actual: %d", tt.expected.KvIdxEnd, kvIdxEnd)
				}
				miner := df.Miner()
				if miner != tt.expected.Miner {
					t.Errorf("invalid Miner expected: %x; actual %x", tt.expected.Miner, miner)
				}
				if err = df.Close(); err != nil {
					t.Errorf("close data file failed: name=%s, error=%v", fileName, err)
				}
			}
		})
	}
}

func TestSortBigIntSlice(t *testing.T) {
	slice := []*big.Int{
		big.NewInt(12345678901234567),
		big.NewInt(11111111111111111),
		big.NewInt(98765432109543210),
		big.NewInt(55555555555555555),
	}
	result := sortBigIntSlice(slice)
	expected := []int{1, 0, 3, 2}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, but got %v", expected, result)
	}
}
