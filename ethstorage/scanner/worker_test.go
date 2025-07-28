package scanner

import (
	"testing"

	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/assert"
)

func TestGetKvsInBatch(t *testing.T) {
	tests := []struct {
		name             string
		shards           []uint64
		kvEntries        uint64
		lastKvIdx        uint64
		batchSize        uint64
		batchStartIndex  uint64
		expectedKvs      []uint64
		expectedTotal    uint64
		expectedBatchEnd uint64
	}{
		{
			name:             "1 shard batch 1",
			shards:           []uint64{0},
			kvEntries:        8,
			lastKvIdx:        6,
			batchSize:        5,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4},
			expectedTotal:    7,
			expectedBatchEnd: 5,
		},
		{
			name:             "1 shard batch 2",
			shards:           []uint64{0},
			kvEntries:        8,
			lastKvIdx:        6,
			batchSize:        5,
			batchStartIndex:  5,
			expectedKvs:      []uint64{5, 6},
			expectedTotal:    7,
			expectedBatchEnd: 7,
		},
		{
			name:             "2 shards batch 1",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        5,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4},
			expectedTotal:    13,
			expectedBatchEnd: 5,
		},
		{
			name:             "2 shards batch 2",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        5,
			batchStartIndex:  5,
			expectedKvs:      []uint64{5, 6, 7, 8, 9},
			expectedTotal:    13,
			expectedBatchEnd: 10,
		},
		{
			name:             "2 shards batch 3",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        5,
			batchStartIndex:  10,
			expectedKvs:      []uint64{10, 11, 12},
			expectedTotal:    13,
			expectedBatchEnd: 13,
		},
		{
			name:             "Batch spans 2 shards 1",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        10,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			expectedTotal:    13,
			expectedBatchEnd: 10,
		},
		{
			name:             "Batch spans 2 shards 2",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        10,
			batchStartIndex:  10,
			expectedKvs:      []uint64{10, 11, 12},
			expectedTotal:    13,
			expectedBatchEnd: 13,
		},
		{
			name:             "Batch spans 2 shards 3: kv increases",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        13,
			batchSize:        10,
			batchStartIndex:  13,
			expectedKvs:      []uint64{13},
			expectedTotal:    14,
			expectedBatchEnd: 14,
		},
		{
			name:             "Batch exceeds total entries 1",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        20,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			expectedTotal:    13,
			expectedBatchEnd: 13,
		},
		{
			name:             "Batch exceeds total entries 2",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        20,
			batchStartIndex:  13,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
			expectedTotal:    13,
			expectedBatchEnd: 13,
		},
		{
			name:             "Discontinuous shards batch 1",
			shards:           []uint64{0, 2},
			kvEntries:        8,
			lastKvIdx:        21,
			batchSize:        10,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 16, 17},
			expectedTotal:    14,
			expectedBatchEnd: 10,
		},
		{
			name:             "Discontinuous shards batch 2",
			shards:           []uint64{0, 2},
			kvEntries:        8,
			lastKvIdx:        21,
			batchSize:        10,
			batchStartIndex:  10,
			expectedKvs:      []uint64{18, 19, 20, 21},
			expectedTotal:    14,
			expectedBatchEnd: 14,
		},
		{
			name:             "Boundary conditions 1 kv",
			shards:           []uint64{0},
			kvEntries:        8,
			lastKvIdx:        0,
			batchSize:        5,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0},
			expectedTotal:    1,
			expectedBatchEnd: 1,
		},
		{
			name:             "Kv exceeds local shard",
			shards:           []uint64{0},
			kvEntries:        8,
			lastKvIdx:        8,
			batchSize:        10,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7},
			expectedTotal:    8,
			expectedBatchEnd: 8,
		},
		{
			name:             "Empty local shards",
			shards:           []uint64{0, 1, 2, 3},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        10,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			expectedTotal:    13,
			expectedBatchEnd: 10,
		},
		{
			name:             "Empty local shards next",
			shards:           []uint64{0, 1, 2, 3},
			kvEntries:        8,
			lastKvIdx:        12,
			batchSize:        10,
			batchStartIndex:  10,
			expectedKvs:      []uint64{10, 11, 12},
			expectedTotal:    13,
			expectedBatchEnd: 13,
		},

		{
			name:             "Full shards",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        15,
			batchSize:        10,
			batchStartIndex:  0,
			expectedKvs:      []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			expectedTotal:    16,
			expectedBatchEnd: 10,
		},
		{
			name:             "Full shards next",
			shards:           []uint64{0, 1},
			kvEntries:        8,
			lastKvIdx:        15,
			batchSize:        10,
			batchStartIndex:  10,
			expectedKvs:      []uint64{10, 11, 12, 13, 14, 15},
			expectedTotal:    16,
			expectedBatchEnd: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := log.New()

			kvs, total, batchEnd := getKvsInBatch(tt.shards, tt.kvEntries, tt.lastKvIdx, tt.batchSize, tt.batchStartIndex, logger)

			assert.Equal(t, tt.expectedKvs, kvs, "KV indices do not match")
			assert.Equal(t, tt.expectedTotal, total, "Total entries do not match")
			assert.Equal(t, tt.expectedBatchEnd, batchEnd, "Batch end index does not match")
		})
	}
}
