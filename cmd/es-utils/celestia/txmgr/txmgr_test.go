package txmgr

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"golang.org/x/term"
)

func TestSend(t *testing.T) {
	l := esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "text",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})
	cfg := CLIConfig{
		DaRpc:       "http://65.108.236.27:26658",
		NamespaceId: "00000000000000003333",
		AuthToken:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.MaHtzm_HvBw810jMsd1Vr4bz1f4oAMPZExRNsOJ9n1g",
		L1RPCURL:    "http://65.109.115.36:32813",
		PrivateKey:  "7da08f856b5956d40a72968f93396f6acff17193f013e8053f6fbb6c08c194d6",
	}
	tests := []struct {
		data    []byte
		want    *types.Receipt
		wantErr bool
	}{{
		[]byte{0x01, 0x02, 0x03}, nil, false,
	},
	}

	txManager, err := NewSimpleTxManager(l, cfg)
	if err != nil {
		t.Fatalf("Failed to init tx mgr %v", err)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			got, err := txManager.send(ctx, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("SimpleTxManager.send() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SimpleTxManager.send() = %v, want %v", got, tt.want)
			}
		})
	}
}
