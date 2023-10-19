package eth

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

// HeadSignalFn is used as callback function to accept head-signals
type HeadSignalFn func(ctx context.Context, sig L1BlockRef)

type NewHeadSource interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
}

// WatchHeadChanges wraps a new-head subscription from NewHeadSource to feed the given Tracker
func WatchHeadChanges(ctx context.Context, src NewHeadSource, fn HeadSignalFn) (ethereum.Subscription, error) {
	headChanges := make(chan *types.Header, 10)
	sub, err := src.SubscribeNewHead(ctx, headChanges)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case header := <-headChanges:
				fn(ctx, L1BlockRef{
					Hash:       header.Hash(),
					Number:     header.Number.Uint64(),
					ParentHash: header.ParentHash,
					Time:       header.Time,
				})
			case err := <-sub.Err():
				return err
			case <-ctx.Done():
				return ctx.Err()
			case <-quit:
				return nil
			}
		}
	}), nil
}

// PollBlockChanges opens a polling loop to fetch the L1 block reference with the given label,
// on provided interval and with request timeout. Results are returned with provided callback fn,
// which may block to pause/back-pressure polling.
func PollBlockChanges(ctx context.Context, log log.Logger, src *PollingClient, fn HeadSignalFn,
	label rpc.BlockNumber, interval time.Duration, timeout time.Duration) ethereum.Subscription {
	return event.NewSubscription(func(quit <-chan struct{}) error {
		if interval <= 0 {
			log.Warn("Polling of block is disabled", "interval", interval, "label", label)
			<-quit
			return nil
		}
		getBlockByLabel := func() {
			reqCtx, reqCancel := context.WithTimeout(ctx, timeout)
			ref, err := L1BlockRefByLabel(src, reqCtx, label)
			reqCancel()
			if err != nil {
				log.Warn("Failed to poll L1 block", "label", label, "err", err)
			} else {
				fn(ctx, ref)
			}
		}
		getBlockByLabel()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				getBlockByLabel()
			case <-ctx.Done():
				return ctx.Err()
			case <-quit:
				return nil
			}
		}
	})
}

// L1BlockRefByLabel returns the [eth.L1BlockRef] for the given block label.
// Notice, we cannot cache a block reference by label because labels are not guaranteed to be unique.
func L1BlockRefByLabel(src *PollingClient, ctx context.Context, label rpc.BlockNumber) (L1BlockRef, error) {
	info, err := src.BlockByNumber(ctx, big.NewInt(label.Int64()))
	if err != nil {
		// Both geth and erigon like to serve non-standard errors for the safe and finalized heads, correct that.
		// This happens when the chain just started and nothing is marked as safe/finalized yet.
		if strings.Contains(err.Error(), "block not found") || strings.Contains(err.Error(), "Unknown block") {
			err = ethereum.NotFound
		}
		return L1BlockRef{}, fmt.Errorf("failed to fetch head header: %w", err)
	}
	ref := InfoToL1BlockRef(info)
	return ref, nil
}
