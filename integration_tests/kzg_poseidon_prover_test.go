// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
)

const (
	prPath   = "../ethstorage/prover"
	zkeyFile = "blob_poseidon.zkey"
)

func TestKZGPoseidonProver_GenerateZKProofs(t *testing.T) {
	type args struct {
		encodingKeys []common.Hash
		chunkIdxes   []uint64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "test 1",
			args: args{
				[]common.Hash{
					common.HexToHash("0x1"),
					common.HexToHash("0x22222222222"),
					common.HexToHash("0x1e88fb83944b20562a100533d0521b90bf7df7cc6e0aaa1c46482b67c7b370ab"),
				},
				[]uint64{0, 2222, 4095},
			},
			wantErr: false,
		},
	}

	proverPath, _ := filepath.Abs(prPath)
	prv := prover.NewKZGPoseidonProver(proverPath, zkeyFile, esLog.NewLogger(esLog.DefaultCLIConfig()))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proofs, masks, err := prv.GenerateZKProofs(tt.args.encodingKeys, tt.args.chunkIdxes)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateZKProofs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for i := 0; i < len(tt.args.chunkIdxes); i++ {
				err = verifyDecodeSample(proofs[i], tt.args.chunkIdxes[i], tt.args.encodingKeys[i], masks[i])
				if (err != nil) != tt.wantErr {
					t.Errorf("ZKProver.GenerateZKProofs() %d decodeSample err: %v", i, err)
				}
			}
		})
	}
}

func init() {
	proverPath, _ := filepath.Abs(prPath)
	zkeyFile := filepath.Join(proverPath, "snarkjs", zkeyFile)
	fileID := "1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
	if _, err := os.Stat(zkeyFile); os.IsNotExist(err) {
		fmt.Printf("%s not found, start downloading...\n", zkeyFile)
		cookieCmd := exec.Command("curl", "-c", "./cookie", "-s", "-L", fmt.Sprintf("https://drive.google.com/uc?export=download&id=%s", fileID))
		cookieOutput, err := cookieCmd.Output()
		if err != nil {
			fmt.Println("Error downloading file:", err)
			panic(err)
		}
		confirmCode := strings.TrimPrefix(strings.TrimSpace(string(cookieOutput)), "confirm=")
		downloadCmd := exec.Command("curl", "-Lb", "./cookie", fmt.Sprintf("https://drive.google.com/uc?export=download&confirm=%s&id=%s", confirmCode, fileID))
		downloadOutput, err := downloadCmd.StdoutPipe()
		if err != nil {
			fmt.Println("Error downloading file:", err)
			panic(err)
		}
		zkeyFileWriter, _ := os.Create(zkeyFile)
		defer zkeyFileWriter.Close()

		if err := downloadCmd.Start(); err != nil {
			fmt.Println("Error downloading file:", err)
			panic(err)
		}

		_, _ = io.Copy(zkeyFileWriter, downloadOutput)

		if err := downloadCmd.Wait(); err != nil {
			fmt.Println("Error downloading file:", err)
		}
	}
}
