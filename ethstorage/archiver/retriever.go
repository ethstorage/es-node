// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package archiver

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/status-im/keycard-go/hexutils"
)

type Retriever struct {
	downloader *downloader.Downloader
	storageMgr *ethstorage.StorageManager
	log        log.Logger
}

func (r *Retriever) GetBlobSidecars(beaconBlockHash common.Hash) (*eth.APIBlobSidecars, error) {
	output, err := r.downloader.ReadBlobSidecars(beaconBlockHash)
	if err != nil {
		return nil, err
	}
	bso := eth.APIBlobSidecars{}
	for _, sidecar := range output.Data {
		log.Info("Retrieved blob sidecars", "index", sidecar.Index, "kvIndex", sidecar.KvIndex, "commitment", hexutils.BytesToHex(sidecar.KZGCommitment[:]))

		commit, _, err := r.storageMgr.TryReadMeta(sidecar.KvIndex)
		if err != nil {
			return nil, err
		}
		s := ethstorage.HashSizeInContract
		blobHash, err := eth.KzgToVersionedHash(sidecar.KZGCommitment[:])
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(commit[0:s], blobHash[0:s]) {
			return nil, errors.New("commits not same")
		}
		readCommit := common.Hash{}
		copy(readCommit[0:s], blobHash[0:s])

		log.Info("Commits same", "versionedHash", blobHash.String(), "readCommit", readCommit.String())
		blobData, found, err := r.storageMgr.TryRead(sidecar.KvIndex, int(r.storageMgr.MaxKvSize()), readCommit)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ethereum.NotFound
		}
		var blb [eth.BlobLength]byte
		for i := 0; i < len(blobData); i++ {
			blb[i] = blobData[i]
		}
		sidecarOut := &eth.APIBlobSidecar{
			BlobSidecar: sidecar.BlobSidecar,
			Blob:        blb,
		}
		bso.Data = append(bso.Data, sidecarOut)
	}
	return &bso, nil
}
