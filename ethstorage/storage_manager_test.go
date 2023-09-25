// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package ethstorage

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)


var (
    contractAddress = common.HexToAddress("0x31c2078945359086152687E43F30d94A52141bEc")
    rawURL = "http://65.108.236.27:8545"
    filename = "../cmd/es-node/storage.dat"
    pc *eth.PollingClient
    sm *ShardManager
    storageManager * StorageManager
)

func setup(t *testing.T) {
    var err error
    pc, err = eth.Dial(rawURL, contractAddress, log.New())
    if err != nil {
        t.Fatal("create polling client failed")
        return
    }

    sm = NewShardManager(contractAddress, 131072, 16, 131072)

    var df *DataFile
    df, err = OpenDataFile(filename)
    if err != nil {
        t.Fatal("open data file failed")
        return
    }

    err = sm.AddDataFileAndShard(df)
    if err != nil {
        t.Fatal("add data file failed")
        return
    }

    storageManager = NewStorageManager(sm, pc)
    err = storageManager.DownloadFinished(97528, []uint64{}, [][]byte{}, []common.Hash{})
    if err != nil {
        t.Fatal("set local L1 failed", err)
        return
    }

}

func TestStorageManager_LastKvIndex(t *testing.T) {
    setup(t)
    idx, err := storageManager.LastKvIndex()
    if err != nil {
        t.Fatal("failed to get lastKvIndex", err)
    }

    t.Log("lastKvIndex", idx)
}

func TestStorageManager_DownloadFinished(t *testing.T) {
    setup(t)
    h := common.Hash{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
    err := storageManager.DownloadFinished(97529, []uint64{2}, [][]byte{{10}}, []common.Hash{h})

    if err != nil {
        t.Fatal("failed to Downloand Finished", err)
    }

    bs, success, err := storageManager.TryReadMeta(2)
    if err != nil || !success {
        t.Fatal("failed to read meta", err)
    }

    meta := common.Hash{}
    copy(meta[:], bs)
    if meta != h {
        t.Fatal("failed to write meta", err)
    }
}

func TestStorageManager_CommitBlobs(t *testing.T) {
    setup(t)
    h := common.Hash{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
    failedCommited, err := storageManager.CommitBlobs([]uint64{2}, [][]byte{{10}}, []common.Hash{h})
    if err != nil {
        t.Fatal("failed to commit blob", err)
    }

    if len(failedCommited) != 0 {
        t.Fatal("should commit all the blobs")
    }

    bs, success, err := storageManager.TryReadMeta(2)
    if err != nil || !success {
        t.Fatal("failed to read meta", err)
    }

    meta := common.Hash{}
    copy(meta[:], bs)
    if meta != h {
        t.Fatal("failed to write meta", err)
    }
}