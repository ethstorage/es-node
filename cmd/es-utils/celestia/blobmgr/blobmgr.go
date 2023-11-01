package blobmgr

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"
	openrpcns "github.com/rollkit/celestia-openrpc/types/namespace"
	"github.com/rollkit/celestia-openrpc/types/share"
)

type BlobManager struct {
	cfg     Config
	chainID *big.Int

	daClient  *openrpc.Client
	namespace openrpcns.Namespace

	backend ETHBackend
	l       log.Logger
}

func NewBlobManager(l log.Logger, cfg CLIConfig) (*BlobManager, error) {
	conf := Config{NetworkTimeout: cfg.NetworkTimeout}
	// TODO uncomment
	// conf, err := NewConfig(l, cfg)
	// if err != nil {
	// 	return nil, err
	// }
	daClient, err := openrpc.NewClient(context.Background(), cfg.DaRpc, cfg.AuthToken)
	if err != nil {
		l.Error("init DA client failed", "rpc", cfg.DaRpc, "err", err)
		return nil, err
	}

	if cfg.NamespaceId == "" {
		return nil, errors.New("namespace id cannot be blank")
	}
	nsBytes, err := hex.DecodeString(cfg.NamespaceId)
	if err != nil {
		return nil, err
	}

	namespace, err := share.NewBlobNamespaceV0(nsBytes)
	if err != nil {
		return nil, err
	}

	return &BlobManager{
		chainID:   conf.ChainID,
		cfg:       conf,
		daClient:  daClient,
		namespace: namespace.ToAppNamespace(),
		backend:   conf.Backend,
		l:         l,
	}, nil
}

func (m *BlobManager) SendBlob(ctx context.Context, data []byte) ([]byte, uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.NetworkTimeout)
	defer cancel()

	dataBlob, err := blob.NewBlobV0(m.namespace.Bytes(), data)
	com, err := blob.CreateCommitment(dataBlob)
	if err != nil {
		m.l.Warn("Unable to create blob commitment to Celestia", "err", err)
		return nil, 0, err
	}
	m.l.Debug("Create commitment", "commitment", hex.EncodeToString(com))
	err = m.daClient.Header.SyncWait(ctx)
	if err != nil {
		m.l.Warn("Unable to wait for Celestia header sync", "err", err)
		return nil, 0, err
	}
	m.l.Debug("Start sending blob", "size", len(data))
	height, err := m.daClient.Blob.Submit(ctx, []*blob.Blob{dataBlob}, nil)
	if err != nil {
		m.l.Warn("Unable to publish tx to Celestia", "err", err)
		return nil, 0, err
	}
	if height == 0 {
		m.l.Warn("unexpected response from Celestia got", "height", height)
		return nil, 0, errors.New("Unexpected response code")
	}
	m.l.Debug("Submit blob to Celestia successfully!", "height", height)
	return com, height, nil
}

func (m *BlobManager) CraftEvmTx(ctx context.Context, commit []byte, height, size uint64) (*types.Transaction, error) {
	// frameRef := FrameRef{
	// 	BlockHeight:  height,
	// 	TxCommitment: com,
	// }

	// frameRefData, _ := frameRef.MarshalBinary()
	// tx, err := m.craftTx(ctx, candidate)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create the tx: %w", err)
	// }
	// return m.sendTx(ctx, tx)
	panic("not implemented")
}

func (m *BlobManager) sendTx(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	panic("not implemented")
}

type FrameRef struct {
	BlockHeight  uint64
	TxCommitment []byte
}

func (f *FrameRef) MarshalBinary() ([]byte, error) {
	ref := make([]byte, 8+len(f.TxCommitment))

	binary.LittleEndian.PutUint64(ref, f.BlockHeight)
	copy(ref[8:], f.TxCommitment)

	return ref, nil
}

// ETHBackend is the set of methods that the transaction manager uses to resubmit gas & determine
// when transactions are included on L1.
type ETHBackend interface {
	BlockNumber(ctx context.Context) (uint64, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
}
