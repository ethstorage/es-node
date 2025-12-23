package eth

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"math/big"
	"regexp"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"golang.org/x/mod/semver"
)

const (
	PutBlobEvent    = "PutBlob(uint256,uint256,bytes32)"
	MinedBlockEvent = "MinedBlock(uint256,uint256,uint256,uint256,address,uint256)"
)

var httpRegex = regexp.MustCompile("^http(s)?://")
var ErrSubscriberClosed = errors.New("subscriber closed")

type PollingClient struct {
	*ethclient.Client
	isHTTP      bool
	lg          log.Logger
	pollRate    time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	currHead    *types.Header
	esContract  common.Address
	subID       int
	NetworkID   *big.Int
	queryHeader func() (*types.Header, error)

	// pollReqCh is used to request new polls of the upstream
	// RPC client.
	pollReqCh chan struct{}

	mtx sync.RWMutex

	subs map[int]chan *types.Header

	closedCh chan struct{}
}

// Dial connects a client to the given URL.
func Dial(rawurl string, esContract common.Address, pollRate uint64, lg log.Logger) (*PollingClient, error) {
	return DialContext(context.Background(), rawurl, esContract, pollRate, lg)
}

func DialContext(ctx context.Context, rawurl string, esContract common.Address, pollRate uint64, lg log.Logger) (*PollingClient, error) {
	c, err := ethclient.DialContext(ctx, rawurl)
	if err != nil {
		return nil, err
	}
	return NewClient(ctx, c, httpRegex.MatchString(rawurl), esContract, pollRate, nil, lg), nil
}

// NewClient creates a client that uses the given RPC client.
func NewClient(
	ctx context.Context,
	c *ethclient.Client,
	isHTTP bool,
	esContract common.Address,
	pollRate uint64,
	qh func() (*types.Header, error),
	lg log.Logger,
) *PollingClient {
	ctx, cancel := context.WithCancel(ctx)
	networkID, err := c.NetworkID(ctx)
	if err != nil {
		lg.Crit("Failed to get network id", "err", err)
	}
	res := &PollingClient{
		Client:     c,
		isHTTP:     isHTTP,
		lg:         lg,
		pollRate:   time.Duration(pollRate) * time.Second,
		ctx:        ctx,
		cancel:     cancel,
		esContract: esContract,
		pollReqCh:  make(chan struct{}, 1),
		subs:       make(map[int]chan *types.Header),
		closedCh:   make(chan struct{}),
		NetworkID:  networkID,
	}
	if qh == nil {
		res.queryHeader = res.getLatestHeader
	} else {
		res.queryHeader = qh
	}
	if isHTTP {
		go res.pollHeads()
	}
	return res
}

// Close closes the PollingClient and the underlying RPC client it talks to.
func (w *PollingClient) Close() {
	w.cancel()
	<-w.closedCh
	w.Client.Close()
}

func (w *PollingClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	if !w.isHTTP {
		return w.Client.SubscribeNewHead(ctx, ch)
	}
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

func (w *PollingClient) pollHeads() {
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
			head, err := w.queryHeader()
			if err != nil {
				w.lg.Info("Error getting latest header", "err", err)
				reqPollAfter()
				continue
			}
			if w.currHead != nil && w.currHead.Hash() == head.Hash() {
				w.lg.Trace("No change in head, skipping notifications")
				reqPollAfter()
				continue
			}

			w.lg.Trace("Notifying subscribers of new head", "head", head.Hash())
			w.currHead = head
			w.mtx.RLock()
			for _, sub := range w.subs {
				sub <- head
			}
			w.mtx.RUnlock()
			reqPollAfter()
		case <-w.ctx.Done():
			w.Client.Close()
			return
		}
	}
}

func (w *PollingClient) getLatestHeader() (*types.Header, error) {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()
	latest, err := w.BlockNumber(ctx)
	if err != nil {
		w.lg.Error("Failed to get latest block number", "err", err)
		return nil, err
	}
	// The latest blockhash could be empty
	number := new(big.Int).SetUint64(latest - 1)
	return w.HeaderByNumber(ctx, number)
}

func (w *PollingClient) reqPoll() {
	w.pollReqCh <- struct{}{}
}

func (w *PollingClient) FilterLogsByBlockRange(start *big.Int, end *big.Int, eventSig string) ([]types.Log, error) {
	topic := crypto.Keccak256Hash([]byte(eventSig))

	// create a new filter query
	query := ethereum.FilterQuery{
		Addresses: []common.Address{w.esContract},
		Topics: [][]common.Hash{
			{
				topic,
			},
		},
		FromBlock: start,
		ToBlock:   end,
	}

	// retrieve past events that match the filter query
	return w.FilterLogs(context.Background(), query)
}

func (w *PollingClient) GetStorageKvEntryCount(blockNumber int64) (uint64, error) {
	h := crypto.Keccak256Hash([]byte(`kvEntryCount()`))

	callMsg := ethereum.CallMsg{
		To:   &w.esContract,
		Data: h[:],
	}

	bs, err := w.Client.CallContract(context.Background(), callMsg, new(big.Int).SetInt64(blockNumber))
	if err != nil {
		return 0, err
	}

	uint40Type, _ := abi.NewType("uint40", "", nil)

	res, err := abi.Arguments{
		{Type: uint40Type},
	}.UnpackValues(bs)

	if err != nil {
		return 0, err
	}

	return res[0].(*big.Int).Uint64(), nil
}

