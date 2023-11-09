// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package integration

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
)

const (
	pkName       = "ES_NODE_SIGNER_PRIVATE_KEY"
	l1Endpoint   = "http://65.108.236.27:8545"
	clefEndpoint = "http://65.108.236.27:8550"
)

var (
	contractAddr1GB     = common.HexToAddress("0xE31DbfB4d12B67eE60690Ad8a5877Ce8D77842ED")
	contractAddrDevnet1 = common.HexToAddress("0x9f9F5Fd89ad648f2C000C954d8d9C87743243eC5")
	minerAddr           = common.HexToAddress("0x534632D6d7aD1fe5f832951c97FDe73E4eFD9a77")
	value               = hexutil.EncodeUint64(10000000000000)
	lg                  = esLog.NewLogger(esLog.DefaultCLIConfig())
)

func callVerify(calldata []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
	defer client.Close()
	msg := ethereum.CallMsg{
		To:   &contractAddr1GB,
		Data: calldata,
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return err
	}
	if bs[len(bs)-1] != 1 {
		return fmt.Errorf("false")
	}
	return nil
}

func readFile() ([]byte, error) {
	txt_77k := "https://www.gutenberg.org/files/7266/7266-0.txt"
	resp, err := http.Get(txt_77k)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading blob txtUrl: %v", err)
	}
	return data, nil
}
