package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	es "github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	v4net "github.com/shirou/gopsutil/v4/net"
)

var (
	listenAddrFlag   = flag.String("address", "0.0.0.0", "Listener address")
	rpcFlag          = flag.String("rpc", "http://65.108.230.142:8545", "RPC server address")
	contractFlag     = flag.String("contract", "0xAb3d380A268d088BA21Eb313c1C23F3BEC5cfe93", "Contracts need to notify through email")
	filenameFlag     = flag.String("filename", "./es-data/shard-0.dat", "Data filename")
	minerFlag        = flag.String("miner", "0x5C935469C5592Aeeac3372e922d9bCEabDF8830d", "Miner of storage file")
	miningTreadsFlag = flag.Int("mining-treads", 8, "Number of mining treads")

	pprofEnabledFlag = flag.Bool("pprof.enabled", false, "Enable the pprof HTTP server")
	pprofPortFlag    = flag.Int("pprof.port", 6060, "pprof HTTP server listening port")
	pprofAddrFlag    = flag.String("pprof.addr", "127.0.0.1", "pprof HTTP server listening interface")
)

type IRunner interface {
	Start()
	ProcessState() (string, string)
}

type state struct {
	ReadBytes  uint64  `json:"readBytes"`
	WriteBytes uint64  `json:"writeBytes"`
	BytesSent  uint64  `json:"bytesSent"`
	BytesRecv  uint64  `json:"bytesRecv"`
	CPUPercent float64 `json:"cpuPercent"`
	MemPercent float64 `json:"memPercent"`
}

type stateSum struct {
	InitReadBytes  uint64    `json:"InitReadBytes"`
	InitWriteBytes uint64    `json:"InitWriteBytes"`
	InitBytesSent  uint64    `json:"InitBytesSent"`
	InitBytesRecv  uint64    `json:"InitBytesRecv"`
	CPUPercent     float64   `json:"cpuPercent"`
	MemPercent     float64   `json:"memPercent"`
	InitTime       time.Time `json:"initTime"`
	LastTime       time.Time `json:"lastTime"`
	Count          uint64    `json:"count"`
	LastState      *state    `json:"LastState"`
}

func (ss *stateSum) AddState(newState *state) {
	if ss.LastState == nil {
		ss.InitBytesRecv = newState.BytesRecv
		ss.InitBytesSent = newState.BytesSent
		ss.InitReadBytes = newState.ReadBytes
		ss.InitWriteBytes = newState.WriteBytes
		ss.InitTime = time.Now()
		ss.Count = 0
		ss.CPUPercent = newState.CPUPercent
		ss.MemPercent = newState.MemPercent
	} else {
		ss.CPUPercent = (ss.CPUPercent*float64(ss.Count) + newState.CPUPercent) / float64(ss.Count+1)
		ss.MemPercent = (ss.MemPercent*float64(ss.Count) + newState.MemPercent) / float64(ss.Count+1)
	}
	ss.LastState = newState
	ss.Count++
	ss.LastTime = time.Now()
}

func (ss *stateSum) String() string {
	sec := uint64(ss.LastTime.Sub(ss.InitTime).Seconds())
	if sec == 0 {
		return fmt.Sprintf("StartTime: %s, CPUPercent: %.2f%%, MemPercent: %.2f%%",
			ss.InitTime.Format("2006-01-02 15:04:05"), ss.CPUPercent, ss.MemPercent)
	} else {
		return fmt.Sprintf("TimeUsed: %d, CPUPercent: %.2f%%, MemPercent: %.2f%%, "+
			"ReadBytesPerSec: %dMB, TotalReadBytes: %dMB, WriteBytesPerSec: %dMB, TotalWriteBytes: %dMB, "+
			"BytesSentPerSec: %dMB, BytesRecvPerSec: %dMB", sec, ss.CPUPercent, ss.MemPercent,
			(ss.LastState.ReadBytes-ss.InitReadBytes)/sec, ss.LastState.ReadBytes-ss.InitReadBytes,
			(ss.LastState.WriteBytes-ss.InitWriteBytes)/sec, ss.LastState.WriteBytes-ss.InitWriteBytes,
			(ss.LastState.BytesSent-ss.InitBytesSent)/sec, (ss.LastState.BytesRecv-ss.InitBytesRecv)/sec)
	}
}

// Simple check for Ethereum address format
func isValidEthAddress(addr string) bool {
	return strings.HasPrefix(addr, "0x") && len(addr) == 42
}

