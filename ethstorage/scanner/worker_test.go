// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	kvSize    uint64 = 1 << 17
	kvEntries uint64 = 16
)

type MockStorageManager struct {
	es.StorageManager
	mock.Mock
}

func (m *MockStorageManager) TryReadMeta(kvIndex uint64) ([]byte, bool, error) {
	args := m.Called(kvIndex)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).([]byte), args.Bool(1), args.Error(2)
}

func (m *MockStorageManager) TryRead(kvIndex uint64, size int, commit common.Hash) ([]byte, bool, error) {
	args := m.Called(kvIndex, size, commit)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).([]byte), args.Bool(1), args.Error(2)
}

func (m *MockStorageManager) TryWriteWithMetaCheck(kvIndex uint64, commit common.Hash, blob []byte, fetchBlob es.FetchBlobFunc) error {
	args := m.Called(kvIndex, commit, blob, fetchBlob)
	return args.Error(0)
}

func (m *MockStorageManager) CheckMeta(kvIndex uint64) (common.Hash, error) {
	args := m.Called(kvIndex)
	return args.Get(0).(common.Hash), args.Error(1)
}

func (m *MockStorageManager) MaxKvSize() uint64 {
	return m.StorageManager.MaxKvSize()
}

func (m *MockStorageManager) KvEntries() uint64 {
	return m.StorageManager.KvEntries()
}

func (m *MockStorageManager) Shards() []uint64 {
	return m.StorageManager.Shards()
}

type MockL1Source struct {
	es.Il1Source
	mock.Mock
}

func (m *MockL1Source) GetKvMetas(kvIndices []uint64, blockNumber int64) ([][32]byte, error) {
	args := m.Called(kvIndices, blockNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][32]byte), args.Error(1)
}

func (m *MockL1Source) GetStorageLastBlobIdx(blockNumber int64) (uint64, error) {
	args := m.Called(blockNumber)
	return args.Get(0).(uint64), args.Error(1)
}

func checkMocks(t *testing.T, sm *MockStorageManager) {
	sm.AssertExpectations(t)
}

func fetchBlob(kvIndex uint64, hash common.Hash) ([]byte, error) {
	return []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, nil
}

func TestFixKv(t *testing.T) {
	tests := []struct {
		name       string
		mode       int
		setupMocks func(*MockStorageManager)
	}{
		{
			name: "successful_fixed_with_commit_in_sync",
			mode: modeCheckBlob,
			setupMocks: func(sm *MockStorageManager) {
				sm.On("CheckMeta", uint64(1)).Return(common.HexToHash("0a0b"), nil)
				sm.On("TryWriteWithMetaCheck", uint64(1), common.HexToHash("0a0b"), mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "successful_fixed_with_commit_mismatch",
			setupMocks: func(sm *MockStorageManager) {
				sm.On("CheckMeta", uint64(1)).Return(common.HexToHash("0a0b"), es.ErrCommitMismatch)
				sm.On("TryWriteWithMetaCheck", uint64(1), common.HexToHash("0a0b"), mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "successful_fixed_with_checkmeta_error",
			setupMocks: func(sm *MockStorageManager) {
				sm.On("CheckMeta", uint64(1)).Return(common.Hash{}, errors.New("check meta error"))
				sm.On("TryWriteWithMetaCheck", uint64(1), common.HexToHash("0102"), mock.Anything, mock.Anything).Return(nil)
			},
		},
		{
			name: "already_fixed_with_matching_commit",
			mode: modeCheckMeta,
			setupMocks: func(sm *MockStorageManager) {
				sm.On("CheckMeta", uint64(1)).Return(common.HexToHash("0a0b"), nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storageManager := es.NewStorageManager(es.NewShardManager(common.Address{}, kvSize, kvEntries, kvSize), nil)
			sm := &MockStorageManager{StorageManager: *storageManager}
			l1 := new(MockL1Source)

			tt.setupMocks(sm)

			worker := NewWorker(sm, nil, fetchBlob, l1, Config{
				Mode: tt.mode,
			}, log.New())

			err := worker.fixKv(uint64(1), common.HexToHash("0102"))
			assert.NoError(t, err)

			checkMocks(t, sm)
		})
	}
}
