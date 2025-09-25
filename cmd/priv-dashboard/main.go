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
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/email"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/node"
)

const (
	timeoutTime         = time.Minute * 10
	emailReportInterval = time.Hour * 24
)

var (
	listenAddrFlag  = flag.String("address", "0.0.0.0", "Listener address")
	portFlag        = flag.Int("port", 8080, "Listener port for the es-node to report node status")
	grafanaPortFlag = flag.Int("grafana", 9500, "Listener port for the metrics report")

	emailContractsFlag = flag.String("email.contracts", "0xf0193d6E8fc186e77b6E63af4151db07524f6a7A", "Contracts need to notify through email")
	emailEnableFlag    = flag.Bool("email.enabled", false, "Email addresses to send notifications to")
	emailUsernameFlag  = flag.String("email.username", "", "Email username for notifications")
	emailPasswordFlag  = flag.String("email.password", "", "Email password for notifications")
	emailHostFlag      = flag.String("email.host", "smtp.gmail.com", "Email host for notifications")
	emailPortFlag      = flag.Int("email.port", 587, "Email port for notifications")
	emailToFlag        = flag.String("email.to", "", "Email addresses to send notifications to")
	emailFromFlag      = flag.String("email.from", "", "Email address that will appear as the sender of the notifications")
)

type record struct {
	receivedTime time.Time
	state        *node.NodeState
}

type dashboard struct {
	ctx              context.Context
	lock             sync.Mutex
	nodes            map[string]*record
	m                *metrics.NetworkMetrics
	lg               log.Logger
	emailCfg         *email.EmailConfig
	lastNotification time.Time
	lastStateCache   map[string]map[string]*record
}

type statistics struct {
	count         int
	versions      map[string]int
	shards        map[uint64]int
	phasesOfShard map[uint64]map[string]int
}

func newDashboard(lg log.Logger, emailCfg *email.EmailConfig) (*dashboard, error) {
	var (
		m   = metrics.NewNetworkMetrics()
		ctx = context.Background()
	)

	return &dashboard{
		ctx:              ctx,
		nodes:            make(map[string]*record),
		m:                m,
		lg:               lg,
		emailCfg:         emailCfg,
		lastNotification: time.Now().Add(-emailReportInterval),
		lastStateCache:   make(map[string]map[string]*record),
	}, nil
}

func (d *dashboard) HelloHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		d.lg.Warn("Read Hello body failed", "err", err.Error())
		return
	}
	d.lg.Info("Get hello from node", "id", string(body))
	answer := `{"status":"ok"}`
	w.Write([]byte(answer))
}

func (d *dashboard) ReportStateHandler(w http.ResponseWriter, r *http.Request) {
	state := node.NodeState{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		d.lg.Warn("Read ReportState body failed", "err", err.Error())
		return
	}
	err = json.Unmarshal(body, &state)
	if err != nil {
		d.lg.Warn("Parse node state failed", "state", string(body), "error", err.Error())
		w.Write(fmt.Appendf(nil, `{"status":"error", "err message":"%s"}`, err.Error()))
		return
	}
	if err = d.checkState(&state); err != nil {
		d.lg.Warn("check node state failed", "state", string(body), "error", err.Error())
		w.Write(fmt.Appendf(nil, `{"status":"error", "err message":"%s"}`, err.Error()))
		return
	}

	d.lg.Info("Get state from peer", "peer id", state.Id, "state", string(body))
	d.lock.Lock()
	d.nodes[state.Id] = &record{receivedTime: time.Now(), state: &state}
	d.lock.Unlock()
	for _, shard := range state.Shards {
		d.m.SetPeerInfo(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner)
		sync, mining, submission, scanStats := shard.SyncState, shard.MiningState, shard.SubmissionState, state.ScanStats
		d.m.SetDownloadState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, state.SavedBlobs, state.DownloadedBlobs)
		if scanStats != nil {
			d.m.SetScanState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, scanStats.MismatchedCount, scanStats.UnfixedCount)
		}
		d.m.SetSyncState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, sync.PeerCount, sync.SyncProgress,
			sync.SyncedSeconds, sync.FillEmptyProgress, sync.FillEmptySeconds, shard.ProvidedBlob)
		if mining != nil {
			d.m.SetMiningState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, mining.MiningPower, mining.SamplingTime)
		}
		if submission != nil {
			d.m.SetSubmissionState(state.Id, state.Contract, state.Version, state.Address, shard.ShardId, shard.Miner, submission.Submitted,
				submission.Failed, submission.Dropped, submission.LastSubmittedTime)
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
	nodesInNetwork := make(map[string]map[string]*record)
	d.lock.Lock()

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
			} else if s.SubmissionState != nil && s.SubmissionState.Submitted > 0 {
				sd.phasesOfShard[shard]["mined"] = sd.phasesOfShard[shard]["mined"] + 1
			} else if s.SubmissionState != nil {
				sd.phasesOfShard[shard]["mining"] = sd.phasesOfShard[shard]["mining"] + 1
			}
		}
		_, ok := nodesInNetwork[r.state.Contract]
		if !ok {
			nodesInNetwork[r.state.Contract] = make(map[string]*record)
		}
		nodesInNetwork[r.state.Contract][r.state.Id] = r
	}
	d.lock.Unlock()

	if *emailEnableFlag && time.Now().After(d.lastNotification.Add(emailReportInterval)) {
		contracts := make(map[string]struct{})
		cs := strings.Split(*emailContractsFlag, ",")
		for _, c := range cs {
			c = strings.TrimSpace(c)
			if c == "" { // ignore empty records
				continue
			}
			contracts[c] = struct{}{}
		}

		for contract, nodeStates := range nodesInNetwork {
			cache, ok := d.lastStateCache[contract]
			if !ok {
				cache = make(map[string]*record)
			}
			if _, ok = contracts[contract]; ok {
				d.outputSummaryHtml(contract, nodeStates, cache)
			}
		}
		d.lastNotification = time.Now()
		d.lastStateCache = nodesInNetwork
	}

	d.m.ResetStaticMetrics()
	for contract, data := range summary {
		d.m.SetStaticMetrics(contract, data.count, data.versions, data.shards, data.phasesOfShard)
	}
}

