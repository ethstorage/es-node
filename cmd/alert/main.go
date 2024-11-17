package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/log"
)

const (
	emailFormat  = "<html><body><div><h2>EthStorage Alert!</h2>%s</div></body></html>"
	errorContent = "<p><b>Alert %s: </b>Check alert fail with error: %s</p>"
)

var (
	ruleFileFlag = flag.String("rules", "rules.json", "File contain the rules need to check")
	bodyFileFlag = flag.String("htmlbody", "body.html", "Alert email html body file")
)

type IChecker interface {
	Check(logger log.Logger) (bool, string)
}

type AlertConfig struct {
	AlertType string            `json:"alert-type"`
	Params    map[string]string `json:"params"`
}

func LoadConfig(ruleFile string) []IChecker {
	file, err := os.Open(ruleFile)
	if err != nil {
		log.Crit("Failed to load rule file", "rule file", ruleFile, "err", err)
	}
	defer file.Close()

	var alerts []*AlertConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&alerts); err != nil {
		log.Crit("Failed to decode rule file", "rule file", *ruleFileFlag, "err", err)
	}

	checkers := make([]IChecker, 0)
	for _, alert := range alerts {
		switch {
		case alert.AlertType == "ESLastMinedBlockChecker":
			checker, err := newESLastMinedBlockChecker(alert.Params)
			if err != nil {
				log.Crit("Failed to load ESLastMinedBlockChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		case alert.AlertType == "LastBlockChecker":
			checker, err := newLastBlockChecker(alert.Params)
			if err != nil {
				log.Crit("Failed to load LastBlockChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		case alert.AlertType == "WebsiteOnlineChecker":
			checker, err := newWebsiteOnlineChecker(alert.Params)
			if err != nil {
				log.Crit("Failed to load WebsiteOnlineChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		default:
			log.Crit("Failed to load alert with unknown type", "type", alert.AlertType, "params", alert.Params)
		}
	}

	return checkers
}

/*
func main() {
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(3), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	file, err := os.Create(*ruleFileFlag)
	if err != nil {
		log.Crit("Failed to load rule file", "rule file", *ruleFileFlag, "err", err)
	}
	defer file.Close()

	cs := []*AlertConfig{
		&AlertConfig{
			AlertType: "ESLastMinedBlockChecker",
			Params: map[string]string{
				"Name":     "L2 Last Mined Block Alert",
				"Contract": "0x804C520d3c084C805E37A35E90057Ac32831F96f",
				"RPC":      "http://88.99.30.186:8545",
			},
		},
		&AlertConfig{
			AlertType: "LastBlockChecker",
			Params: map[string]string{
				"Name": "Galileo Last Block Alert",
				"RPC":  "https://galileo.web3q.io:8545",
			},
		},
		&AlertConfig{
			AlertType: "WebsiteOnlineChecker",
			Params: map[string]string{
				"Name": "Galileo explorer online Alert",
				"URL":  "https://explorer.galileo.web3q.io/",
			},
		},
	}
	//
	// checkers := make([]IChecker, 0)
	// checkers = append(checkers, &ESLastMinedBlockChecker{
	// 	Name:     "L2 Last Mined Block Alert",
	// 	Contract: common.HexToAddress("0x804C520d3c084C805E37A35E90057Ac32831F96f"),
	// 	RPC:      "http://88.99.30.186:8545",
	// })
	// checkers = append(checkers, &LastBlockChecker{
	// 	Name: "Galileo Last Block Alert",
	// 	RPC:  "https://galileo.web3q.io:8545",
	// })
	// checkers = append(checkers, &WebsiteOnlineChecker{
	// 	Name: "Galileo explorer online Alert",
	// 	URL:  "https://explorer.galileo.web3q.io/",
	// })

	// decoder := json.NewDecoder(file)
	// if err := decoder.Decode(&checkers); err != nil {
	// 	log.Crit("Failed to decode rule file", "rule file", *ruleFileFlag, "err", err)
	// }

	// Encode data into JSON and write to the file
	// encoder := json.NewEncoder(file)
	// if err := encoder.Encode(checkers); err != nil {
	// 	fmt.Println("Error encoding JSON:", err)
	// 	return
	// }

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(cs); err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}
}*/

func main() {
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(3), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	var (
		logger    = log.New("app", "alert")
		needAlert = false
		contents  = ""
	)

	checkers := LoadConfig(*ruleFileFlag)

	for _, checker := range checkers {
		res, content := checker.Check(logger)
		if res {
			needAlert = res
			contents = contents + content + "\n"
		}
	}

	if needAlert {
		writeHtmlFile(fmt.Sprintf(emailFormat, contents))
		os.Exit(1)
	}
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
