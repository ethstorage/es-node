// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/peer"
)

// task represents the sync task for a storage shard.
type task struct {
	// These fields get serialized to leveldb on shutdown
	Contract      common.Address // Contract address
	ShardId       uint64         // ShardId
	SubTasks      []*subTask
	nextIdx       int
	healTask      *healTask
	SubEmptyTasks []*subEmptyTask

	// TODO: consider whether we need to retry those stateless peers or disconnect the peer
	statelessPeers map[peer.ID]struct{} // Peers that failed to deliver kv Data
	State          *SyncState

	done bool // Flag whether the task has done
}

// task which is used to write empty to storage file, so the files will fill up with encode data
type subEmptyTask struct {
	task *task

	First uint64 // First blob to fill in empty in this interval
	Last  uint64 // Last blob to fill in empty in this interval

	isRunning bool
	done      bool // Flag whether the subEmptyTask can be removed
}

// subTask represents the sync for a range of storage within a shard.
type subTask struct {
	task *task

	// E.g. if the range of subTask is 0 to 127.
	// The subTask will be initialized with next = 0, First = 0, Last = 128;
	// After a remote peer returns results for blob 0 ~ 15, but without blob 3;
	// Blob index 3 will be added to heal task list for retrieval;
	// Then next will change to 16, and First will change to 3,
	// which means next range request start from blob 16
	// and the range should cover by this subTask is from 3 to 127.
	// When saveSyncStatus() be called to serialize tasks and save it to DB,
	// healTask will be dropped, only subTask's First and Last params will be saved
	// That means when task be reloaded from DB, the subTask's First and next will be set to 3
	// and blobs 4 ~ 15 will retrieval again.
	// That is a balance between saving heal list which may be large and retrieving blobs.
	next  uint64 // next blob start to sync in the next BlobsByRange request
	First uint64 // First blob to sync in this interval, it is use for serialization and deserialization of subtask
	Last  uint64 // Last blob to sync in this interval

	isRunning bool
	done      bool // Flag whether the subTask can be removed
}

// healTask represents the sync task for healing blobs fail to fetch from remote  .
type healTask struct {
	task    *task
	Indexes map[uint64]int64 // Set of blobs currently queued for retrieval
}

func (h *healTask) remove(list []uint64) {
	for _, idx := range list {
		if _, ok := h.Indexes[idx]; ok {
			delete(h.Indexes, idx)
		}
	}
}

func (h *healTask) count() int {
	return len(h.Indexes)
}

func (h *healTask) insert(list []uint64) {
	for _, idx := range list {
		h.Indexes[idx] = 0
	}
}

func (h *healTask) refresh(list []uint64) {
	t := time.Now().UnixMilli()
	for _, idx := range list {
		h.Indexes[idx] = t
	}
}

func (h *healTask) hasIndexInRange(first, next uint64) (bool, uint64) {
	min, exist := next, false
	for idx := range h.Indexes {
		if idx < next && idx >= first {
			exist = true
			if min > idx {
				min = idx
			}
		}
	}
	return exist, min
}

func (h *healTask) getBlobIndexesForRequest(batch uint64) []uint64 {
	indexes := make([]uint64, 0)
	l := uint64(0)
	for idx, tm := range h.Indexes {
		if time.Now().UnixMilli()-tm > requestTimeoutInMillisecond.Milliseconds() {
			indexes = append(indexes, idx)
			l++
		}
		if l >= batch {
			break
		}
	}

	return indexes
}

type SyncProgress struct {
	Tasks []*task // The suspended kv tasks

	// TODO keep it to make it compatible
	// Status report during syncing phase
	BlobsSynced      uint64             // Number of kvs downloaded
	SyncedBytes      common.StorageSize // Number of kv bytes downloaded
	EmptyBlobsToFill uint64
	EmptyBlobsFilled uint64
	TotalSecondsUsed uint64
}
