// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package protocol

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethstorage/go-ethstorage/ethstorage"
	"github.com/ethstorage/go-ethstorage/ethstorage/rollup"
)

// TestSync_RequestL2Range test peer RequestBlobsByRange func and verify result
func TestSync_RequestL2Range(t *testing.T) {
	var (
		kvSize       = defaultChunkSize
		kvEntries    = uint64(16)
		lastKvIndex  = uint64(16)
		ctx, cancel  = context.WithCancel(context.Background())
		excludedList = make(map[uint64]struct{})
		db           = rawdb.NewMemoryDatabase()
		mux          = new(event.Feed)
		shards       = make(map[common.Address][]uint64)
		metrics      = NewMetrics("sync_test")
		rollupCfg    = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: false,
		}
	)
	defer cancel()

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	// create ethstorage and generate data
	shardManager, files := createEthStorage(contract, []uint64{0}, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}
	shards[shardManager.ContractAddress()] = shardManager.ShardIds()

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	data := makeKVStorage(contract, []uint64{0}, defaultChunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, defaultEncodeType, metafile)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	smr := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      defaultEncodeType,
		shards:          []uint64{0},
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    data[contract],
	}

	// create local and remote hosts, set up sync client and server
	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.loadSyncStatus()
	remoteHost := createRemoteHost(t, ctx, rollupCfg, smr, metrics, testLog)
	connect(t, localHost, remoteHost, shards, shards)

	time.Sleep(2 * time.Second)
	// send request
	_, err = syncCl.RequestL2Range(ctx, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	verifyKVs(data, excludedList, t)
}

// TestSync_RequestL2Range test peer RequestBlobsByList func and verify result
func TestSync_RequestL2List(t *testing.T) {
	var (
		kvSize       = defaultChunkSize
		kvEntries    = uint64(16)
		lastKvIndex  = uint64(16)
		ctx, cancel  = context.WithCancel(context.Background())
		excludedList = make(map[uint64]struct{})
		db           = rawdb.NewMemoryDatabase()
		mux          = new(event.Feed)
		shards       = make(map[common.Address][]uint64)
		metrics      = NewMetrics("sync_test")
		rollupCfg    = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: false,
		}
	)
	defer cancel()

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	// create ethstorage and generate data
	shardManager, files := createEthStorage(contract, []uint64{0}, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)
	shards[shardManager.ContractAddress()] = shardManager.ShardIds()

	data := makeKVStorage(contract, []uint64{0}, defaultChunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, defaultEncodeType, metafile)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	smr := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      defaultEncodeType,
		shards:          []uint64{0},
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    data[contract],
	}

	// create local and remote hosts, set up sync client and server
	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.loadSyncStatus()
	remoteHost := createRemoteHost(t, ctx, rollupCfg, smr, metrics, testLog)
	connect(t, localHost, remoteHost, shards, shards)

	indexes := make([]uint64, 0)
	for i := uint64(0); i < 16; i++ {
		indexes = append(indexes, i)
	}
	time.Sleep(2 * time.Second)
	// send request
	_, err = syncCl.RequestL2List(indexes)
	if err != nil {
		t.Fatal(err)
	}
	verifyKVs(data, excludedList, t)
}

// TestSaveAndLoadSyncStatus test save sync state to DB for tasks and load sync state from DB for tasks.
func TestSaveAndLoadSyncStatus(t *testing.T) {
	var (
		entries     = uint64(1) << 10
		kvSize      = defaultChunkSize
		lastKvIndex = entries*3 - 20
		db          = rawdb.NewMemoryDatabase()
		mux         = new(event.Feed)
		metrics     = NewMetrics("sync_test")
		rollupCfg   = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: true,
		}
	)
	// create ethstorage and generate data
	shardManager, files := createEthStorage(contract, []uint64{0, 1, 2}, defaultChunkSize, kvSize, entries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	_, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.loadSyncStatus()
	indexes := []uint64{30, 5, 8}
	syncCl.tasks[0].healTask.insert(indexes)
	syncCl.tasks[0].SubTasks[0].First = 1
	syncCl.tasks[0].SubTasks[0].next = 33
	syncCl.tasks[1].SubTasks = make([]*subTask, 0)

	tasks := syncCl.tasks
	syncCl.cleanTasks()
	if !syncCl.tasks[1].done {
		t.Fatalf("task 1 should be done.")
	}
	syncCl.saveSyncStatus(true)

	syncCl.tasks = make([]*task, 0)
	syncCl.loadSyncStatus()
	tasks[0].healTask.Indexes = make(map[uint64]int64)
	tasks[0].SubTasks[0].First = 5
	tasks[0].SubTasks[0].next = 5
	tasks[1].done = false
	if err := compareTasks(tasks, syncCl.tasks); err != nil {
		t.Fatalf("compare kv task fail. err: %s", err.Error())
	}
}

