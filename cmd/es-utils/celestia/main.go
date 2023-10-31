package main

import (
	"fmt"
	"os"

	"github.com/ethstorage/go-ethstorage/cmd/es-utils/celestia/txmgr"
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()
	app.Name = "Celestia blob uploader"
	app.Usage = "A simple command line app to upload blobs to Celestia"
	app.Flags = txmgr.CLIFlags()
	app.Commands = []cli.Command{
		{
			Name:  "greet",
			Usage: "Say a greeting",
			Action: func(c *cli.Context) error {
				fmt.Println("Greetings!")
				return nil
			},
		},
		{
			Name:  "upload",
			Usage: "Upload a blob to Celestia",
			Action: func(c *cli.Context) error {
				fmt.Println("Uploading")
				return nil
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
