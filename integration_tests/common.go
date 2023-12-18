// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package integration

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

const (
	pkName       = "ES_NODE_SIGNER_PRIVATE_KEY"
	l1Endpoint   = "http://65.109.115.36:8545"
	clefEndpoint = "http://65.108.236.27:8550"
)

var (
	contractAddr1GB     = common.HexToAddress("0x24d29453B4B4917117B4af022Ad1B0a9aBc2c7DE")
	contractAddrDevnet2 = common.HexToAddress("0xb4B46bdAA835F8E4b4d8e208B6559cD267851051")
	minerAddr           = common.HexToAddress("0x534632D6d7aD1fe5f832951c97FDe73E4eFD9a77")
	value               = "1000000000000000"
	lg                  = esLog.NewLogger(esLog.DefaultCLIConfig())
)

func initStorageConfig(ctx context.Context, client *ethclient.Client, l1Contract, miner common.Address) (*storage.StorageConfig, error) {
	maxKvSizeBits, err := readUintFromContract(ctx, client, l1Contract, "maxKvSizeBits")
	if err != nil {
		return nil, err
	}
	chunkSizeBits := maxKvSizeBits
	shardEntryBits, err := readUintFromContract(ctx, client, l1Contract, "shardEntryBits")
	if err != nil {
		return nil, err
	}
	return &storage.StorageConfig{
		L1Contract:        l1Contract,
		Miner:             miner,
		KvSize:            1 << maxKvSizeBits,
		ChunkSize:         1 << chunkSizeBits,
		KvEntriesPerShard: 1 << shardEntryBits,
	}, nil
}

func readSlotFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) ([]byte, error) {
	h := crypto.Keccak256Hash([]byte(fieldName + "()"))
	msg := ethereum.CallMsg{
		To:   &l1Contract,
		Data: h[0:4],
	}
	bs, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s from contract: %v", fieldName, err)
	}
	return bs, nil
}

func readUintFromContract(ctx context.Context, client *ethclient.Client, l1Contract common.Address, fieldName string) (uint64, error) {
	bs, err := readSlotFromContract(ctx, client, l1Contract, fieldName)
	if err != nil {
		return 0, err
	}
	value := new(big.Int).SetBytes(bs).Uint64()
	lg.Info("Read uint from contract", "field", fieldName, "value", value)
	return value, nil
}

func callVerify(calldata []byte) error {
	ctx := context.Background()
	client, err := ethclient.DialContext(ctx, l1Endpoint)
	if err != nil {
		lg.Error("Failed to connect to the Ethereum client", "error", err)
		return err
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
