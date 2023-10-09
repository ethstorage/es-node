// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/ethstorage/go-ethstorage/ethstorage/db"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
	"github.com/ethstorage/go-ethstorage/ethstorage/storage"
)

type Config struct {
	L1         eth.L1EndpointConfig
	Downloader downloader.Config
	// L2     L2EndpointSetup
	// L2Sync L2SyncEndpointSetup

	DataDir  string
	DBConfig *db.Config

	// Driver driver.Config

	Rollup rollup.EsConfig

	// // P2PSigner will be used for signing off on published content
	// // if the node is sequencing and if the p2p stack is enabled
	// P2PSigner p2p.SignerSetup

	RPC RPCConfig

	P2P p2p.SetupP2P

	Storage storage.StorageConfig

	// Metrics MetricsConfig

	// Pprof oppprof.CLIConfig

	// Used to poll the L1 for new finalized or safe blocks
	L1EpochPollInterval time.Duration

	// // Optional
	// Tracer    Tracer
	// Heartbeat HeartbeatConfig
	Mining *miner.Config
}

type RPCConfig struct {
	ListenAddr string
	ListenPort int
	ESCallURL  string
}

// Check verifies that the given configuration makes sense
func (cfg *Config) Check() error {
	// if err := cfg.L2.Check(); err != nil {
	// 	return fmt.Errorf("l2 endpoint config error: %w", err)
	// }
	// if err := cfg.L2Sync.Check(); err != nil {
	// 	return fmt.Errorf("sync config error: %w", err)
	// }
	// if err := cfg.Rollup.Check(); err != nil {
	// 	return fmt.Errorf("rollup config error: %w", err)
	// }
	// if err := cfg.Metrics.Check(); err != nil {
	// 	return fmt.Errorf("metrics config error: %w", err)
	// }
	// if err := cfg.Pprof.Check(); err != nil {
	// 	return fmt.Errorf("pprof config error: %w", err)
	// }
	if cfg.P2P != nil {
		if err := cfg.P2P.Check(); err != nil {
			return fmt.Errorf("p2p config error: %w", err)
		}
	}
	return nil
}

// ResolvePath resolves path in the instance directory.
func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, path)
}

func (c *Config) ResolveAncient(name string, ancient string) string {
	switch {
	case ancient == "":
		ancient = filepath.Join(c.ResolvePath(name), "ancient")
	case !filepath.IsAbs(ancient):
		ancient = c.ResolvePath(ancient)
	}
	return ancient
}
