// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package storage

import "github.com/ethereum/go-ethereum/common"

type StorageConfig struct {
	Filenames         []string
	KvSize            uint64
	ChunkSize         uint64
	KvEntriesPerShard uint64
	L1Contract        common.Address
	Miner             common.Address
}