func (d *dashboard) outputSummaryHtml(contract string, nodes map[string]*record, cache map[string]*record) {
	var (
		dstr          = time.Now().Format("2006-01-02")
		dataRang      = "(24h)"
		content       = ""
		contentFormat = "<tr>\n\t<td>%s</td>\n\t<td>%s</td>\n\t<td>%d</td>\n\t<td>%d</td>\n\t<td>%d</td>\n\t<td>%d</td>\n\t" +
			"<td>%d</td>\n\t<td>%s</td>\n\t<td>%d</td>\n\t<td>%d</td>\n\t<td>%s</td>\n\t<td>%s</td>\n\t</tr>\n"
		subject = fmt.Sprintf("Subject: Daily Network Statistics Report | %s | %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n", contract, dstr)
	)

	if len(cache) == 0 {
		dataRang = "(total)"
	}

	for _, n := range nodes {
		downloadedBlobs := n.state.DownloadedBlobs
		if r, ok := cache[n.state.Id]; ok {
			if downloadedBlobs >= r.state.DownloadedBlobs {
				downloadedBlobs -= r.state.DownloadedBlobs
			}
		}

		for _, shard := range n.state.Shards {
			submitted, dropped, failed, submittedTime := 0, 0, 0, "N/A"
			if shard.MiningState != nil {
				submitted, dropped, failed = shard.SubmissionState.Submitted, shard.SubmissionState.Dropped, shard.SubmissionState.Failed
				if shard.SubmissionState.LastSubmittedTime > 0 && shard.SubmissionState.Submitted > 0 {
					submittedTime = time.Unix(shard.SubmissionState.LastSubmittedTime, 0).Format("2006-01-02T15:04:05")
				}
			}
			mismatchedCount, unfixedCount := "N/A", "N/A"
			if n.state.ScanStats != nil {
				mismatchedCount = strconv.Itoa(n.state.ScanStats.MismatchedCount)
				unfixedCount = strconv.Itoa(n.state.ScanStats.UnfixedCount)
			}
			content += fmt.Sprintf(contentFormat, n.state.Address, shard.Miner, n.state.SavedBlobs, downloadedBlobs,
				shard.ShardId, shard.SyncState.PeerCount, submitted, submittedTime, dropped, failed, mismatchedCount, unfixedCount)
		}
	}

	body := fmt.Sprintf(`
	<html>
	<body>
		<h3>Daily Network Statistics Report | %s | %s</h3>
		<table border="1" cellspacing="0" cellpadding="5">
			<tr>
				<th>Address</th>
				<th>Miner</th>
                <th>SavedBlobs</th>
                <th>DownloadedBlobs %s</th>
                <th>Shard</th>
                <th>PeerCount</th>
                <th>Submitted</th>
                <th>LastSubmittedTime</th>
                <th>Dropped</th>
                <th>Failed</th>
                <th>MismatchedKVCount</th>
                <th>UnfixedKVCount</th>
			</tr>
			%s
		</table>
	</body>
	</html>`, contract, dstr, dataRang, content)

	msg := []byte(subject + body)
	auth := smtp.PlainAuth("", d.emailCfg.Username, d.emailCfg.Password, d.emailCfg.Host)
	err := smtp.SendMail(fmt.Sprintf("%s:%d", d.emailCfg.Host, d.emailCfg.Port), auth, d.emailCfg.Username, []string{d.emailCfg.To}, msg)
	if err != nil {
		d.lg.Warn("Send email fail ❌:", err)
		return
	}

	d.lg.Info("Send email success ✅")
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

func getEmailConfig() (*email.EmailConfig, error) {
	cfg := &email.EmailConfig{
		Username: *emailUsernameFlag,
		Password: *emailPasswordFlag,
		Host:     *emailHostFlag,
		Port:     uint64(*emailPortFlag),
		To:       *emailToFlag,
		From:     *emailFromFlag,
	}
	if err := cfg.Check(); err != nil {
		return nil, fmt.Errorf("email config check error: %w", err)
	}
	return cfg, nil
}

func main() {
	// Parse the flags and set up the lg to print everything requested
	flag.Parse()
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))
	lg := log.New("app", "Dashboard")

	if *portFlag < 0 || *portFlag > math.MaxUint16 {
		lg.Crit("Invalid port")
	}

	if *grafanaPortFlag < 0 || *grafanaPortFlag > math.MaxUint16 {
		lg.Crit("Invalid grafana port")
	}

	var emailCfg *email.EmailConfig
	var err error

	if *emailEnableFlag {
		emailCfg, err = getEmailConfig()
		if err != nil {
			lg.Crit("Invalid email config", "err", err)
		}
	}

	d, err := newDashboard(lg, emailCfg)
	if err != nil {
		lg.Crit("New dashboard fail", "err", err)
	}

	go d.listenAndServe(*portFlag)

	if err := d.m.Serve(d.ctx, *listenAddrFlag, *grafanaPortFlag); err != nil {
		lg.Crit("Error starting metrics server", "err", err)
	}
}
