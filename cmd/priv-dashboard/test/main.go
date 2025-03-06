package main

import (
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

const (
	TotalPeer = 70

	L1Contract = "0x804C520d3c084C805E37A35E90057Ac32831F96f"
	L2Contract = "0x64003adbdf3014f7E38FC6BE752EB047b95da89A"

	L2Percent      = 3000
	LegacyPercent  = 5000
	SyncingPercent = 1000 // 1000 / 10000 = 10%
	MinedPercent   = 100  // 100/10000 = 1%

	BlobsFilled = uint64(2007734)
	BlobsEmpty  = uint64(2186570)
)

type IState interface {
	ID() string
	Mine()
	Serialize() (string, error)
}

type MiningState struct {
	MiningPower  uint64 `json:"mining_power"`
	SamplingTime uint64 `json:"sampling_time"`
}

type SubmissionState struct {
	Succeeded         int   `json:"succeeded_submission"`
	Failed            int   `json:"failed_submission"`
	Dropped           int   `json:"dropped_submission"`
	LastSucceededTime int64 `json:"last_succeeded_time"`
}

type SyncState struct {
	PeerCount         int    `json:"peer_count"`
	BlobsSynced       uint64 `json:"blobs_synced"`
	BlobsToSync       uint64 `json:"blobs_to_sync"`
	SyncProgress      uint64 `json:"sync_progress"`
	SyncedSeconds     uint64 `json:"sync_seconds"`
	EmptyFilled       uint64 `json:"empty_filled"`
	EmptyToFill       uint64 `json:"empty_to_fill"`
	FillEmptyProgress uint64 `json:"fill_empty_progress"`
	FillEmptySeconds  uint64 `json:"fill_empty_seconds"`
}

type ShardState struct {
	ShardId         uint64           `json:"shard_id"`
	Miner           common.Address   `json:"miner"`
	ProvidedBlob    uint64           `json:"provided_blob"`
	SyncState       *SyncState       `json:"sync_state"`
	MiningState     *MiningState     `json:"mining_state"`
	SubmissionState *SubmissionState `json:"submission_state"`
}

type LegacyNodeState struct {
	Id      string        `json:"id"`
	Version string        `json:"version"`
	Address string        `json:"address"`
	Shards  []*ShardState `json:"shards"`
}

func (s *LegacyNodeState) ID() string {
	return s.Id
}

func (s *LegacyNodeState) Mine() {
	for _, s := range s.Shards {
		if s.SyncState.SyncProgress != 10000 || s.SyncState.FillEmptyProgress != 10000 {
			continue
		}
		r := rand.Intn(10100)
		if r < MinedPercent {
			s.SubmissionState.Succeeded++
		} else if r > 10080 {
			s.SubmissionState.Failed++
		} else if r > 10000 {
			s.SubmissionState.Dropped++
		}
	}
}

func (s *LegacyNodeState) Serialize() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), err
}

type NodeState struct {
	Contract string `json:"contract"`
	*LegacyNodeState
}

func (s *NodeState) Serialize() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), err
}

func UploadNodeState(url string) {
	helloUrl := fmt.Sprintf(url + "/hello")
	stateUrl := fmt.Sprintf(url + "/reportstate")
	states := make([]IState, 0)

	for i := 0; i < TotalPeer; i++ {
		s := generateState()
		states = append(states, s)
		_, err := sendMessage(helloUrl, s.ID())
		if err != nil {
			log.Warn("Send message to resp", "err", err.Error())
			return
		}
	}

	sendStates := func() {
		for _, s := range states {
			s.Mine()
			data, err := s.Serialize()
			if err != nil {
				log.Info("Fail to Marshal node state", "error", err.Error())
				continue
			}
			res, err := sendMessage(stateUrl, data)
			if err != nil {
				log.Info("Fail to upload node state", "id", s.ID(), "error", err.Error())
			} else {
				log.Info("get response", "Serialize data", data, "response", res)
			}
		}
	}

	sendStates()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sendStates()
		}
	}
}

func generateState() IState {
	privateKey, _ := ecdsa.GenerateKey(crypto.S256(), crand.Reader)
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	syncState := &SyncState{
		PeerCount:         rand.Intn(50),
		BlobsSynced:       BlobsFilled,
		BlobsToSync:       0,
		SyncProgress:      10000,
		SyncedSeconds:     rand.Uint64() % 36000,
		EmptyFilled:       BlobsEmpty,
		EmptyToFill:       0,
		FillEmptyProgress: 10000,
		FillEmptySeconds:  rand.Uint64() % 36000,
	}
	submissionState := &SubmissionState{
		Succeeded:         0,
		Failed:            0,
		Dropped:           0,
		LastSucceededTime: time.Now().Add(-1 * time.Minute).Unix(),
	}
	miningState := &MiningState{
		MiningPower:  10000,
		SamplingTime: 3169,
	}

	sp := rand.Intn(10000)
	if sp < SyncingPercent {
		syncState.SyncProgress = uint64(sp * 10)
		syncState.BlobsSynced = BlobsFilled / 10000 * syncState.SyncProgress
		syncState.BlobsToSync = BlobsFilled / 10000 * (10000 - syncState.SyncProgress)
		syncState.FillEmptyProgress = uint64(sp * 10)
		syncState.EmptyFilled = BlobsEmpty / 10000 * syncState.FillEmptyProgress
		syncState.EmptyToFill = BlobsEmpty / 10000 * (10000 - syncState.FillEmptyProgress)
	}

	shards := make([]*ShardState, 0)
	s := ShardState{
		ShardId:         0,
		Miner:           address,
		ProvidedBlob:    100,
		SyncState:       syncState,
		MiningState:     miningState,
		SubmissionState: submissionState,
	}
	shards = append(shards, &s)

	var state IState
	nodePercent := rand.Intn(10000)
	if nodePercent < LegacyPercent {
		state = &LegacyNodeState{
			Id:      "test-node-id-" + address.Hex(),
			Version: "v0.1.15",
			Address: fmt.Sprintf("%d.%d.%d.%d:%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(10000)),
			Shards:  shards,
		}
	} else if nodePercent > LegacyPercent+L2Percent {
		state = &NodeState{
			LegacyNodeState: &LegacyNodeState{
				Id:      "test-node-id-" + address.Hex(),
				Version: "v0.1.16",
				Address: fmt.Sprintf("%d.%d.%d.%d:%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(10000)),
				Shards:  shards,
			},
			Contract: L1Contract,
		}
	} else {
		state = &NodeState{
			LegacyNodeState: &LegacyNodeState{
				Id:      "test-node-id-" + address.Hex(),
				Version: "v0.1.16",
				Address: fmt.Sprintf("%d.%d.%d.%d:%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(10000)),
				Shards:  shards,
			},
			Contract: L2Contract,
		}
	}

	return state
}

func sendMessage(url string, data string) (string, error) {
	contentType := "application/json"
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func main() {
	flag.Parse()
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))

	UploadNodeState("http://65.109.63.154:8888")
}
