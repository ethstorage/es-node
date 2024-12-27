// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	EthStorageENRKey = "ethstorage"

	AllShardDone = iota
	SingleShardDone
)

type Msg struct {
	ReturnCode byte
	Payload    []byte
}

type BlobPayload struct {
	MinerAddress common.Address `json:"minerAddress"`
	BlobIndex    uint64         `json:"blobIndex"`
	BlobCommit   common.Hash    `json:"blobCommit"`
	EncodeType   uint64         `json:"encodeType"`
	EncodedBlob  []byte         `json:"blob"`
}

type blobsByRangeRequest struct {
	peer     peer.ID
	id       uint64
	contract common.Address
	shardId  uint64
	origin   uint64
	limit    uint64

	subTask *subTask
	time    time.Time // Timestamp when the request was sent
}

type blobsByListRequest struct {
	peer     peer.ID
	id       uint64
	contract common.Address
	shardId  uint64
	indexes  []uint64

	healTask *healTask
	time     time.Time // Timestamp when the request was sent
}

type blobsByRangeResponse struct {
	req   *blobsByRangeRequest
	Blobs []*BlobPayload // List of the returning Blobs data

	time time.Time // Timestamp when the request was sent
}

type blobsByListResponse struct {
	req   *blobsByListRequest
	Blobs []*BlobPayload // List of the returning Blobs data
	time  time.Time      // Timestamp when the request was sent
}

// GetBlobsByRangePacket represents a Blobs query.
type GetBlobsByRangePacket struct {
	ID       uint64         // Request ID to match up responses with
	Contract common.Address // Contract of the sharded storage
	ShardId  uint64         // ShardId
	Origin   uint64         // Index of the first Blob to retrieve
	Limit    uint64         // Index of the last Blob to retrieve
	Bytes    uint64         // Soft limit at which to stop returning data
}

// BlobsByRangePacket represents a Blobs query response.
type BlobsByRangePacket struct {
	ID       uint64         // ID of the request this is a response for
	Contract common.Address // Contract of the sharded storage
	ShardId  uint64
	Blobs    []*BlobPayload // List of the returning Blobs data
}

// GetBlobsByListPacket represents a Blobs query.
type GetBlobsByListPacket struct {
	ID       uint64         // Request ID to match up responses with
	Contract common.Address // Contract of the sharded storage
	ShardId  uint64         // ShardId
	BlobList []uint64       // BlobList index list to retrieve
	Bytes    uint64         // Soft limit at which to stop returning data
}

// BlobsByListPacket represents a Blobs query response.
type BlobsByListPacket struct {
	ID       uint64         // ID of the request this is a response for
	Contract common.Address // Contract of the sharded storage
	ShardId  uint64
	Blobs    []*BlobPayload // List of the returning Blobs data
}

type requestResultErr byte

func (r requestResultErr) Error() string {
	return fmt.Sprintf("peer failed to serve request with code %d", uint8(r))
}

func (r requestResultErr) ResultCode() byte {
	return byte(r)
}

type ContractShards struct {
	Contract common.Address
	ShardIds []uint64
}

// EthStorageENRData The discovery ENRs are just key-value lists, and we filter them by records tagged with the "ethstorage" key,
// and then check the chain ID and Version.
type EthStorageENRData struct {
	L1ChainID uint64
	Version   uint64
	Shards    []*ContractShards
}

func (e *EthStorageENRData) ENRKey() string {
	return EthStorageENRKey
}

type EthStorageSyncDone struct {
	DoneType int
	ShardId  uint64
}

type SyncerParams struct {
	MaxPeers              int
	InitRequestSize       uint64
	SyncConcurrency       uint64
	FillEmptyConcurrency  int
	MetaDownloadBatchSize uint64
}

type SyncState struct {
	PeerCount         int    `json:"peer_count"`
	BlobsSynced       uint64 `json:"blobs_synced"`
	BlobsToSync       uint64 `json:"blobs_to_sync"`
	SyncProgress      uint64 `json:"sync_progress"`
	SyncedSeconds     uint64 `json:"sync_seconds"`
	EmptyFilled       uint64 `json:"empty_filled"`
	EmptyToFill       uint64 `json:"empty_to_fill"`
	FillEmptyProgress uint64 `json:"fill_empty_progress"`
	FillEmptySeconds  uint64 `json:"fill_empty_seconds"`
}
