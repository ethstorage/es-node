package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
)

const (
	emailFormat              = "<html><body><div><h3>Ethstorage Alert!</h3>%s</div></body></html>"
	noMinedBlockAlertContent = "<p>No blocks mined in last 24 hours. Last mined block %d, last mined time: %v.</p>"
	errorContent             = "<p>Check alert fail with error: %s</p>"
)

var (
	rpcURLFlag     = flag.String("rpcurl", "http://88.99.30.186:8545", "L1 RPC URL")
	l1ContractFlag = flag.String("l1contract", "0x804C520d3c084C805E37A35E90057Ac32831F96f", "Storage contract address on l1")
	bodyFileFlag   = flag.String("htmlbody", "body.html", "Alert email html body file")
)

func main() {
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(3), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	var (
		contract = common.HexToAddress(*l1ContractFlag)
		logger   = log.New("app", "alert")
	)

	client, err := eth.Dial(*rpcURLFlag, contract, 12, logger)
	if err != nil {
		log.Crit("Failed to create L1 source", "err", err)
	}

	needAlert, content := checkLastMinedBlock(client, contract, logger)
	if needAlert {
		writeHtmlFile(fmt.Sprintf(emailFormat, content))
		os.Exit(1)
	}
}

func checkLastMinedBlock(client *eth.PollingClient, contract common.Address, logger log.Logger) (bool, string) {
	var (
		ctx = context.Background()
		api = miner.NewL1MiningAPI(client, nil, logger)
		err error
	)

	for i := 0; i < 3; i++ {
		info, e := api.GetMiningInfo(ctx, contract, 0)
		if e != nil {
			time.Sleep(time.Minute)
			log.Error("Get mining info fail", "error", e)
			err = e
			continue
		}

		lastMinedTime := time.Unix(int64(info.LastMineTime), 0)
		logger.Info("", "last mined time", lastMinedTime, "mined block", info.BlockMined)
		targetTime := time.Now().Add(-1 * time.Hour)
		if targetTime.After(lastMinedTime) {
			content := fmt.Sprintf(noMinedBlockAlertContent, info.BlockMined, lastMinedTime)
			return true, content
		}
		return false, ""
	}

	return true, fmt.Sprintf(errorContent, err.Error())
}

func writeHtmlFile(content string) {
	file, err := os.Create(*bodyFileFlag)
	if err != nil {
		log.Crit("Create html file fail", "error", err.Error())
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		log.Crit("Write html file fail", "error", err.Error())
	}
}
