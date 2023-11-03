package blobmgr

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"golang.org/x/term"
)

func TestUploadDownload(t *testing.T) {
	l := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "terminal",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
	cfg := CLIConfig{
		DaRpc:          "http://65.108.236.27:26658",
		NamespaceId:    "00000000000000003333",
		AuthToken:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.MaHtzm_HvBw810jMsd1Vr4bz1f4oAMPZExRNsOJ9n1g",
		L1RPCURL:       "http://65.108.236.27:8545",
		L1Contract:     "0xd8767d1C4226326b695A95E46a79Abd724eb8690",
		PrivateKey:     "95eb6ffd2ae0b115db4d1f0d58388216f9d026896696a5211d77b5f14eb5badf",
		NetworkTimeout: 30 * time.Second,
	}

	type gen func(*testing.T, int) []byte
	tests := []gen{
		generateSequentialBytes,
		// readTxt,
	}

	bmgr, err := NewBlobManager(l, cfg)
	if err != nil {
		t.Fatalf("Failed to init tx mgr %v", err)
	}

	for _, tt := range tests {
		data := tt(t, 10*32)
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			com, height, err := bmgr.SendBlob(ctx, data)
			if err != nil {
				t.Fatalf("SendBlob error = %v", err)
			}
			t.Logf("upload done commit %x, height %d, size %d", com, height, len((data)))
			// create blob key
			keySource := make([]byte, 8)
			binary.LittleEndian.PutUint64(keySource, height)
			keySource = append(keySource, com...)
			key := crypto.Keccak256Hash(keySource)

			// publish blob info to Ethereum
			value := big.NewInt(1000000000000000)
			txHash, err := bmgr.PublishBlob(context.Background(), key.Bytes(), com, uint64(len((data))), height, value)
			if err != nil {
				t.Fatalf("PublishBlob error = %v", err)
			}

			// get height and commitment from event
			receipt, err := bmgr.Backend.TransactionReceipt(context.Background(), txHash)
			if err != nil {
				t.Fatalf("TransactionReceipt error = %v", err)
			}
			block := receipt.BlockNumber
			eventSig := []byte("BlobPublished(uint256,uint256,bytes32,uint256)")
			query := ethereum.FilterQuery{
				Addresses: []common.Address{bmgr.L1Contract},
				Topics:    [][]common.Hash{{crypto.Keccak256Hash(eventSig)}},
				FromBlock: block,
				ToBlock:   block,
			}
			logs, err := bmgr.Backend.FilterLogs(context.Background(), query)
			if err != nil {
				t.Fatalf("FilterLogs error = %v", err)
			}
			if len(logs) == 0 {
				t.Fatalf("No event found.")
			}
			eventTopics := logs[0].Topics
			blobHeight := new(big.Int).SetBytes(eventTopics[2][:]).Uint64()
			commitment := eventTopics[3].Bytes()
			t.Logf("Got blob info from storage contract: height %d, commitment=%x \n", blobHeight, commitment)

			// retrieve blob from Celestia
			downloaded, err := bmgr.GetBlob(context.Background(), commitment, blobHeight)
			if err != nil {
				t.Fatalf("GetBlob error=%v, height=%d", err, blobHeight)
			}

			// compare the blobs
			t.Log("Comparing blobs")
			if crypto.Keccak256Hash(downloaded) != crypto.Keccak256Hash(data) {
				t.Error("data mismatch")
				fmt.Println("uploaded", string(data))
				fmt.Println("downloaded", string(downloaded))
			}
		})
	}
}

func generateSequentialBytes(t *testing.T, n int) []byte {
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		data[i] = byte(i % 32)
	}
	return data
}

func readTxt(t *testing.T, n int) []byte {
	resp, err := http.Get("https://www.gutenberg.org/cache/epub/11/pg11.txt")
	if err != nil {
		t.Fatal("read txt error")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("read txt error")
	}
	return data[:n]
}
