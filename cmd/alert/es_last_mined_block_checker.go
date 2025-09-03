package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/pkg/errors"
)

const (
	noMinedBlockAlertContent = "<p><b>Alert: </b>%s </p><p><b>Message: </b>No blocks mined in last 24 hours. Last mined block: %d; last mined time: %v; source: %s.</p>"
)

type ESLastMinedBlockChecker struct {
	Name     string         `json:"name"`
	EmailTo  string         `json:"email-to"`
	Contract common.Address `json:"contract"`
	RPC      string         `json:"rpc"`
}

func newESLastMinedBlockChecker(params map[string]string, emailTo string) (*ESLastMinedBlockChecker, error) {
	name, contract, rpc := params["name"], params["contract"], params["rpc"]
	if name == "" || contract == "" || rpc == "" {
		return nil, errors.New("invalid params to load ESLastMinedBlockChecker")
	}

	return &ESLastMinedBlockChecker{
		Name:     name,
		EmailTo:  emailTo,
		Contract: common.HexToAddress(contract),
		RPC:      rpc,
	}, nil
}

func (c *ESLastMinedBlockChecker) Check(lg log.Logger) (bool, string, string) {
	client, err := eth.Dial(c.RPC, c.Contract, 12, lg)
	if err != nil {
		lg.Error("Failed to create source", "alert", c.Name, "err", err)
		return true, fmt.Sprintf(errorContent, c.Name, err.Error()), c.EmailTo
	}
	var (
		ctx = context.Background()
		api = miner.NewL1MiningAPI(client, nil, lg)
	)
	for i := 0; i < 3; i++ {
		info, e := api.GetMiningInfo(ctx, c.Contract, 0)
		if e != nil {
			time.Sleep(time.Minute)
			lg.Error("Get mining info fail", "alert", c.Name, "error", e)
			err = e
			continue
		}

		lastMinedTime := time.Unix(int64(info.LastMineTime), 0)
		lg.Info("Check last mined block", "alert", c.Name, "time", lastMinedTime, "mined block", info.BlockMined, "rpc", c.RPC)
		targetTime := time.Now().Add(-24 * time.Hour)
		if targetTime.After(lastMinedTime) {
			content := fmt.Sprintf(noMinedBlockAlertContent, c.Name, info.BlockMined, lastMinedTime, c.RPC)
			return true, content, c.EmailTo
		}
		return false, "", c.EmailTo
	}

	return true, fmt.Sprintf(errorContent, c.Name, err.Error()), c.EmailTo
}
