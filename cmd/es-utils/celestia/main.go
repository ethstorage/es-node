package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/celestia/blobmgr"
	esLog "github.com/ethstorage/go-ethstorage/ethstorage/log"
	"github.com/urfave/cli"
	"golang.org/x/term"
)

var (
	l log.Logger
)

const (
	BlobFileFlagName   = "blob-file"
	HeightFlagName     = "height"
	commitmentFlagName = "commitment"
)

func init() {
	l = esLog.NewLogger(esLog.CLIConfig{
		Level:  "debug",
		Format: "terminal",
		Color:  term.IsTerminal(int(os.Stdout.Fd())),
	})

}
func main() {

	app := cli.NewApp()
	app.Name = "Celestia blob utils"
	app.Usage = "A simple command line app to upload blobs to Celestia"
	app.Flags = blobmgr.CLIFlags()
	app.Commands = []cli.Command{
		{
			Name:  "upload",
			Usage: "Upload a blob to Celestia",
			Flags: append(
				app.Flags,
				cli.StringFlag{
					Name:  BlobFileFlagName,
					Usage: "File path to read blob data",
				}),
			Action: func(c *cli.Context) error {

				// config blobmgr
				cfg := blobmgr.ReadCLIConfig(c)
				l.Info("Starting blob uploader", "config", cfg)
				bmgr, err := blobmgr.NewBlobManager(l, cfg)
				if err != nil {
					l.Crit("Failed to init tx manager", "err", err)
				}

				// get data
				filePath := c.String(BlobFileFlagName)
				data, err := readFile(filePath)
				if err != nil {
					l.Crit("Failed to read blob file", "path", filePath, "err", err)
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
				value := big.NewInt(2000000000000)
				hash, err := bmgr.PublishBlob(context.Background(), key.Bytes(), com, uint64(len((data))), height, value)
				if err != nil {
					l.Crit("Failed to publish blob info", "txHash", hash, "err", err)
				}
				l.Info("Successfully published blob info", "key", key.String(), "height", height, "commitment", hex.EncodeToString(com))
				return nil
			},
		},
		{
			Name:  "download",
			Usage: "Download a blob from Celestia",
			Flags: append(
				app.Flags,
				cli.Uint64Flag{
					Name:  HeightFlagName,
					Usage: "Height",
				},
				cli.StringFlag{
					Name:  commitmentFlagName,
					Usage: "Commitment",
				}),
			Action: func(c *cli.Context) error {
				cfg := blobmgr.ReadCLIConfig(c)
				l.Info("Starting blob downloader", "config", cfg)
				bmgr, err := blobmgr.NewBlobDownloader(l, cfg)
				if err != nil {
					l.Crit("Failed to init tx manager", "err", err)
				}

				height := c.Uint64(HeightFlagName)
				commitment, err := hex.DecodeString(c.String(commitmentFlagName))
				if err != nil {
					l.Crit("Failed to decode commitment", "err", err)
				}
				// get blob from Celestia
				data, err := bmgr.GetBlob(context.Background(), commitment, height)
				if err != nil {
					l.Crit("Failed to get blob", "err", err)
				}
				l.Info("Successfully downloaded blob", "data", string(data))
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
