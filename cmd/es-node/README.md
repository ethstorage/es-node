# Initialise data storage

 Before running an EthStorage node, you will need to create data files for the shards to be mined. 
 
 The data file will be created in the directory specified by `--datadir`.
 
 You can specify the number of shards to be mined by `shard_len`, with each data file being created per shard named `shard-{shard_index}.dat`, where the shard indexes come from the storage contract which manages the shards being mined. If there are more shards in the storage contract than `shard_len`, the `shard_len` shards with the lowest difficulties are selected to create the corresponding files. E.g.,
```sh
 ./es-node init --l1.rpc http://65.108.236.27:8545 --storage.l1contract 0x43d6A8d89E99A6AfDe21E6778518394D8ba5aEc1 --storage.miner 0x0000000000000000000000000000000000001234 --shard_len 2 --datadir /root/es-data
```

 You can also directly specify a list of shard indexes by `shard_index`(es) to create data files for shard. If both `shard_len` and `shard_index`(es) are provided, `shard_index`(es) take precedence. E.g.,

```sh
 ./es-node init --l1.rpc http://65.108.236.27:8545 --storage.l1contract 0x43d6A8d89E99A6AfDe21E6778518394D8ba5aEc1 --storage.miner 0x0000000000000000000000000000000000001234 --shard_index 0 --shard_index 1 --datadir /root/es-data
```
# Run a bootnode

To config a bootnode, we need to find the ENR of the node via

./es-node --network $network --p2p.listen.udp 30305 --p2p.advertise.ip 192.168.1.2

and es-node should output an ENR record like enr:-....

The enr record can be viewed via https://enr-viewer.com/

And then replace the ENR record in ethstorage/p2p/config.go

To connect the node on the same machine, we need to make sure the TCP port is different
--p2p.listen.tcp ...

# Test Using Simple Sync (--network dev)

1. Prepare storage data for source and destination nodes

On source node
../es-utils/es-utils create --filename storage.dat --chunk_size=131072 --kv_size=131072 --len=256 --miner=0x0000000000000000000000000000000000000000
echo hello | ../es-utils/es-utils shard_write --filename storage.dat --kv_idx=1 --kv_entries=16 --kv_size 131072 --chunk_size 131072 --commit 0xd2f03eafdd8f3de1aba543e57db4936427703cc09bdb7d000000000000000000

On destination node
../es-utils/es-utils create --filename storage.dat --chunk_size=131072 --kv_size=131072 --len=256 --miner=0x0000000000000000000000000000000000000000

2. Prepare source node

./es-node --network dev --p2p.listen.udp 30305 --p2p.advertise.ip 192.168.1.2 --storage.files storage.dat

3. Prepare destination node

../es-node --network dev --p2p.listen.tcp 9223 --p2p.advertise.ip 192.168.1.2 --p2p.test.simple-sync.end 16 --storage.files storage.dat

where --p2p.test.simple-sync.end 16 will trigger a sync from index 0 to 16 (exclusive) when the node starts.

# Test Downloader

1. Start the downloader: `./es-node --network dev --l1.rpc http://65.108.236.27:8545 --l1.beacon http://65.108.236.27:5052 --storage.files storage.dat --storage.l1contract 0xA41e05C4a3Ed4E2c5971bB952d9753508d4dfFB4 --datadir ./database --download.start -2 --download.dump ../es-utils/compare`. We are using devnet6 for testing, and will update the RPC endpoint when the new version is ready.
2. Upload those blob files: `/es-utils blob_upload --private_key xxx`
3. es-node will download the uploaded blobs to ./es-utils/compare/ which is specified by --download.dump
4. Compare the uploaded and downloaded files to check if they are the same: `./test_download.sh`.
5. You can iterate step 2~4 multiple times and see if the downloader works fine
