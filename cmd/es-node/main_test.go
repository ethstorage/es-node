// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/flags"
	"github.com/urfave/cli"
)

func TestEsNodeInitRejectsNegativeShardIndex(t *testing.T) {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "init",
			Flags: []cli.Flag{
				cli.IntFlag{Name: encodingTypeFlagName},
				cli.Int64SliceFlag{Name: shardIndexFlagName},
				flags.DataDir,
				flags.L1NodeAddr,
				flags.StorageL1Contract,
			},
			Action: EsNodeInit,
		},
	}

	err := app.Run([]string{
		"es-node", "init",
		"--encoding_type", "0",
		"--shard_index=-1",
		"--datadir", t.TempDir(),
		"--l1.rpc", "invalid://rpc",
		"--storage.l1contract", "0x0000000000000000000000000000000000000001",
	})
	if err == nil || err.Error() != "shard_index must be non-negative: -1" {
		t.Fatalf("expected negative shard index error, got %v", err)
	}
}
