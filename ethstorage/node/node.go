// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package node

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	ethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/downloader"
	"github.com/ethstorage/go-ethstorage/ethstorage/eth"
	"github.com/ethstorage/go-ethstorage/ethstorage/metrics"
	"github.com/ethstorage/go-ethstorage/ethstorage/miner"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
	"github.com/ethstorage/go-ethstorage/ethstorage/prover"
	"github.com/hashicorp/go-multierror"
)

type EsNode struct {
	log        log.Logger
	appVersion string
	metrics    *metrics.Metrics

	l1HeadsSub     ethereum.Subscription // Subscription to get L1 heads (automatically re-subscribes on error)
	l1SafeSub      ethereum.Subscription // Subscription to get L1 safe blocks, a.k.a. justified data (polling)
	l1FinalizedSub ethereum.Subscription // Subscription to get L1 safe blocks, a.k.a. justified data (polling)

	l1Source   *eth.PollingClient     // L1 Client to fetch data from
	l1Beacon   *eth.BeaconClient      // L1 Beacon Chain to fetch blobs from
	downloader *downloader.Downloader // L2 Engine to Sync
	// l2Source  *sources.EngineClient // L2 Execution Engine RPC bindings
	// rpcSync   *sources.SyncClient   // Alt-sync RPC client, optional (may be nil)
	server  *rpcServer   // RPC server hosting the rollup-node API
	p2pNode *p2p.NodeP2P // P2P node functionality
	// p2pSigner p2p.Signer            // p2p gogssip application messages will be signed with this signer
	// tracer    Tracer                // tracer to get events for testing/debugging
	// runCfg    *RuntimeConfig        // runtime configurables
	storageManager *ethstorage.StorageManager
	db             ethdb.Database

	// some resources cannot be stopped directly, like the p2p gossipsub router (not our design),
	// and depend on this ctx to be closed.
	resourcesCtx   context.Context
	resourcesClose context.CancelFunc
	miner          *miner.Miner
	// feed to notify miner of the sync done event to start mining
	feed *event.Feed
}

func New(ctx context.Context, cfg *Config, log log.Logger, appVersion string) (*EsNode, error) {
	// if err := cfg.Check(); err != nil {
	// 	return nil, err
	// }

	n := &EsNode{
		log:        log,
		appVersion: appVersion,
	}
	// not a context leak, gossipsub is closed with a context.
	n.resourcesCtx, n.resourcesClose = context.WithCancel(context.Background())
	n.feed = new(event.Feed)
	err := n.init(ctx, cfg)
	if err != nil {
		log.Error("Error initializing the rollup node", "err", err)
		// ensure we always close the node resources if we fail to initialize the node.
		if closeErr := n.Close(); closeErr != nil {
			return nil, multierror.Append(err, closeErr)
		}
		return nil, err
	}
	return n, nil
}

func (n *EsNode) init(ctx context.Context, cfg *Config) error {
	// if err := n.initTracer(ctx, cfg); err != nil {
	// 	return err
	// }
	if err := n.initL1(ctx, cfg); err != nil {
		return err
	}
	// if err := n.initRuntimeConfig(ctx, cfg); err != nil {
	// 	return err
	// }
	// if err := n.initRPCSync(ctx, cfg); err != nil {
	// 	return err
	// }
	if err := n.initDatabase(cfg); err != nil {
		return err
	}
	if err := n.initStorageManager(ctx, cfg); err != nil {
		return err
	}
	if err := n.initP2P(ctx, cfg); err != nil {
		return err
	}
	if err := n.initL2(ctx, cfg); err != nil {
		return err
	}
	if err := n.initMiner(ctx, cfg); err != nil {
		return err
	}

	// Only expose the server at the end, ensuring all RPC backend components are initialized.
	if err := n.initRPCServer(ctx, cfg); err != nil {
		return err
	}
	// if err := n.initMetricsServer(ctx, cfg); err != nil {
	// 	return err
	// }
	if &cfg.Mining != nil {
		if err := n.initMiner(ctx, cfg); err != nil {
			return err
		}
	}
	return nil
}

func (n *EsNode) initL2(ctx context.Context, cfg *Config) error {
	n.downloader = downloader.NewDownloader(
		n.l1Source,
		n.l1Beacon,
		n.db,
		n.storageManager,
		cfg.Downloader.DownloadStart,
		cfg.Downloader.DownloadDump,
		cfg.L1.L1MinDurationForBlobsRequest,
		n.log,
	)
	return nil
}

