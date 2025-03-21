// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"golang.org/x/term"

	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
)

func main() {
	cfg := scanner.Config{
		Enabled:  true,
		Interval: 15,
	}
	mockStorageReader := &MockStorageReader{StorageManager: es.StorageManager{}}
	mockStorageReader.On("TryRead", mock.Anything, mock.Anything, mock.Anything).Return([]byte{0x1, 0x2, 0x3}, true, nil)
	mockStorageReader.On("MaxKvSize").Return(uint64(64))
	mockStorageReader.On("KvEntries").Return(uint64(16))
	mockStorageReader.On("Shards").Return([]uint64{0, 1})

	mockL1source := &MockL1Source{PollingClient: eth.PollingClient{}}
	mockL1source.On("GetStorageLastBlobIdx", mock.Anything).Return(uint64(38), nil)

	lgr := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})

	worker := scanner.New(context.Background(), cfg, mockStorageReader, mockL1source, lgr)
	worker.Start()
	time.Sleep(10 * time.Minute)
	worker.Close()
}

type MockStorageReader struct {
	mock.Mock
	es.StorageManager
}

func (m *MockStorageReader) TryRead(kvIdx uint64, readLen int, commit common.Hash) ([]byte, bool, error) {
	args := m.Called(kvIdx, readLen, commit)
	return args.Get(0).([]byte), args.Bool(1), args.Error(2)
}

func (m *MockStorageReader) KvEntries() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

func (m *MockStorageReader) MaxKvSize() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

func (m *MockStorageReader) Shards() []uint64 {
	args := m.Called()
	return args.Get(0).([]uint64)
}

type MockL1Source struct {
	mock.Mock
	eth.PollingClient
}

func (m *MockL1Source) GetStorageLastBlobIdx(block int64) (uint64, error) {
	args := m.Called(block)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockL1Source) GetKvMetas(kvIndexRange []uint64, block int64) ([][32]byte, error) {
	meta := make([][32]byte, len(kvIndexRange))
	for i, kvi := range kvIndexRange {
		bytes := make([]byte, 32)
		for j := range bytes {
			bytes[j] = byte(kvi)
		}
		copy(meta[i][:], bytes)
	}
	return meta, nil
}