// TestReadWrite tests a basic eth storage read/write
func TestReadWrite(t *testing.T) {
	var (
		kvSize    = defaultChunkSize
		kvEntries = uint64(16)
	)
	shards, files := createEthStorage(contract, []uint64{0}, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shards == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	sm := ethstorage.ContractToShardManager[contract]
	success, err := sm.TryWrite(0, []byte{1}, common.Hash{})
	if !success || err != nil {
		t.Fatalf("failed to write")
	}
	rdata, success, err := sm.TryRead(0, 1, common.Hash{})
	if !success || err != nil {
		t.Fatalf("failed to read")
	}
	if !bytes.Equal([]byte{1}, rdata) {
		t.Fatalf("failed to compare")
	}
}

// testSync sync test with a general process:
// 1. create a storage manager and a local node, then start the sync client;
// 2. prepare test data which need to sync to the local node;
// 3. copy data for remote peers (only copy the data for shard remote peer supported, exclude data whose
// blob index in the excluded list) and create storage manager reader for remote peers;
// 4. create remote peers with storage manager reader and connect to local node;
// 5. wait for sync client syncDone or time out
// 6. verify blobs synced to local node with test data
func testSync(t *testing.T, chunkSize, kvSize, kvEntries uint64, localShards []uint64, lastKvIndex uint64,
	encodeType uint64, waitTime time.Duration, remotePeers []*remotePeer, expectedState bool) {
	var (
		db            = rawdb.NewMemoryDatabase()
		ctx, cancel   = context.WithCancel(context.Background())
		mux           = new(event.Feed)
		localShardMap = make(map[common.Address][]uint64)
		metrics       = NewMetrics("sync_test")
		rollupCfg     = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: true,
		}
	)

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries)*int64(len(localShards)))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer func() {
		metafile.Close()
		os.Remove(metafileName)
	}()

	localShardMap[contract] = localShards
	shardManager, files := createEthStorage(contract, localShards, chunkSize, kvSize, kvEntries, common.Address{}, encodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	data := makeKVStorage(contract, localShards, chunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, encodeType, metafile)
	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.Start()

	finalExcludedList := remotePeers[0].excludedList
	for _, rPeer := range remotePeers {
		// fill empty to excludedList for verify KVs
		fillEmpty(shardManager, rPeer.excludedList)
		finalExcludedList = mergeExcludedList(finalExcludedList, rPeer.excludedList)
		pData := copyShardData(data[contract], rPeer.shards, kvEntries, rPeer.excludedList)
		smr := &mockStorageManagerReader{
			kvEntries:       kvEntries,
			maxKvSize:       kvSize,
			encodeType:      encodeType,
			shards:          rPeer.shards,
			contractAddress: contract,
			shardMiner:      common.Address{},
			blobPayloads:    pData,
		}
		rShardMap := make(map[common.Address][]uint64)
		rShardMap[contract] = rPeer.shards
		remoteHost := createRemoteHost(t, ctx, rollupCfg, smr, metrics, testLog)
		connect(t, localHost, remoteHost, localShardMap, rShardMap)
	}

	checkStall(t, waitTime, mux, cancel)

	if syncCl.syncDone != expectedState {
		t.Fatalf("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, expectedState, len(syncCl.peers))
	}
	verifyKVs(data, finalExcludedList, t)
}

// TestSimpleSync test sync process with local node support a single small (its task contains only 1 subTask) shard
// and sync data from 1 remote peer, it should be sync done.
func TestSimpleSync(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = uint64(16)
	)
	remotePeers := []*remotePeer{{
		shards:       []uint64{0},
		excludedList: make(map[uint64]struct{}),
	}}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, defaultEncodeType, 3, remotePeers, true)
}

// TestMultiSubTasksSync test sync process with local node support a single big (its task contains multi subTask) shard
// and sync data from 1 remote peer, it should be sync done.
func TestMultiSubTasksSync(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(64)
		lastKvIndex = uint64(64)
	)
	remotePeers := []*remotePeer{{
		shards:       []uint64{0},
		excludedList: make(map[uint64]struct{}),
	}}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, defaultEncodeType, 6, remotePeers, true)
}

// TestMultiSync test sync process with local node support two shards and sync shard data from two remote peers,
// it should be sync done.
func TestMultiSync(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = uint64(32)
	)
	remotePeers := []*remotePeer{
		{
			shards:       []uint64{0},
			excludedList: make(map[uint64]struct{}),
		},
		{
			shards:       []uint64{1},
			excludedList: make(map[uint64]struct{}),
		},
	}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0, 1}, lastKvIndex, defaultEncodeType, 4, remotePeers, true)
}

