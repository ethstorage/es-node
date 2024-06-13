package eth

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

type RandaoClient struct {
	// rpc endpoint where randao is fetched from
	c1 *ethclient.Client
	// rpc endpoint where ethstorage contract is deployed
	c2 *ethclient.Client

	lgr             log.Logger
	pollRate        time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	currL1Number    *big.Int
	l1BlockContract common.Address
	subID           int

	// pollReqCh is used to request new polls of the upstream
	// RPC client.
	pollReqCh chan struct{}

	mtx sync.RWMutex

	subs map[int]chan *types.Header

	closedCh chan struct{}
}

// Dial connects a client to the given URL.
func DialRandaoSource(randaoUrl string, c2 *ethclient.Client, lgr log.Logger) (*RandaoClient, error) {
	ctx := context.Background()
	c, err := ethclient.DialContext(ctx, randaoUrl)
	if err != nil {
		return nil, err
	}
	return NewRandaoClient(ctx, c, c2, lgr), nil
}

// NewRandaoClient creates a client that uses the given RPC client.
func NewRandaoClient(ctx context.Context, c1, c2 *ethclient.Client, lgr log.Logger) *RandaoClient {
	ctx, cancel := context.WithCancel(ctx)
	res := &RandaoClient{
		c1:              c1,
		c2:              c2,
		lgr:             lgr,
		pollRate:        2 * time.Second, // TODO: @Qiang everytime devnet changed, we may need to change it
		ctx:             ctx,
		cancel:          cancel,
		l1BlockContract: common.HexToAddress("0x4200000000000000000000000000000000000015"),
		pollReqCh:       make(chan struct{}, 1),
		subs:            make(map[int]chan *types.Header),
		closedCh:        make(chan struct{}),
	}
	go res.pollHeads()
	return res
}

// Close closes the RandaoClient and the underlying RPC client it talks to.
func (w *RandaoClient) Close() {
	w.cancel()
	<-w.closedCh
	w.c1.Close()
}

func (w *RandaoClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	select {
	case <-w.ctx.Done():
		return nil, ErrSubscriberClosed
	default:
	}

	sub := make(chan *types.Header, 1)
	w.mtx.Lock()
	subID := w.subID
	w.subID++
	w.subs[subID] = sub
	w.mtx.Unlock()

	return event.NewSubscription(func(quit <-chan struct{}) error {
		for {
			select {
			case header := <-sub:
				ch <- header
			case <-quit:
				w.mtx.Lock()
				delete(w.subs, subID)
				w.mtx.Unlock()
				return nil
			case <-w.ctx.Done():
				return nil
			}
		}
	}), nil
}

func (w *RandaoClient) pollHeads() {
	// To prevent polls from stacking up in case HTTP requests
	// are slow, use a similar model to the driver in which
	// polls are requested manually after each header is fetched.
	reqPollAfter := func() {
		if w.pollRate == 0 {
			return
		}
		time.AfterFunc(w.pollRate, w.reqPoll)
	}

	reqPollAfter()

	defer close(w.closedCh)

	for {
		select {
		case <-w.pollReqCh:
			// We don't need backoff here because we'll just try again
			// after the pollRate elapses.
			num, err := w.getLatestNumber()
			if err != nil {
				w.lgr.Info("error getting latest l1 block number", "err", err)
				reqPollAfter()
				continue
			}
			if w.currL1Number != nil && w.currL1Number.Cmp(num) == 0 {
				w.lgr.Trace("no change in l1 number, skipping notifications")
				reqPollAfter()
				continue
			}

			head, err := w.HeaderByNumber(w.ctx, num)
			if err != nil {
				w.lgr.Error("failed to get header by number", "err", err)
				reqPollAfter()
				continue
			}
			w.currL1Number = num

			w.lgr.Trace("notifying subscribers of new number", "number", num)
			w.mtx.RLock()
			for _, sub := range w.subs {
				sub <- head
			}
			w.mtx.RUnlock()
			reqPollAfter()
		case <-w.ctx.Done():
			w.c1.Close()
			return
		}
	}
}

func (w *RandaoClient) getLatestNumber() (*big.Int, error) {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	h := crypto.Keccak256Hash([]byte("number()"))
	ret, err := w.c2.CallContract(ctx, ethereum.CallMsg{
		From: common.Address{},
		To:   &w.l1BlockContract,
		Data: h[0:4],
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call number() %v", err)
	}
	// L1Block contract cannot get latest blockhash
	curBlock := new(big.Int).Sub(new(big.Int).SetBytes(ret), common.Big1)
	return curBlock, nil
}

func (w *RandaoClient) HeaderByNumber(ctx context.Context, blockNumber *big.Int) (*types.Header, error) {
	header, err := w.c1.HeaderByNumber(ctx, blockNumber)
	if err != nil {
		return nil, err
	}
	w.lgr.Debug("fetching header by number", "number", blockNumber, "header", header)
	return w.c1.HeaderByNumber(ctx, blockNumber)
}

func (w *RandaoClient) reqPoll() {
	w.pollReqCh <- struct{}{}
}