func (w *PollingClient) GetKvMetas(kvIndices []uint64, blockNumber int64) ([][32]byte, error) {
	// TODO: @Qiang need to implement this view function to get multiple hash at once
	h := crypto.Keccak256Hash([]byte(`getKvMetas(uint256[])`))

	indices := make([]*big.Int, len(kvIndices))
	for i, num := range kvIndices {
		indices[i] = new(big.Int).SetUint64(num)
	}

	uint256Array, _ := abi.NewType("uint256[]", "", nil)
	dataField, err := abi.Arguments{
		{Type: uint256Array},
	}.Pack(indices)
	if err != nil {
		return nil, err
	}

	calldata := append(h[0:4], dataField...)
	callMsg := ethereum.CallMsg{
		To:   &w.esContract,
		Data: calldata,
	}

	bs, err := w.Client.CallContract(context.Background(), callMsg, new(big.Int).SetInt64(blockNumber))
	if err != nil {
		return nil, err
	}

	bytes32Array, _ := abi.NewType("bytes32[]", "", nil)

	res, err := abi.Arguments{
		{Type: bytes32Array},
	}.UnpackValues(bs)

	if err != nil {
		return nil, err
	}

	if len(res[0].([][32]byte)) != len(kvIndices) {
		return nil, errors.New("invalid return from GetKvMetas")
	}

	return res[0].([][32]byte), nil
}

func (w *PollingClient) GetMiningReward(shard uint64, timestamp uint64) (*big.Int, error) {
	h := crypto.Keccak256Hash([]byte(`miningReward(uint256,uint256)`))
	uint256Type, _ := abi.NewType("uint256", "", nil)
	dataField, err := abi.Arguments{
		{Type: uint256Type},
		{Type: uint256Type},
	}.Pack(new(big.Int).SetUint64(shard), new(big.Int).SetUint64(timestamp))
	if err != nil {
		return nil, err
	}
	calldata := append(h[0:4], dataField...)
	callMsg := ethereum.CallMsg{
		To:   &w.esContract,
		Data: calldata,
	}
	bs, err := w.Client.CallContract(context.Background(), callMsg, nil)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(bs), nil
}

func (w *PollingClient) ReadContractField(fieldName string, blockNumber *big.Int) ([]byte, error) {
	h := crypto.Keccak256Hash([]byte(fieldName + "()"))
	msg := ethereum.CallMsg{
		To:   &w.esContract,
		Data: h[0:4],
	}
	bs, err := w.Client.CallContract(context.Background(), msg, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s from contract: %v", fieldName, err)
	}
	return bs, nil
}

func (w *PollingClient) ReadContractBigIntField(fieldName string, blockNumber *big.Int) (*big.Int, error) {
	bs, err := w.ReadContractField(fieldName, blockNumber)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(bs), nil
}

func (w *PollingClient) ReadContractUint64Field(fieldName string, blockNumber *big.Int) (uint64, error) {
	bs, err := w.ReadContractField(fieldName, blockNumber)
	if err != nil {
		return 0, err
	}
	return new(big.Int).SetBytes(bs).Uint64(), nil
}

func (w *PollingClient) GetContractVersion() (string, error) {
	bs, err := w.ReadContractField("version", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get version from contract: %v", err)
	}
	versionStr, err := decodeString(bs)
	if err != nil {
		return "", fmt.Errorf("failed to decode version string: %v", err)
	}
	version := "v" + versionStr
	if !semver.IsValid(version) {
		return "", fmt.Errorf("invalid version string: %s", versionStr)
	}
	return version, nil
}

func (w *PollingClient) IsContractExist() (bool, error) {
	code, err := w.CodeAt(w.ctx, w.esContract, nil)
	if err != nil {
		return false, fmt.Errorf("failed to get code at %s: %v", w.esContract.Hex(), err)
	}
	return len(code) > 0, nil
}

func (w *PollingClient) ContractAddress() common.Address {
	return w.esContract
}

func (w *PollingClient) LoadStorageConfigFromContract(miner common.Address) (*storage.StorageConfig, error) {
	exist, err := w.IsContractExist()
	if err != nil {
		return nil, fmt.Errorf("check contract exist fail: %s", err.Error())
	}
	if !exist {
		return nil, fmt.Errorf("contract does not exist")
	}
	maxKvSizeBits, err := w.ReadContractUint64Field("maxKvSizeBits", nil)
	if err != nil {
		return nil, fmt.Errorf("get maxKvSizeBits: %s", err.Error())
	}
	chunkSizeBits := maxKvSizeBits
	shardEntryBits, err := w.ReadContractUint64Field("shardEntryBits", nil)
	if err != nil {
		return nil, fmt.Errorf("get shardEntryBits: %s", err.Error())
	}

	return &storage.StorageConfig{
		L1Contract:        w.esContract,
		Miner:             miner,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}, nil
}

func decodeString(data []byte) (string, error) {
	if len(data) < 64 {
		return "", fmt.Errorf("data too short")
	}
	strLen := new(big.Int).SetBytes(data[32:64]).Int64()
	if int64(len(data)) < 64+strLen {
		return "", fmt.Errorf("data length mismatch")
	}
	strBytes := data[64 : 64+strLen]
	return string(strBytes), nil
}
