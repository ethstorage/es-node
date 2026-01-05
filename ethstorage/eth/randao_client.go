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
	"github.com/ethereum/go-ethereum/log"
)

var l1BlockContract = common.HexToAddress("0x4200000000000000000000000000000000000015")

// RandaoClient is a client that fetches the latest randao from the L1 block contract.
// It uses the PollingClient to fetch the latest block number and then fetches the header
// corresponding to that block number.
type RandaoClient struct {
	// inherits PollingClient to reuse SubscribeNewHead
	*PollingClient
	// rpc endpoint where randao is fetched from
	rc *ethclient.Client
}

func DialRandaoSource(ctx context.Context, randaoUrl, rawurl string, pollRate uint64, lg log.Logger) (*RandaoClient, error) {
	rc, err := ethclient.DialContext(ctx, randaoUrl)
	if err != nil {
		return nil, err
	}
	cc, err := ethclient.DialContext(ctx, rawurl)
	if err != nil {
		return nil, err
	}
	bq := &RandaoBlockQuerier{rc, cc, lg}
	p := NewClient(ctx, cc, httpRegex.MatchString(rawurl), common.Address{}, pollRate, bq.GetLatestHeader, lg)
	return NewRandaoClient(ctx, p, rc), nil
}

// NewRandaoClient creates a client that uses the given RPC client.
func NewRandaoClient(ctx context.Context, p *PollingClient, c *ethclient.Client) *RandaoClient {
	res := &RandaoClient{
		PollingClient: p,
		rc:            c,
	}
	return res
}

func (w *RandaoClient) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	w.lg.Debug("Fetching header by number by Randao client", "number", blockNumber)
	return w.rc.HeaderByNumber(ctx, blockNumber)
}

// Close closes the RandaoClient and the underlying RPC client it talks to.
func (w *RandaoClient) Close() {
	w.rc.Close()
	w.PollingClient.Close()
}

type RandaoBlockQuerier struct {
	// where randao is queried from
	rc *ethclient.Client
	// where contract is deployed
	cc *ethclient.Client
	lg log.Logger
}

func (q *RandaoBlockQuerier) getLatestNumber(ctx context.Context) (*big.Int, error) {
	h := crypto.Keccak256Hash([]byte("number()"))
	ret, err := q.cc.CallContract(ctx, ethereum.CallMsg{
		To:   &l1BlockContract,
		Data: h[0:4],
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call number() %v", err)
	}
	curBlock := new(big.Int).SetBytes(ret)
	q.lg.Debug("Got latest block number by Randao querier", "number", curBlock)
	return curBlock, nil
}

func (q *RandaoBlockQuerier) GetLatestHeader() (*types.Header, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	blockNumber, err := q.getLatestNumber(ctx)
	if err != nil {
		return nil, err
	}
	q.lg.Debug("Fetching header by number by Randao querier", "number", blockNumber)
	return q.rc.HeaderByNumber(ctx, blockNumber)
}