// TestSyncWithFewerResult test sync process with shard which is not full (lastKvIndex < kvSize), it should be sync done.
func TestSyncWithFewerResult(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = uint64(14)
	)
	remotePeers := []*remotePeer{
		{
			shards:       []uint64{0},
			excludedList: make(map[uint64]struct{}),
		},
	}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, defaultEncodeType, 4, remotePeers, true)
}

// TestSyncWithPeerShardsOverlay test sync process with local node support multi shards and sync from multi remote peers,
// and shards supported by remote peers have overlaid, it should be sync done.
func TestSyncWithPeerShardsOverlay(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = kvEntries*4 - 10
	)
	remotePeers := []*remotePeer{
		{
			shards:       []uint64{0, 1, 2},
			excludedList: make(map[uint64]struct{}),
		},
		{
			shards:       []uint64{2, 3},
			excludedList: make(map[uint64]struct{}),
		},
	}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0, 1, 2, 3}, lastKvIndex, defaultEncodeType, 6, remotePeers, true)
}

// TestSyncWithExcludedDataOverlay test sync process with local node support multi shards and sync from multi remote peers,
// and shards supported by peers have overlaid and their excluded list do not have overlaid, it should be sync done.
func TestSyncWithExcludedListNotOverlay(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = kvEntries * 4
	)
	excludedList0 := getRandomU64InRange(make(map[uint64]struct{}), 16, 47, 3)
	excludedList1 := getRandomU64InRange(excludedList0, 16, 47, 3)
	remotePeers := []*remotePeer{
		{
			shards:       []uint64{0, 1, 2},
			excludedList: excludedList0,
		},
		{
			shards:       []uint64{1, 2, 3},
			excludedList: excludedList1,
		},
	}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0, 1, 2, 3}, lastKvIndex, defaultEncodeType, 6, remotePeers, true)
}

// TestSyncWithExcludedList test sync process with local node support a shard and sync data from 1 remote peer
// which has excluded list, it should not be sync done.
func TestSyncWithExcludedList(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = uint64(16)
	)
	remotePeers := []*remotePeer{{
		shards:       []uint64{0},
		excludedList: getRandomU64InRange(make(map[uint64]struct{}), 0, 15, 3),
	}}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, defaultEncodeType, 2, remotePeers, false)
}

// TestSyncDiffEncodeType test sync process with local node support a shard and sync data from 1 remote peer
// with different encode type, they should sync done.
func TestSyncDiffEncodeType(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(16)
		lastKvIndex = uint64(16)
	)
	remotePeers := []*remotePeer{{
		shards:       []uint64{0},
		excludedList: make(map[uint64]struct{}),
	}}

	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, ethstorage.ENCODE_KECCAK_256, 4, remotePeers, true)
	testSync(t, defaultChunkSize, kvSize, kvEntries, []uint64{0}, lastKvIndex, ethstorage.ENCODE_BLOB_POSEIDON, 4, remotePeers, true)
}

// TestAddPeerDuringSyncing test sync process with local node support a shard and sync data from first remote peer
// which has excluded list. After first peer sync finish (blob indexes in excluded list included in heal task),
// the second peer connect and sync the rest of the blobs. The local node should sync done.
func TestAddPeerDuringSyncing(t *testing.T) {
	var (
		kvSize       = defaultChunkSize
		kvEntries    = uint64(16)
		lastKvIndex  = uint64(16)
		encodeType   = uint64(defaultEncodeType)
		db           = rawdb.NewMemoryDatabase()
		ctx, cancel  = context.WithCancel(context.Background())
		mux          = new(event.Feed)
		shards       = []uint64{0}
		shardMap     = make(map[common.Address][]uint64)
		excludedList = getRandomU64InRange(make(map[uint64]struct{}), 0, 15, 3)
		metrics      = NewMetrics("sync_test")
		rollupCfg    = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: true,
		}
	)

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	shardMap[contract] = shards
	shardManager, files := createEthStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	// fill empty to excludedList for verify KVs
	fillEmpty(shardManager, excludedList)

	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.Start()

	data := makeKVStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, encodeType, metafile)
	pData := copyShardData(data[contract], shards, kvEntries, excludedList)
	smr0 := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      encodeType,
		shards:          shards,
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    pData,
	}
	remoteHost0 := createRemoteHost(t, ctx, rollupCfg, smr0, metrics, testLog)
	connect(t, localHost, remoteHost0, shardMap, shardMap)
	time.Sleep(2 * time.Second)

	if syncCl.syncDone {
		t.Fatalf("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, false, len(syncCl.peers))
	}
	verifyKVs(data, excludedList, t)

	smr1 := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      encodeType,
		shards:          shards,
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    data[contract],
	}
	remoteHost1 := createRemoteHost(t, ctx, rollupCfg, smr1, metrics, testLog)
	connect(t, localHost, remoteHost1, shardMap, shardMap)
	checkStall(t, 2, mux, cancel)

	if !syncCl.syncDone {
		t.Fatalf("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, true, len(syncCl.peers))
	}
	verifyKVs(data, make(map[uint64]struct{}), t)
}

