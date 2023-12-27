// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package integration

import (
	"context"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"golang.org/x/term"
)

var (
	l1Endpoint       = os.Getenv("ES_NODE_L1_ETH_RPC")
	privateKey       = os.Getenv("ES_NODE_SIGNER_PRIVATE_KEY")
	contractAddr16kv = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_L1CONTRACT"))
	minerAddr        = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_MINER"))

	value = "1000000000000000"
	lg    = esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
)

func callVerify(calldata []byte) error {
	contract := contractAddr16kv
	lg.Info("Verifying against contract", "contract", contract)
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		lg.Error("Failed to connect to the Ethereum client", "error", err)
		return err
	}
	defer client.Close()
	msg := ethereum.CallMsg{
		To:   &contract,
		Data: calldata,
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		lg.Error("Call verify", "err", err.Error())
		return err
	}
	if len(bs) == 0 {
		return fmt.Errorf("no return value")
	}
	if bs[len(bs)-1] != 1 {
		return fmt.Errorf("false")
	}
	return nil
}
