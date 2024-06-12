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
	l1Endpoint = os.Getenv("ES_NODE_L1_ETH_RPC")
	l1Contract = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_L1CONTRACT"))
	privateKey = os.Getenv("ES_NODE_SIGNER_PRIVATE_KEY")
	minerAddr  = common.HexToAddress(os.Getenv("ES_NODE_STORAGE_MINER"))
	lg         = esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
)

func callVerify(calldata []byte, contract common.Address) error {
	lg.Info("Verifying against contract", "contract", contract)
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		lg.Error("Failed to connect to the Ethereum client", "error", err)
		return err
	}
	defer client.Close()
	msg := ethereum.CallMsg{
		From: common.HexToAddress("0x0000000000000000000000000000000000000001"),
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