// TestCloseSyncWhileFillEmpty test the sync can be cancel while the fill empty is running.
func TestCloseSyncWhileFillEmpty(t *testing.T) {
	var (
		kvSize      = defaultChunkSize
		kvEntries   = uint64(512)
		lastKvIndex = uint64(0)
		db          = rawdb.NewMemoryDatabase()
		mux         = new(event.Feed)
		shards      = []uint64{0}
		shardMap    = make(map[common.Address][]uint64)
		metrics     = NewMetrics("sync_test")
		rollupCfg   = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: true,
		}
	)

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	shardMap[contract] = shards
	shardManager, files := createEthStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}
	defer func(files []string) {
		for _, file := range files {
			os.Remove(file)
		}
	}(files)

	makeKVStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, defaultEncodeType, metafile)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	_, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.Start()
	time.Sleep(10 * time.Millisecond)
	syncCl.Close()

	t.Log("Fill empty status", "filled", syncCl.emptyBlobsFilled, "toFill", syncCl.emptyBlobsToFill)
	if syncCl.syncDone {
		t.Fatalf("fill empty shoud be cancel")
	}
}

// TestAddPeerAfterSyncDone test add peer after sync done, the peer should add successfully (the connection is kept),
// as the remote peer may need to sync data from this local peer, we also need to use the sync client to control
// the peer count.
func TestAddPeerAfterSyncDone(t *testing.T) {
	var (
		kvSize       = defaultChunkSize
		kvEntries    = uint64(16)
		lastKvIndex  = uint64(16)
		encodeType   = uint64(defaultEncodeType)
		db           = rawdb.NewMemoryDatabase()
		ctx, cancel  = context.WithCancel(context.Background())
		mux          = new(event.Feed)
		shards       = []uint64{0}
		shardMap     = make(map[common.Address][]uint64)
		excludedList = make(map[uint64]struct{})
		metrics      = NewMetrics("sync_test")
		rollupCfg    = &rollup.EsConfig{
			L2ChainID:     new(big.Int).SetUint64(3333),
			MetricsEnable: true,
		}
	)

	metafile, err := CreateMetaFile(metafileName, int64(kvEntries))
	if err != nil {
		t.Error("Create metafileName fail", err.Error())
	}
	defer metafile.Close()

	shardMap[contract] = shards
	shardManager, files := createEthStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, common.Address{}, defaultEncodeType)
	if shardManager == nil {
		t.Fatalf("createEthStorage failed")
	}

	defer func(files []string) {
		for _, f := range files {
			os.Remove(f)
		}
	}(files)

	l1 := NewMockL1Source(lastKvIndex, metafileName)
	sm := ethstorage.NewStorageManager(shardManager, l1)
	// fill empty to excludedList for verify KVs
	fillEmpty(shardManager, excludedList)

	localHost, syncCl := createLocalHostAndSyncClient(t, testLog, rollupCfg, db, sm, metrics, mux)
	syncCl.Start()

	data := makeKVStorage(contract, shards, defaultChunkSize, kvSize, kvEntries, lastKvIndex, common.Address{}, encodeType, metafile)
	smr0 := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      encodeType,
		shards:          shards,
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    data[contract],
	}
	remoteHost0 := createRemoteHost(t, ctx, rollupCfg, smr0, metrics, testLog)
	connect(t, localHost, remoteHost0, shardMap, shardMap)
	checkStall(t, 3, mux, cancel)

	if !syncCl.syncDone {
		t.Fatalf("sync state %v is not match with expected state %v, peer count %d", syncCl.syncDone, true, len(syncCl.peers))
	}
	verifyKVs(data, excludedList, t)

	smr1 := &mockStorageManagerReader{
		kvEntries:       kvEntries,
		maxKvSize:       kvSize,
		encodeType:      encodeType,
		shards:          shards,
		contractAddress: contract,
		shardMiner:      common.Address{},
		blobPayloads:    data[contract],
	}
	remoteHost1 := createRemoteHost(t, ctx, rollupCfg, smr1, metrics, testLog)
	connect(t, localHost, remoteHost1, shardMap, shardMap)

	time.Sleep(10 * time.Millisecond)
	if len(syncCl.peers) != 2 {
		t.Fatalf("sync client peers count is not match, expected: %d, actual count %d;", 2, len(syncCl.peers))
	}
}
