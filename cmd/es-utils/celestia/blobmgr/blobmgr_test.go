package blobmgr

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"golang.org/x/term"
)

func TestSend(t *testing.T) {
	l := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "terminal",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
	cfg := CLIConfig{
		DaRpc:          "http://65.108.236.27:26658",
		NamespaceId:    "00000000000000003333",
		AuthToken:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.MaHtzm_HvBw810jMsd1Vr4bz1f4oAMPZExRNsOJ9n1g",
		L1RPCURL:       "http://65.109.115.36:32813",
		L1Contract:     "0x878705ba3f8Bc32FCf7F4CAa1A35E72AF65CF766",
		PrivateKey:     "7da08f856b5956d40a72968f93396f6acff17193f013e8053f6fbb6c08c194d6",
		NetworkTimeout: 30 * time.Second,
	}

	type gen func(*testing.T, int) []byte
	tests := []gen{
		generateSequentialBytes,
		readTxt,
	}

	txManager, err := NewBlobManager(l, cfg)
	if err != nil {
		t.Fatalf("Failed to init tx mgr %v", err)
	}

	for _, tt := range tests {
		data := tt(t, 10*32)
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			cmt, ht, err := txManager.SendBlob(ctx, data)
			if err != nil {
				t.Errorf("SimpleTxManager.send() error = %v", err)
				return
			}
			t.Log("upload done", hex.EncodeToString(cmt), ht)
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
