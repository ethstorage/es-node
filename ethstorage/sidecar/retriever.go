// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package sidecar

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
)

type Retriever struct {
	downloader *downloader.Downloader
	storageMgr *ethstorage.StorageManager
	log        log.Logger
}

func (r *Retriever) BlobSidecars(beaconBlockHash common.Hash) (*eth.BlobSidecars, error) {
	output, err := r.downloader.ReadBlobSidecars(beaconBlockHash)
	if err != nil {
		return nil, err
	}
	bso := eth.BlobSidecars{}
	for _, sidecar := range output.Data {
		log.Info("blob", "index", sidecar.Index, "kvIndex", sidecar.KvIndex)

		commit, _, err := r.storageMgr.TryReadMeta(sidecar.KvIndex)
		if err != nil {
			return nil, err
		}
		s := ethstorage.HashSizeInContract
		blobHash, err := eth.KzgToVersionedHash(string(sidecar.KZGCommitment[:]))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(commit[0:s], blobHash[0:s]) {
			return nil, errors.New("commits not same")
		}

		readCommit := common.Hash{}
		copy(readCommit[0:s], blobHash[0:s])

		blobData, found, err := r.storageMgr.TryRead(sidecar.KvIndex, int(r.storageMgr.MaxKvSize()), readCommit)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ethereum.NotFound
		}
		var blb [eth.BlobSize]byte
		for i := 0; i < len(blobData); i++ {
			blb[i] = blobData[i]
		}
		sidecarOut := &eth.BlobSidecar{
			BlobSidecarMeta: *&sidecar.BlobSidecarMeta,
			Blob:            blb,
		}
		bso.Data = append(bso.Data, sidecarOut)
	}
	return &bso, nil
}
