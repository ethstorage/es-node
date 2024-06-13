// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package eth

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var l1BlockContract = common.HexToAddress("0x4200000000000000000000000000000000000015")

// RandaoClient is a client that fetches the latest randao from the L1 block contract.
// It uses the PollingClient to fetch the latest block number and then fetches the header
// corresponding to that block number.
type RandaoClient struct {
	*PollingClient
	// rpc endpoint where randao is fetched from
	rc        *ethclient.Client
	rCtx      context.Context
	rCancel   context.CancelFunc
	rClosedCh chan struct{}
}

func DialRandaoSource(randaoUrl string, p *PollingClient) (*RandaoClient, error) {
	ctx := context.Background()
	c, err := ethclient.DialContext(ctx, randaoUrl)
	if err != nil {
		return nil, err
	}
	return NewRandaoClient(ctx, p, c), nil
}

// NewRandaoClient creates a client that uses the given RPC client.
func NewRandaoClient(ctx context.Context, p *PollingClient, c *ethclient.Client) *RandaoClient {
	cCtx, cancel := context.WithCancel(ctx)
	res := &RandaoClient{
		PollingClient: p,
		rc:            c,
		rCtx:          cCtx,
		rCancel:       cancel,
		rClosedCh:     make(chan struct{}),
	}
	return res
}

// Close closes the RandaoClient and the underlying RPC client it talks to.
func (w *RandaoClient) Close() {
	w.rCancel()
	<-w.rClosedCh
	w.rc.Close()
}

func (w *RandaoClient) GetLatestNumber() (*big.Int, error) {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	h := crypto.Keccak256Hash([]byte("number()"))
	ret, err := w.CallContract(ctx, ethereum.CallMsg{
		From: common.Address{},
		To:   &l1BlockContract,
		Data: h[0:4],
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call number() %v", err)
	}
	w.lgr.Info("DEBUG: GetLatestNumber from number()")
	// The latest blockhash could be empty
	curBlock := new(big.Int).Sub(new(big.Int).SetBytes(ret), common.Big1)
	return curBlock, nil
}

func (w *RandaoClient) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	return w.rc.HeaderByNumber(ctx, blockNumber)
}
