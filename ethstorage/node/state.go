package node

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p/protocol"
	"github.com/ethstorage/go-ethstorage/ethstorage/scanner"
)

type ShardState struct {
	ShardId         uint64                 `json:"shard_id"`
	Miner           common.Address         `json:"miner"`
	SyncState       *protocol.SyncState    `json:"sync_state"`
	ProvidedBlob    uint64                 `json:"provided_blob"`
	MiningState     *miner.MiningState     `json:"mining_state"`
	SubmissionState *miner.SubmissionState `json:"submission_state"`
}

type NodeState struct {
	Id              string             `json:"id"`
	Contract        string             `json:"contract"`
	Version         string             `json:"version"`
	Address         string             `json:"address"`
	SavedBlobs      uint64             `json:"saved_blobs"`
	DownloadedBlobs uint64             `json:"downloaded_blobs"`
	ScanStats       *scanner.ScanStats `json:"scan_stats"`
	Shards          []*ShardState      `json:"shards"`
}
