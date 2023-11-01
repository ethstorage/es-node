package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"

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
				l := log.NewLogger(log.CLIConfig{
					Level:  "info",
					Format: "terminal",
					Color:  term.IsTerminal(int(os.Stdout.Fd())),
				})
				cfg := blobmgr.ReadCLIConfig(c)
				l.Info("Starting blob uploader", "config", cfg)
				bmgr, err := blobmgr.NewBlobManager(l, cfg)
				if err != nil {
					l.Crit("Failed to init tx manager", "err", err)
				}
				data, err := readFile(cfg.BlobFilePath)
				if err != nil {
					l.Crit("Failed to read blob file", "err", err)
				}
				com, hight, err := bmgr.SendBlob(context.Background(), data)
				if err != nil {
					l.Crit("Failed to send blob", "err", err)
				}
				l.Info("Successfully uploaded blob", "height", hight, "commitment", hex.EncodeToString(com))
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