// Check if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validateFlags() error {
	// Validate listener address
	if ip := net.ParseIP(*listenAddrFlag); ip == nil {
		return fmt.Errorf("invalid address: %s", *listenAddrFlag)
	}

	// Validate miner address format
	if !isValidEthAddress(*minerFlag) {
		return fmt.Errorf("invalid miner address: %s", *minerFlag)
	}

	// Validate contract address format
	if !isValidEthAddress(*contractFlag) {
		return fmt.Errorf("invalid contract address: %s", *contractFlag)
	}

	// Check if file exists
	if !fileExists(*filenameFlag) {
		return fmt.Errorf("file not found: %s", *filenameFlag)
	}

	// Validate RPC URL
	if _, err := url.ParseRequestURI(*rpcFlag); err != nil {
		return fmt.Errorf("invalid rpc url: %s", *rpcFlag)
	}

	// If pprof is enabled, validate port and address
	if *pprofEnabledFlag {
		if *pprofPortFlag <= 0 || *pprofPortFlag > 65535 {
			return fmt.Errorf("invalid pprof port: %d", *pprofPortFlag)
		}
		if ip := net.ParseIP(*pprofAddrFlag); ip == nil {
			return fmt.Errorf("invalid pprof addr: %s", *pprofAddrFlag)
		}
	}

	return nil
}

func main() {
	// Parse the flags and set up the lg to print everything requested
	flag.Parse()
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))
	lg := log.New("app", "Perf")

	if err := validateFlags(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid flag: %v\n", err)
		os.Exit(1)
	}

	if *pprofEnabledFlag {
		address := net.JoinHostPort(*pprofAddrFlag, fmt.Sprintf("%d", *pprofPortFlag))
		// This context value ("metrics.addr") represents the utils.MetricsHTTPFlag.Name.
		// It cannot be imported because it will cause a cyclical dependency.
		log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
		go func() {
			if err := http.ListenAndServe(address, nil); err != nil {
				log.Error("Failure in running pprof server", "err", err)
			}
		}()
	}

	client, err := eth.Dial(*rpcFlag, common.HexToAddress(*contractFlag), 0, lg)
	if err != nil {
		lg.Error("❌ init client error", "err", err)
		os.Exit(1)
	}
	config, err := client.LoadStorageConfigFromContract(common.HexToAddress(*minerFlag))
	if err != nil {
		lg.Error("❌ LoadStorageConfigFromContract error", "err", err)
		os.Exit(1)
	}
	config.Filenames = []string{*filenameFlag}

	runner, err := initMinerPerfRunner(config, lg)
	if err != nil {
		lg.Error("❌ initMinerPerfRunner perf error", "err", err)
		os.Exit(1)
	}

	ss := new(stateSum)
	go func() {

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			s := collectRuntimeStats("E:")
			ss.AddState(s)
			key, ps := runner.ProcessState()
			lg.Info("Runtime state", key, ps, "state sum", ss.String())
			<-ticker.C
		}
	}()

	lg.Info("Starting miner perf", "miner", config.Miner, "time", time.Now())
	runner.Start()
	key, ps := runner.ProcessState()
	lg.Info("Stop miner perf", key, ps, "final state", ss.String(), "time", time.Now())
}

func initMinerPerfRunner(config *storage.StorageConfig, lg log.Logger) (IRunner, error) {
	threads := uint64(*miningTreadsFlag)
	multiple := (threads + 3) / 4
	nonceLimit := miner.DefaultConfig.NonceLimit * multiple

	shardManager := es.NewShardManager(config.L1Contract, config.KvSize, config.KvEntriesPerShard, config.ChunkSize)
	runner := miner.NewMinerPerfRunner(shardManager.KvEntriesBits(), shardManager.MaxKvSizeBits(), nonceLimit, threads, *filenameFlag, config.Miner, lg)

	return runner, nil
}

func collectRuntimeStats(targetDrive string) *state {
	percent, _ := cpu.Percent(time.Second, false)
	vm, _ := mem.VirtualMemory()
	netStat, _ := v4net.IOCounters(false)
	byteRead, byteWrite := uint64(0), uint64(0)

	ioStat, _ := disk.IOCounters()
	for name, stat := range ioStat {
		if strings.EqualFold(name, targetDrive) {
			byteRead, byteWrite = stat.ReadBytes/1024/1024, stat.WriteBytes/1024/1024
		}
	}
	s := state{
		CPUPercent: percent[0],
		MemPercent: vm.UsedPercent,
		BytesSent:  netStat[0].BytesSent / 1024 / 1024,
		BytesRecv:  netStat[0].BytesRecv / 1024 / 1024,
		ReadBytes:  byteRead,
		WriteBytes: byteWrite,
	}
	return &s
}
