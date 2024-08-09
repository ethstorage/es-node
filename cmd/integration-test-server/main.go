// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
)

const (
	expectedSaidHelloTime    = 10 * time.Minute
	expectedStateRefreshTime = 5 * time.Minute
	executionTime            = 2 * time.Hour
)

var (
	portFlag = flag.Int("port", 9096, "Listener port for the es-node to report node status")
)

var (
	errorMessages    = make([]string, 0)
	lastQueryTime    = time.Now()
	lastRecord       *node.NodeState
	hasConnectedPeer = false
)

func HelloHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		addErrorMessage(fmt.Sprintf("Read Hello body failed with error %s", err.Error()))
		return
	}
	log.Info("Get hello from node", "id", string(body))

	if time.Since(lastQueryTime) > expectedSaidHelloTime {
		addErrorMessage(fmt.Sprintf("Get Hello message later then expect time %v real value %v", expectedSaidHelloTime, time.Since(lastQueryTime)))
	}
	lastQueryTime = time.Now()
	log.Info("Get Hello request from es-node", "id", string(body))

	answer := `{"status":"ok"}`
	w.Write([]byte(answer))
}

func ReportStateHandler(w http.ResponseWriter, r *http.Request) {
	if time.Since(lastQueryTime) > expectedStateRefreshTime*2 {
		addErrorMessage(fmt.Sprintf("Get Hello message later then expect time %v real value %v",
			expectedStateRefreshTime*2, time.Since(lastQueryTime)))
	}
	lastQueryTime = time.Now()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		addErrorMessage(fmt.Sprintf("Read ReportState body failed with error %s", err.Error()))
		return
	}

	state := &node.NodeState{}
	err = json.Unmarshal(body, state)
	if err != nil {
		addErrorMessage(fmt.Sprintf("Parse node state failed with error %s", err.Error()))
		w.Write([]byte(fmt.Sprintf(`{"status":"error", "err message":"%s"}`, err.Error())))
		return
	}

	// If no state updated
	log.Info("Get state from peer", "peer id", state.Id, "Version", state.Version)
	if lastRecord != nil {
		checkState(lastRecord, state)
	}

	lastRecord = state

	w.Write([]byte(`{"status":"ok"}`))
}

func checkState(oldState, newState *node.NodeState) {
	if len(oldState.Shards) != len(newState.Shards) {
		addErrorMessage(fmt.Sprintf("shards count mismatch between two state, new %d, old %d", len(newState.Shards), len(oldState.Shards)))
		return
	}

	for _, shardState := range newState.Shards {
		check := false
		for _, oldShardState := range oldState.Shards {
			if shardState.ShardId != oldShardState.ShardId {
				continue
			}
			check = true
			if shardState.SyncState.PeerCount > 0 {
				hasConnectedPeer = true
			}

			if oldShardState.SyncState.SyncProgress < 10000 &&
				(shardState.SyncState.BlobsSynced < oldShardState.SyncState.BlobsSynced ||
					shardState.SyncState.SyncProgress < oldShardState.SyncState.SyncProgress) {
				addErrorMessage(fmt.Sprintf("es-node sync progress do not increase in %f minutes, "+
					"old synced: %d, new synced %d; old progress: %d, new progress: %d", expectedStateRefreshTime.Minutes(), oldShardState.SyncState.BlobsSynced,
					shardState.SyncState.BlobsSynced, oldShardState.SyncState.SyncProgress, shardState.SyncState.SyncProgress))
			}
			if oldShardState.SyncState.FillEmptyProgress < 10000 &&
				(shardState.SyncState.EmptyFilled < oldShardState.SyncState.EmptyFilled ||
					shardState.SyncState.FillEmptyProgress < oldShardState.SyncState.FillEmptyProgress) {
				addErrorMessage(fmt.Sprintf("es-node fill empty progress do not increase in %f minutes, "+
					"old filled: %d, new filled %d; old progress: %d, new progress: %d", expectedStateRefreshTime.Minutes(), oldShardState.SyncState.EmptyFilled,
					shardState.SyncState.EmptyFilled, oldShardState.SyncState.FillEmptyProgress, shardState.SyncState.FillEmptyProgress))
			}

			if oldShardState.SyncState.FillEmptyProgress == 10000 && oldShardState.SyncState.SyncProgress == 10000 &&
				(shardState.MiningState.MiningPower == 0 || shardState.MiningState.SamplingTime == 0) {
				addErrorMessage("Mining should be start after sync done.")
			}
		}
		if !check {
			addErrorMessage(fmt.Sprintf("Shard %d in the new state do not exist in the old state", shardState.ShardId))
		}
	}
}

func checkFinalState(state *node.NodeState) {
	if state == nil {
		addErrorMessage("No state submitted during the test")
		return
	}
	if !hasConnectedPeer {
		addErrorMessage("es-node peer count should larger than 0")
	}

	log.Info("Final state", "id", state.Id, "version", state.Version)
	for _, shardState := range state.Shards {
		if shardState.SyncState.SyncProgress != 10000 {
			addErrorMessage("Sync should be finished during the test")
		}
		if shardState.SyncState.FillEmptyProgress != 10000 {
			addErrorMessage("Fill should be finished during the test")
		}
		if shardState.MiningState.SamplingTime == 0 || shardState.MiningState.MiningPower == 0 {
			addErrorMessage("Mining should be start after sync done.")
		}
		if shardState.SubmissionState.LastSucceededTime == 0 || shardState.SubmissionState.Succeeded == 0 {
			addErrorMessage("At lease one block should be mined successfully during the test.")
		}
		if shardState.SubmissionState.Failed > 0 {
			addErrorMessage(fmt.Sprintf("%d submission failed during the test.", shardState.SubmissionState.Failed))
		}
		if shardState.SubmissionState.Dropped > 0 {
			addErrorMessage(fmt.Sprintf("%d submission dropped during the test.", shardState.SubmissionState.Dropped))
		}
		log.Info("Final state", "id", state.Id, "shard", shardState.ShardId, "miner", shardState.Miner, "sync progress",
			shardState.SyncState.SyncProgress, "fill progress", shardState.SyncState.FillEmptyProgress, "mining power",
			shardState.MiningState.MiningPower, "sampling time", shardState.MiningState.SamplingTime, "succeeded submission",
			shardState.SubmissionState.Succeeded, "failed submission", shardState.SubmissionState.Failed, "dropped submission",
			shardState.SubmissionState.Dropped, "last succeeded time", shardState.SubmissionState.LastSucceededTime)
	}
}

func addErrorMessage(errMessage string) {
	log.Warn("Add error message", "msg", errMessage)
	errorMessages = append(errorMessages, errMessage+"\n")
}

func listenAndServe(port int) error {
	http.HandleFunc("/hello", HelloHandler)
	http.HandleFunc("/reportstate", ReportStateHandler)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func main() {
	// Parse the flags and set up the logger to print everything requested
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(3), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		log.Crit("Invalid port")
	}

	go listenAndServe(*portFlag)

	time.Sleep(executionTime)
	checkFinalState(lastRecord)

	if len(errorMessages) > 0 {
		panic(fmt.Sprintf("integration test fail %v", errorMessages))
	}
}
