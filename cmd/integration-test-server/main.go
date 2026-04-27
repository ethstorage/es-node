// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/cmd/es-utils/utils"
	itutils "github.com/ethstorage/go-ethstorage/cmd/integration-test-server/utils"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
)

const (
	expectedSaidHelloTime    = 10 * time.Minute
	expectedStateRefreshTime = 5 * time.Minute
	executionTime            = 2 * time.Hour

	kvEntries = 8192
	kvSize    = 32 * 4096
	dataSize  = 31 * 4096

	rpcEndpoint      = "http://127.0.0.1:9595"
	uploadedDataFile = ".data"
	shardFile0       = "../../es-data-it/shard-0.dat"
	shardFile1       = "../../es-data-it/shard-1.dat"
	logFile          = "../../es-node-it.log"
)

var (
	portFlag     = flag.Int("port", 9096, "Listener port for the es-node to report node status")
	contractAddr = flag.String("contract_addr", "", "EthStorage contract address")
)

var (
	errorMessages    = make([]string, 0)
	lastQueryTime    = time.Now()
	lastRecord       *node.NodeState
	hasConnectedPeer = false
	contractAddress  = common.Address{}
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
		w.Write(fmt.Appendf(nil, `{"status":"error", "err message":"%s"}`, err.Error()))
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
				(shardState.SyncState.BlobsSynced <= oldShardState.SyncState.BlobsSynced ||
					shardState.SyncState.SyncProgress <= oldShardState.SyncState.SyncProgress) {
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
		if shardState.SubmissionState.LastSubmittedTime == 0 || shardState.SubmissionState.Submitted == 0 {
			addErrorMessage("At lease one block should be mined successfully during the test.")
		}
		if shardState.SubmissionState.Failed > 0 {
			knownFailureCount, err := itutils.CheckKnowFailure(logFile)
			if err != nil {
				addErrorMessage(err.Error())
			}
			if knownFailureCount > 0 {
				log.Warn("Check known failure result", "failure", shardState.SubmissionState.Failed, "known failure", knownFailureCount)
			}
			failureCount := shardState.SubmissionState.Failed - knownFailureCount
			if failureCount > 0 {
				addErrorMessage(fmt.Sprintf("%d submission failed during the test.", failureCount))
			}
		}
		log.Info("Final state", "id", state.Id, "shard", shardState.ShardId, "miner", shardState.Miner, "sync progress",
			shardState.SyncState.SyncProgress, "fill progress", shardState.SyncState.FillEmptyProgress, "mining power",
			shardState.MiningState.MiningPower, "sampling time", shardState.MiningState.SamplingTime, "succeeded submission",
			shardState.SubmissionState.Submitted, "failed submission", shardState.SubmissionState.Failed, "dropped submission",
			shardState.SubmissionState.Dropped, "last succeeded time", shardState.SubmissionState.LastSubmittedTime)
	}
}

func createShardManager() (*es.ShardManager, error) {
	sm := es.NewShardManager(contractAddress, kvSize, kvEntries, kvSize)
	df0, err := es.OpenDataFile(shardFile0)
	if err != nil {
		return nil, err
	}
	err = sm.AddDataFileAndShard(df0)
	if err != nil {
		return nil, err
	}

	df1, err := es.OpenDataFile(shardFile1)
	if err != nil {
		return nil, err
	}
	err = sm.AddDataFileAndShard(df1)
	if err != nil {
		return nil, err
	}

	return sm, nil
}

func verifyData() error {
	file, err := os.OpenFile(uploadedDataFile, os.O_RDONLY, 0755)
	if err != nil {
		return err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Buffer(make([]byte, dataSize*2), kvSize*2)
	fileScanner.Split(bufio.ScanLines)

	sm, err := createShardManager()
	if err != nil {
		return err
	}

	client, err := rpc.DialHTTP(rpcEndpoint)
	if err != nil {
		return err
	}
	defer client.Close()

	i := uint64(0)
	for fileScanner.Scan() {
		expectedData := common.Hex2Bytes(fileScanner.Text())
		blob := utils.EncodeBlobs(expectedData)[0]
		commit, _, _ := sm.TryReadMeta(i)
		data, _, err := sm.TryRead(i, kvSize, common.BytesToHash(commit))
		if err != nil {
			return errors.New(fmt.Sprintf("read %d from shard fail with err: %s", i, err.Error()))
		}
		if bytes.Compare(blob[:], data) != 0 {
			return errors.New(fmt.Sprintf("compare shard data %d fail, expected data %s; data: %s",
				i, common.Bytes2Hex(blob[:64]), common.Bytes2Hex(data[:64])))
		}

		rpcdata, err := downloadBlobFromRPC(client, i, common.BytesToHash(commit))
		if err != nil {
			return errors.New(fmt.Sprintf("get data %d from rpc fail with err: %s", i, err.Error()))
		}
		if bytes.Compare(blob[:], rpcdata) != 0 {
			return errors.New(fmt.Sprintf("compare rpc data %d fail, expected data %s; data: %s",
				i, common.Bytes2Hex(blob[:64]), common.Bytes2Hex(rpcdata[:64])))
		}
		i++
	}
	return nil
}

func downloadBlobFromRPC(client *rpc.Client, kvIndex uint64, hash common.Hash) ([]byte, error) {
	var result hexutil.Bytes
	err := client.Call(&result, "es_getBlob", kvIndex, hash, 0, 0, 4096*32)
	if err != nil {
		return nil, err
	}

	var blob kzg4844.Blob
	copy(blob[:], result)
	commit, err := kzg4844.BlobToCommitment(&blob)
	if err != nil {
		return nil, fmt.Errorf("blobToCommitment failed: %w", err)
	}
	cmt := common.Hash(kzg4844.CalcBlobHashV1(sha256.New(), &commit))
	if bytes.Compare(cmt[:es.HashSizeInContract], hash[:es.HashSizeInContract]) != 0 {
		return nil, fmt.Errorf("invalid blob for %d hash: %s, commit: %s", kvIndex, hash, cmt)
	}

	return result, nil
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
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		log.Crit("Invalid port")
	}
	if *contractAddr == "" {
		log.Crit("Invalid contract address")
	} else {
		contractAddress = common.HexToAddress(*contractAddr)
	}

	go listenAndServe(*portFlag)

	time.Sleep(executionTime)
	checkFinalState(lastRecord)
	if err := verifyData(); err != nil {
		addErrorMessage(err.Error())
	}

	if len(errorMessages) > 0 {
		log.Crit(fmt.Sprintf("integration test fail %v", errorMessages))
	}
}
