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
	errorContent = "<p><b>Alert: </b>%s </p><p><b>Message: </b>Check alert fail with error: %s</p>"
)

var (
	ruleFileFlag = flag.String("rules", "rules.json", "File contain the rules need to check")
	bodyFileFlag = flag.String("htmlbody", "body.html", "Alert email html body file")
)

type IChecker interface {
	Check(lg log.Logger) (bool, string)
}

type AlertConfig struct {
	AlertType string            `json:"alert-type"`
	Params    map[string]string `json:"params"`
}

func LoadConfig(ruleFile string, lg log.Logger) []IChecker {
	file, err := os.Open(ruleFile)
	if err != nil {
		lg.Crit("Failed to load rule file", "rule file", ruleFile, "err", err)
	}
	defer file.Close()

	var alerts []*AlertConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&alerts); err != nil {
		lg.Crit("Failed to decode rule file", "rule file", ruleFile, "err", err)
	}

	checkers := make([]IChecker, 0)
	for _, alert := range alerts {
		switch {
		case alert.AlertType == "ESLastMinedBlockChecker":
			checker, err := newESLastMinedBlockChecker(alert.Params)
			if err != nil {
				lg.Crit("Failed to load ESLastMinedBlockChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		case alert.AlertType == "LastBlockChecker":
			checker, err := newLastBlockChecker(alert.Params)
			if err != nil {
				lg.Crit("Failed to load LastBlockChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		case alert.AlertType == "WebsiteOnlineChecker":
			checker, err := newWebsiteOnlineChecker(alert.Params)
			if err != nil {
				lg.Crit("Failed to load WebsiteOnlineChecker", "params", alert.Params, "err", err)
			}
			checkers = append(checkers, checker)
		default:
			lg.Crit("Failed to load alert with unknown type", "type", alert.AlertType, "params", alert.Params)
		}
	}

	return checkers
}

func main() {
	flag.Parse()
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))

	var (
		lg        = log.New("app", "alert")
		needAlert = false
		contents  = ""
	)

	checkers := LoadConfig(*ruleFileFlag, lg)

	for _, checker := range checkers {
		res, content := checker.Check(lg)
		if res {
			needAlert = res
			contents = contents + content + "\n"
		}
	}

	if needAlert {
		writeHtmlFile(fmt.Sprintf(emailFormat, contents), lg)
		os.Exit(1)
	}
}

func writeHtmlFile(content string, lg log.Logger) {
	file, err := os.Create(*bodyFileFlag)
	if err != nil {
		lg.Crit("Create html file fail", "error", err.Error())
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		lg.Crit("Write html file fail", "error", err.Error())
	}
}
