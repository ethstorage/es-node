package txmgr

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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

// ETHBackend is the set of methods that the transaction manager uses to resubmit gas & determine
// when transactions are included on L1.
type ETHBackend interface {
	// BlockNumber returns the most recent block number.
	BlockNumber(ctx context.Context) (uint64, error)

	// TransactionReceipt queries the backend for a receipt associated with
	// txHash. If lookup does not fail, but the transaction is not found,
	// nil should be returned for both values.
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)

	// SendTransaction submits a signed transaction to L1.
	SendTransaction(ctx context.Context, tx *types.Transaction) error

	// These functions are used to estimate what the basefee & priority fee should be set to.
	// TODO(CLI-3318): Maybe need a generic interface to support different RPC providers
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	// NonceAt returns the account nonce of the given account.
	// The block number can be nil, in which case the nonce is taken from the latest known block.
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	// PendingNonceAt returns the pending nonce.
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	// EstimateGas returns an estimate of the amount of gas needed to execute the given
	// transaction against the current pending block.
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
}

// SimpleTxManager is a implementation of TxManager that performs linear fee
// bumping of a tx until it confirms.
type SimpleTxManager struct {
	cfg     Config
	chainID *big.Int

	daClient  *openrpc.Client
	namespace openrpcns.Namespace

	backend ETHBackend
	l       log.Logger
}

