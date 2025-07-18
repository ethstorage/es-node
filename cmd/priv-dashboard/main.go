// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
)

const (
	timeoutTime = time.Minute * 10
)

var (
	listenAddrFlag  = flag.String("address", "0.0.0.0", "Listener address")
	portFlag        = flag.Int("port", 8080, "Listener port for the es-node to report node status")
	grafanaPortFlag = flag.Int("grafana", 9500, "Listener port for the metrics report")
	logFlag         = flag.Int("loglevel", 3, "Log level to use for Ethereum and the faucet")
	contractFlag    = flag.String("contract", "0x804C520d3c084C805E37A35E90057Ac32831F96f", "Default contract address used to compatible with older versions of es-node")
)

type record struct {
	receivedTime time.Time
	state        *node.NodeState
}

type dashboard struct {
	ctx    context.Context
	lock   sync.Mutex
	nodes  map[string]*record
	m      *metrics.NetworkMetrics
	logger log.Logger
}

type statistics struct {
	count         int
	versions      map[string]int
	shards        map[uint64]int
	phasesOfShard map[uint64]map[string]int
}

func newDashboard() (*dashboard, error) {
	var (
		m      = metrics.NewNetworkMetrics()
		logger = log.New("app", "Dashboard")
		ctx    = context.Background()
	)

	return &dashboard{
		ctx:    ctx,
		nodes:  make(map[string]*record),
		m:      m,
		logger: logger,
	}, nil
}

func (d *dashboard) HelloHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		d.logger.Warn("Read Hello body failed", "err", err.Error())
		return
	}
	d.logger.Info("Get hello from node", "id", string(body))
	answer := `{"status":"ok"}`
	w.Write([]byte(answer))
}

func (d *dashboard) ReportStateHandler(w http.ResponseWriter, r *http.Request) {
	state := node.NodeState{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		d.logger.Warn("Read ReportState body failed", "err", err.Error())
		return
	}
	err = json.Unmarshal(body, &state)
	if err != nil {
		log.Warn("Parse node state failed", "state", string(body), "error", err.Error())
		w.Write(fmt.Appendf(nil, `{"status":"error", "err message":"%s"}`, err.Error()))
		return
	}
	if err = d.checkState(&state); err != nil {
		log.Warn("check node state failed", "state", string(body), "error", err.Error())
		w.Write(fmt.Appendf(nil, `{"status":"error", "err message":"%s"}`, err.Error()))
		return
	}

	if state.Contract == "" {
		state.Contract = *contractFlag
	}

	log.Info("Get state from peer", "peer id", state.Id, "state", string(body))
	d.lock.Lock()
	d.nodes[state.Id] = &record{receivedTime: time.Now(), state: &state}
	d.lock.Unlock()
	for _, shard := range state.Shards {
		d.m.SetPeerInfo(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner)
		sync, mining, submission := shard.SyncState, shard.MiningState, shard.SubmissionState
		d.m.SetSyncState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, sync.PeerCount, sync.SyncProgress,
			sync.SyncedSeconds, sync.FillEmptyProgress, sync.FillEmptySeconds, shard.ProvidedBlob)
		if mining != nil {
			d.m.SetMiningState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, mining.MiningPower, mining.SamplingTime)
		}
		if submission != nil {
			d.m.SetSubmissionState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, submission.Succeeded,
				submission.Failed, submission.Dropped, submission.LastSucceededTime)
		}
	}

	w.Write([]byte(`{"status":"ok"}`))
}

func (d *dashboard) checkState(state *node.NodeState) error {
	if state == nil {
		return errors.New("state is nil")
	}
	if len(state.Shards) == 0 {
		return fmt.Errorf("no shard exist in the node state %s", state.Id)
	}
	for _, shard := range state.Shards {
		if shard.SyncState == nil {
			return fmt.Errorf("invalid shard state in the node state %s", state.Id)
		}
	}

	return nil
}

func (d *dashboard) Report() {
	summary := make(map[string]*statistics)

	d.lock.Lock()
	defer d.lock.Unlock()
	for id, r := range d.nodes {
		if time.Since(r.receivedTime) > timeoutTime {
			delete(d.nodes, id)
			for _, shard := range r.state.Shards {
				d.m.DeletePeerInfo(r.state.Id, r.state.Contract, r.state.Version, r.state.Address, shard.ShardId, shard.Miner)
			}
			continue
		}

		if _, ok := summary[r.state.Contract]; !ok {
			summary[r.state.Contract] = &statistics{
				count:         0,
				versions:      make(map[string]int),
				shards:        make(map[uint64]int),
				phasesOfShard: make(map[uint64]map[string]int),
			}
		}
		sd := summary[r.state.Contract]
		sd.count++

		if _, ok := sd.versions[r.state.Version]; !ok {
			sd.versions[r.state.Version] = 0
		}
		sd.versions[r.state.Version] = sd.versions[r.state.Version] + 1

		for _, s := range r.state.Shards {
			shard := s.ShardId
			if _, ok := sd.shards[shard]; !ok {
				sd.shards[shard] = 0
			}
			sd.shards[shard] = sd.shards[shard] + 1

			if _, ok := sd.phasesOfShard[shard]; !ok {
				phases := make(map[string]int)
				phases["syncing"] = 0
				phases["mining"] = 0
				phases["mined"] = 0
				sd.phasesOfShard[shard] = phases
			}
			if s.SyncState.SyncProgress < 10000 || s.SyncState.FillEmptyProgress < 10000 {
				sd.phasesOfShard[shard]["syncing"] = sd.phasesOfShard[shard]["syncing"] + 1
			} else if s.SubmissionState != nil && s.SubmissionState.Succeeded > 0 {
				sd.phasesOfShard[shard]["mined"] = sd.phasesOfShard[shard]["mined"] + 1
			} else if s.SubmissionState != nil {
				sd.phasesOfShard[shard]["mining"] = sd.phasesOfShard[shard]["mining"] + 1
			}
		}
	}

	d.m.ResetStaticMetrics()
	for contract, data := range summary {
		d.m.SetStaticMetrics(contract, data.count, data.versions, data.shards, data.phasesOfShard)
	}
}

func (d *dashboard) loop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.Report()
		case <-d.ctx.Done():
			return
		}
	}
}

func (d *dashboard) listenAndServe(port int) error {
	go d.loop()

	http.HandleFunc("/hello", d.HelloHandler)
	http.HandleFunc("/reportstate", d.ReportStateHandler)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func main() {
	// Parse the flags and set up the logger to print everything requested
	flag.Parse()
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		log.Crit("Invalid port")
	}

	if *grafanaPortFlag < 0 || *grafanaPortFlag > math.MaxUint16 {
		log.Crit("Invalid grafana port")
	}
	d, err := newDashboard()
	if err != nil {
		log.Crit("New dashboard fail", "err", err)
	}

	go d.listenAndServe(*portFlag)

	if err := d.m.Serve(d.ctx, *listenAddrFlag, *grafanaPortFlag); err != nil {
		log.Crit("Error starting metrics server", "err", err)
	}
}