func (n *EsNode) initL1(ctx context.Context, cfg *Config) error {
	client, err := eth.Dial(cfg.L1.L1NodeAddr, cfg.Storage.L1Contract, n.log)
	if err != nil {
		return fmt.Errorf("failed to create L1 source: %w", err)
	}
	n.l1Source = client

	n.l1Beacon = eth.NewBeaconClient(cfg.L1.L1BeaconURL, cfg.L1.L1BeaconBasedTime, cfg.L1.L1BeaconBasedSlot, cfg.L1.L1BeaconSlotTime)
	return nil
}

func (n *EsNode) startL1(cfg *Config) {
	// Keep subscribed to the L1 heads, which keeps the L1 maintainer pointing to the best headers to sync
	n.l1HeadsSub = event.ResubscribeErr(time.Second*10, func(ctx context.Context, err error) (event.Subscription, error) {
		if err != nil {
			n.log.Warn("resubscribing after failed L1 subscription", "err", err)
		}
		return eth.WatchHeadChanges(n.resourcesCtx, n.l1Source, n.OnNewL1Head)
	})
	go func() {
		err, ok := <-n.l1HeadsSub.Err()
		if !ok {
			return
		}
		n.log.Error("l1 heads subscription error", "err", err)
	}()

	// Poll for the safe L1 block and finalized block,
	// which only change once per epoch at most and may be delayed.
	n.l1SafeSub = eth.PollBlockChanges(n.resourcesCtx, n.log, n.l1Source, n.OnNewL1Safe, ethRPC.SafeBlockNumber,
		cfg.L1EpochPollInterval, time.Second*10)
	n.l1FinalizedSub = eth.PollBlockChanges(n.resourcesCtx, n.log, n.l1Source, n.OnNewL1Finalized, ethRPC.FinalizedBlockNumber,
		cfg.L1EpochPollInterval, time.Second*10)
}

func (n *EsNode) initP2P(ctx context.Context, cfg *Config) error {
	if cfg.P2P != nil {
		p2pNode, err := p2p.NewNodeP2P(n.resourcesCtx, &cfg.Rollup, cfg.L1.L1ChainID, n.log, cfg.P2P, n.storageManager, n.db, n.metrics, n.feed)
		if err != nil || p2pNode == nil {
			return err
		}
		n.p2pNode = p2pNode
		if n.p2pNode.Dv5Udp() != nil {
			go n.p2pNode.DiscoveryProcess(n.resourcesCtx, n.log, cfg.L1.L1ChainID, cfg.P2P.TargetPeers())
		}
	}
	return nil
}

func (n *EsNode) initStorageManager(ctx context.Context, cfg *Config) error {
	shardManager := ethstorage.NewShardManager(cfg.Storage.L1Contract, cfg.Storage.KvSize, cfg.Storage.KvEntriesPerShard, cfg.Storage.ChunkSize)
	for _, filename := range cfg.Storage.Filenames {
		var err error
		var df *ethstorage.DataFile
		df, err = ethstorage.OpenDataFile(filename)
		if err != nil {
			return fmt.Errorf("open failed: %w", err)
		}
		if df.Miner() != cfg.Storage.Miner {
			return fmt.Errorf("miner mismatches datafile")
		}
		shardManager.AddDataFileAndShard(df)
	}

	if shardManager.IsComplete() != nil {
		return fmt.Errorf("shard is not completed")
	}

	log.Info("Initalized storage",
		"miner", cfg.Storage.Miner,
		"l1contract", cfg.Storage.L1Contract,
		"kvSize", shardManager.MaxKvSize(),
		"chunkSize", shardManager.ChunkSize(),
		"kvsPerShard", shardManager.KvEntries())

	if cfg.Storage.UseMockL1 {
		mockL1 := ethstorage.NewMockL1Source(shardManager, cfg.Storage.L1MockMetaFile)
		n.storageManager = ethstorage.NewStorageManager(shardManager, mockL1)
		metrics.Enabled = true
	} else {
		n.storageManager = ethstorage.NewStorageManager(shardManager, n.l1Source)
	}

	return nil
}

func (n *EsNode) initRPCServer(ctx context.Context, cfg *Config) error {
	server, err := newRPCServer(ctx, &cfg.RPC, n.storageManager, n.downloader, n.log, n.appVersion)
	if err != nil {
		return err
	}
	n.log.Info("Starting JSON-RPC server")
	if err := server.Start(); err != nil {
		return fmt.Errorf("unable to start RPC server: %w", err)
	}
	n.server = server
	return nil
}

