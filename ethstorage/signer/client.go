package signer

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type SignerClient struct {
	client *rpc.Client
	status string
}

type signTransactionResult struct {
	Raw hexutil.Bytes      `json:"raw"`
	Tx  *types.Transaction `json:"tx"`
}

func NewSignerClient(endpoint string) (*SignerClient, error) {

	rpcClient, err := rpc.DialOptions(context.Background(), endpoint, rpc.WithHTTPClient(http.DefaultClient))
	if err != nil {
		return nil, err
	}

	signer := &SignerClient{client: rpcClient}
	// Check if reachable
	version, err := signer.pingVersion()
	if err != nil {
		return nil, err
	}
	signer.status = fmt.Sprintf("ok [version=%v]", version)
	fmt.Println("signer version", signer.status)
	return signer, nil
}

func (s *SignerClient) pingVersion() (string, error) {
	var v string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	if err := s.client.CallContext(ctx, &v, "account_version"); err != nil {
		return "", err
	}
	return v, nil
}

func (s *SignerClient) SignTransaction(ctx context.Context, chainId *big.Int, from common.Address, tx *types.Transaction) (*types.Transaction, error) {
	args := NewTransactionArgsFromTransaction(chainId, from, tx)
	signed := &signTransactionResult{}
	if err := s.client.CallContext(ctx, &signed, "account_signTransaction", args); err != nil {
		return nil, fmt.Errorf("account_signTransaction failed: %w", err)
	}
	return signed.Tx, nil
}
