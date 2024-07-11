// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
)

type esAPI struct {
	rpcCfg *RPCConfig
	log    log.Logger
	sm     *ethstorage.StorageManager
	dl     *downloader.Downloader
}

type DecodeType uint64

const (
	RawData DecodeType = iota
	PaddingPer31Bytes
	OptimismCompact
)

func NewESAPI(config *RPCConfig, sm *ethstorage.StorageManager, dl *downloader.Downloader, log log.Logger) *esAPI {
	return &esAPI{
		rpcCfg: config,
		sm:     sm,
		dl:     dl,
		log:    log,
	}
}

func (api *esAPI) GetBlob(kvIndex uint64, blobHash common.Hash, decodeType DecodeType, off, size uint64) (hexutil.Bytes, error) {
	blob := api.dl.Cache.GetKeyValueByIndex(kvIndex, blobHash)

	if blob == nil {
		commit, _, err := api.sm.TryReadMeta(kvIndex)
		if err != nil {
			return nil, err
		}

		if !bytes.Equal(commit[0:ethstorage.HashSizeInContract], blobHash[0:ethstorage.HashSizeInContract]) {
			return nil, errors.New("commits not same")
		}

		readCommit := common.Hash{}
		copy(readCommit[0:ethstorage.HashSizeInContract], blobHash[0:ethstorage.HashSizeInContract])

		var found bool
		blob, found, err = api.sm.TryRead(kvIndex, int(api.sm.MaxKvSize()), readCommit)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ethereum.NotFound
		}
	} else {
		blob = api.sm.DecodeBlob(blob, blobHash, kvIndex, api.sm.MaxKvSize())
	}

	ret := blob

	if decodeType == PaddingPer31Bytes {
		ret = utils.DecodeBlob(blob)
	} else if decodeType == OptimismCompact {
		var err error
		if ret, err = utils.ToData(blob); err != nil {
			return nil, err
		}
	}

	if len(ret) < int(off+size) {
		return nil, errors.New("beyond the range of blob size")
	}

	return ret[off : off+size], nil
}