func (n *EsNode) initMiner(ctx context.Context, cfg *Config) error {
	l1api := miner.NewL1MiningAPI(n.l1Source, n.log)
	zkExecPath, err := filepath.Abs("../prover")
	if err != nil {
		return err
	}
	pvr := prover.NewKZGPoseidonProver(zkExecPath, cfg.Mining.ZKeyFileName, n.log)
	n.miner = miner.New(&cfg.Mining, n.storageManager, l1api, &pvr, n.feed, n.log)
	log.Info("Initalized miner")
	return nil
}

func (n *EsNode) Start(ctx context.Context, cfg *Config) error {
	n.startL1(cfg)

	// miner must be started before p2p sync
	if n.miner != nil {
		n.miner.Start()
	}

	if err := n.downloader.Start(); err != nil {
		n.log.Error("Could not start a downloader", "err", err)
		return err
	}

	n.p2pNode.Start()

	go metrics.CollectProcessMetrics(3 * time.Second)
	return nil
}

func (n *EsNode) OnNewL1Head(ctx context.Context, sig eth.L1BlockRef) {
	log.Debug("OnNewL1Head", "BlockNumber", sig.Number)
	if n.downloader != nil {
		n.downloader.OnNewL1Head(sig)
	}
	if n.miner != nil {
		select {
		case n.miner.ChainHeadCh <- sig:
		default:
			// Channel is full, skipping
		}
	}
}

func (n *EsNode) OnNewL1Safe(ctx context.Context, sig eth.L1BlockRef) {
	log.Debug("OnNewL1Safe", "BlockNumber", sig.Number)
}

func (n *EsNode) OnNewL1Finalized(ctx context.Context, sig eth.L1BlockRef) {
	log.Debug("OnNewL1Finalized", "BlockNumber", sig.Number)
	if n.downloader != nil {
		n.downloader.OnL1Finalized(sig.Number)
	}
}

func (n *EsNode) RequestL2Range(ctx context.Context, start, end uint64) (uint64, error) {
	if n.p2pNode != nil {
		return n.p2pNode.RequestL2Range(ctx, start, end)
	}
	n.log.Debug("ignoring request to sync L2 range, no sync method available", "start", start, "end", end)
	return 0, nil
}

func (n *EsNode) Close() error {
	var result *multierror.Error

	metrics.PrintRuntimeMetrics()

	log.Warn("stop node")
	if n.server != nil {
		n.server.Stop()
	}
	if n.p2pNode != nil {
		if err := n.p2pNode.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close p2p node: %w", err))
		}
	}

	if n.downloader != nil {
		if err := n.downloader.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close downloader: %w", err))
		}
	}
	// if n.p2pSigner != nil {
	// 	if err := n.p2pSigner.Close(); err != nil {
	// 		result = multierror.Append(result, fmt.Errorf("failed to close p2p signer: %w", err))
	// 	}
	// }

	if n.resourcesClose != nil {
		n.resourcesClose()
	}

	// stop L1 heads feed
	if n.l1HeadsSub != nil {
		n.l1HeadsSub.Unsubscribe()
	}

	if n.miner != nil {
		n.miner.Close()
	}

	// close L2 driver
	// if n.l2Driver != nil {
	// 	if err := n.l2Driver.Close(); err != nil {
	// 		result = multierror.Append(result, fmt.Errorf("failed to close L2 engine driver cleanly: %w", err))
	// 	}

	// 	// If the L2 sync client is present & running, close it.
	// 	if n.rpcSync != nil {
	// 		if err := n.rpcSync.Close(); err != nil {
	// 			result = multierror.Append(result, fmt.Errorf("failed to close L2 engine backup sync client cleanly: %w", err))
	// 		}
	// 	}
	// }

	// // close L2 engine RPC client
	// if n.l2Source != nil {
	// 	n.l2Source.Close()
	// }

	// close L1 data source
	if n.l1Source != nil {
		n.l1Source.Close()
	}
	if n.storageManager != nil {
		n.storageManager.Close()
	}
	return result.ErrorOrNil()
}

func (n *EsNode) initDatabase(cfg *Config) error {
	var db ethdb.Database
	var err error
	if cfg.DataDir == "" {
		db = rawdb.NewMemoryDatabase()
	} else {
		db, err = rawdb.Open(rawdb.OpenOptions{
			Type:              "leveldb",
			Directory:         cfg.ResolvePath(cfg.DBConfig.Name),
			AncientsDirectory: cfg.ResolveAncient(cfg.DBConfig.Name, cfg.DBConfig.DatabaseFreezer),
			Namespace:         cfg.DBConfig.NameSpace,
			Cache:             cfg.DBConfig.DatabaseCache,
			Handles:           cfg.DBConfig.DatabaseHandles,
			ReadOnly:          false,
		})
	}
	if err == nil {
		n.db = db
	}
	return nil
}
