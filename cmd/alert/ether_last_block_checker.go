package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/pkg/errors"
)

const (
	noBlockIn10MinutesAlertContent = "<p><b>Alert: </b>%s </p><p><b>Message: </b>No blocks generated in last 10 minutes. Last block: %d; time: %v; source: %s.</p>"
)

type LastBlockChecker struct {
	Name string `json:"name"`
	RPC  string `json:"rpc"`
}

func newLastBlockChecker(params map[string]string) (*LastBlockChecker, error) {
	name, rpc := params["name"], params["rpc"]
	if name == "" || rpc == "" {
		return nil, errors.New("invalid params to load LastBlockChecker")
	}

	return &LastBlockChecker{
		Name: name,
		RPC:  rpc,
	}, nil
}

func (c *LastBlockChecker) Check(logger log.Logger) (bool, string) {
	ctx := context.Background()
	client, err := ethclient.Dial(c.RPC)
	if err != nil {
		logger.Error("Failed to create L1 source", "alert", c.Name, "err", err)
		return true, fmt.Sprintf(errorContent, c.Name, err.Error())
	}

	for i := 0; i < 3; i++ {
		header, e := client.HeaderByNumber(ctx, nil)
		if e != nil {
			time.Sleep(time.Minute)
			logger.Error("Get block fail", "alert", c.Name, "rpc", c.RPC, "error", e)
			err = e
			continue
		}

		lastMinedTime := time.Unix(int64(header.Time), 0)
		logger.Info("Check last block", "alert", c.Name, "time", lastMinedTime, "block", header.Number, "rpc", c.RPC)
		targetTime := time.Now().Add(-10 * time.Minute)
		if targetTime.After(lastMinedTime) {
			content := fmt.Sprintf(noBlockIn10MinutesAlertContent, c.Name, header.Number, lastMinedTime, c.RPC)
			return true, content
		}
		return false, ""
	}

	return true, fmt.Sprintf(errorContent, c.Name, err.Error())
}