// NewSimpleTxManager initializes a new SimpleTxManager with the passed Config.
func NewSimpleTxManager(l log.Logger, cfg CLIConfig) (*SimpleTxManager, error) {
	// conf, err := NewConfig(l, cfg)
	// if err != nil {
	// 	return nil, err
	// }
	conf := Config{NetworkTimeout: 60 * time.Second}
	daClient, err := openrpc.NewClient(context.Background(), cfg.DaRpc, cfg.AuthToken)
	if err != nil {
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

	return &SimpleTxManager{
		chainID:   conf.ChainID,
		cfg:       conf,
		daClient:  daClient,
		namespace: namespace.ToAppNamespace(),
		backend:   conf.Backend,
		l:         l,
	}, nil
}

// send performs the actual transaction creation and sending.
func (m *SimpleTxManager) send(ctx context.Context, data []byte) (*types.Receipt, error) {
	fmt.Println("m.cfg.NetworkTimeout", m.cfg.NetworkTimeout)
	ctx, cancel := context.WithTimeout(ctx, m.cfg.NetworkTimeout)
	defer cancel()
	dataBlob, err := blob.NewBlobV0(m.namespace.Bytes(), data)
	com, err := blob.CreateCommitment(dataBlob)
	if err != nil {
		m.l.Warn("unable to create blob commitment to celestia", "err", err)
		return nil, err
	}
	fmt.Printf("commitment: %x", com)
	err = m.daClient.Header.SyncWait(ctx)
	if err != nil {
		m.l.Warn("unable to wait for celestia header sync", "err", err)
		return nil, err
	}
	height, err := m.daClient.Blob.Submit(ctx, []*blob.Blob{dataBlob}, nil)
	if err != nil {
		m.l.Warn("unable to publish tx to celestia", "err", err)
		return nil, err
	}
	fmt.Printf("height: %v\n", height)
	if height == 0 {
		m.l.Warn("unexpected response from celestia got", "height", height)
		return nil, errors.New("unexpected response code")
	}

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
	return nil, nil
}

// send submits the same transaction several times with increasing gas prices as necessary.
// It waits for the transaction to be confirmed on chain.
func (m *SimpleTxManager) sendTx(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {

	// m.l.Debug("Submit mined result", "shard", rst.startShardId, "block", rst.blockNumber, "nonce", rst.nonce)
	// uint256Type, _ := abi.NewType("uint256", "", nil)
	// addrType, _ := abi.NewType("address", "", nil)
	// bytes32Array, _ := abi.NewType("bytes32[]", "", nil)
	// bytesArray, _ := abi.NewType("bytes[]", "", nil)
	// dataField, _ := abi.Arguments{
	// 	{Type: uint256Type},
	// 	{Type: uint256Type},
	// 	{Type: addrType},
	// 	{Type: uint256Type},
	// 	{Type: bytes32Array},
	// 	{Type: bytesArray},
	// }.Pack(
	// 	rst.blockNumber,
	// 	new(big.Int).SetUint64(rst.startShardId),
	// 	rst.miner,
	// 	new(big.Int).SetUint64(rst.nonce),
	// 	rst.encodedData,
	// 	rst.proofs,
	// )
	// h := crypto.Keccak256Hash([]byte(`mine(uint256,uint256,address,uint256,bytes32[],bytes[])`))
	// calldata := append(h[0:4], dataField...)

	// chainID, err := m.NetworkID(ctx)
	// if err != nil {
	// 	m.l.Error("Get chainID failed", "error", err.Error())
	// 	return common.Hash{}, err
	// }
	// sign := cfg.SignerFnFactory(chainID)
	// nonce, err := m.NonceAt(ctx, cfg.SignerAddr, big.NewInt(rpc.LatestBlockNumber.Int64()))
	// if err != nil {
	// 	m.l.Error("Query nonce failed", "error", err.Error())
	// 	return common.Hash{}, err
	// }
	// m.l.Debug("Query nonce done", "nonce", nonce)
	// gasPrice := cfg.GasPrice
	// if gasPrice == nil {
	// 	gasPrice, err = m.SuggestGasPrice(ctx)
	// 	if err != nil {
	// 		m.l.Error("Query gas price failed", "error", err.Error())
	// 		return common.Hash{}, err
	// 	}
	// 	m.l.Debug("Query gas price done", "gasPrice", gasPrice)
	// }
	// tip := cfg.PriorityGasPrice
	// if tip == nil {
	// 	tip, err = m.SuggestGasTipCap(ctx)
	// 	if err != nil {
	// 		m.l.Error("Query gas tip cap failed", "error", err.Error())
	// 		tip = common.Big0
	// 	}
	// 	m.l.Debug("Query gas tip cap done", "gasTipGap", tip)
	// }
	// estimatedGas, err := m.EstimateGas(ctx, ethereum.CallMsg{
	// 	From:      cfg.SignerAddr,
	// 	To:        &contract,
	// 	GasTipCap: tip,
	// 	GasFeeCap: gasPrice,
	// 	Value:     common.Big0,
	// 	Data:      calldata,
	// })
	// if err != nil {
	// 	m.l.Error("Estimate gas failed", "error", err.Error())
	// 	return common.Hash{}, fmt.Errorf("failed to estimate gas: %w", err)
	// }
	// m.l.Info("Estimated gas done", "gas", estimatedGas)
	// gas := uint64(float64(estimatedGas) * gasBufferRatio)
	// rawTx := &types.DynamicFeeTx{
	// 	ChainID:   chainID,
	// 	Nonce:     nonce,
	// 	GasTipCap: tip,
	// 	GasFeeCap: gasPrice,
	// 	Gas:       gas,
	// 	To:        &contract,
	// 	Value:     common.Big0,
	// 	Data:      calldata,
	// }
	// signedTx, err := sign(ctx, cfg.SignerAddr, types.NewTx(rawTx))
	// if err != nil {
	// 	m.l.Error("Sign tx error", "error", err)
	// 	return common.Hash{}, err
	// }
	// err = m.SendTransaction(ctx, signedTx)
	// if err != nil {
	// 	m.l.Error("Send tx failed", "error", err)
	// 	return common.Hash{}, err
	// }
	// m.l.Info("Submit mined result done", "shard", rst.startShardId, "block", rst.blockNumber,
	// 	"nonce", rst.nonce, "txSigner", cfg.SignerAddr.Hex(), "hash", signedTx.Hash().Hex())
	// return signedTx.Hash(), nil
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
