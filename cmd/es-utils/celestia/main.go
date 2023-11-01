package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/celestia/blobmgr"
	"github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/urfave/cli"
	"golang.org/x/term"
)

func main() {

	app := cli.NewApp()
	app.Name = "Celestia blob utils"
	app.Usage = "A simple command line app to upload blobs to Celestia"
	app.Commands = []cli.Command{
		{
			Name:  "upload",
			Usage: "Upload a blob to Celestia",
			Flags: blobmgr.CLIFlags(),
			Action: func(c *cli.Context) error {

				// config blobmgr
				l := log.NewLogger(log.CLIConfig{
					Level:  "debug",
					Format: "terminal",
					Color:  term.IsTerminal(int(os.Stdout.Fd())),
				})
				cfg := blobmgr.ReadCLIConfig(c)
				l.Info("Starting blob uploader", "config", cfg)
				bmgr, err := blobmgr.NewBlobManager(l, cfg)
				if err != nil {
					l.Crit("Failed to init tx manager", "err", err)
				}

				// get data
				data, err := readFile(cfg.BlobFilePath)
				if err != nil {
					l.Crit("Failed to read blob file", "err", err)
				}

				// send blob to Celestia
				com, height, err := bmgr.SendBlob(context.Background(), data)
				if err != nil {
					l.Crit("Failed to send blob", "err", err)
				}
				l.Info("Successfully uploaded blob", "height", height, "commitment", hex.EncodeToString(com))

				// create blob key
				keySource := make([]byte, 8)
				binary.LittleEndian.PutUint64(keySource, height)
				keySource = append(keySource, com...)
				key := crypto.Keccak256Hash(keySource)

				// publish blob info to Ethereum
				value := big.NewInt(10000000000000)
				err = bmgr.PublishBlob(context.Background(), key.Bytes(), com, uint64(len((data))), height, value)
				if err != nil {
					l.Crit("Failed to publish blob info", "err", err)
				}
				l.Info("Successfully published blob info", "key", key.String(), "height", height, "commitment", hex.EncodeToString(com))
				return nil
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func readFile(filePath string) ([]byte, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get the file size
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()

	// Allocate a buffer to store the file contents
	buffer := make([]byte, fileSize)

	// Read the file contents into the buffer
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}

	return buffer, nil
}
