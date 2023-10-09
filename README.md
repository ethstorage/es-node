# es-node

Golang implementation of the EthStorage node.

## Getting started

In order to get ready for storage mining, you need to prepare a miner account as the recipient of mining rewards and a private key as a signer for the mining result transactions. 

It is recommended to use different accounts for the signer and the miner.

Note that you need to have some ETH balance in the account of the private key as the gas fee to submit transactions.

### How to launch an es-node with binary

#### Environment

* go 1.20 or above
* node 16 or above

#### Build and run es-node 

```sh
git clone git@github.com:ethstorage/es-node.git

# build
cd es-node/cmd/es-node && go build && cd ../..

# run
chmod +x run.sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run.sh
```
#### Launch an es-node bootnode

To launch a bootnode for the es-node network, you need to modify the `run.sh` file by adding the `--p2p.advertise.ip` flag: 
```sh
# replace <your_ip_address> with your ip address the other nodes can access.
--p2p.advertise.ip <your_ip_address> \
``` 
to replace the line with the `--p2p.bootnodes` flag:
```sh
--p2p.bootnodes enr:... \
```
Then, soon after the bootnode is started up, you can find the base64 encoded enr value prefixed with `enr:` in the log. 

Next, you will need to replace the enr value of `--p2p.bootnodes` flag in other nodes with the new one to connect to the bootnode. 

### How to launch an es-node with Docker

#### Environment

- Docker-compose version 1.29.2 or above
- Docker version 24.0.5 or above

#### Docker compose
To start es-node with docker-compose, pull es-node source code and execute:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> docker compose up 
```
or
```sh 
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> docker-compose up
```

#### Docker as a background process
Or you can "build and run" a container in a single line of command which runs an es-node Docker container in the background:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh
```
Then check logs by
```sh
docker logs -f es 
```
Where `es` is the name of the es-node container.
#### Docker
To start es-node in a Docker container without docker-compose, pull es-node source code and execute:
```sh
# build image
docker build -t es-node .
# start container
docker run -v ./es-data:/es-node/es-data -e ES_NODE_STORAGE_MINER=<miner> -e ES_NODE_PRIVATE_KEY=<private_key> -p 9545:9545 -p 9222:9222 -p 30305:30305/udp -it --entrypoint /es-node/run.sh es-node
```
Where `es-node` is the name of the es-node image.

## Configuration
The full list of options that you can use to configure an es-node are as follows:
|Name|Description|Default|Required|
|---|---|---|---|
|`--datadir`|Data directory for the storage files, databases and keystore||✓|
|`--download.dump`|Where to dump the downloaded blobs|||
|`--download.start`|Block number which the downloader download blobs from|`0`||
|`--l1.beacon`|Address of L1 beacon chain endpoint to use||✓|
|`--l1.beacon-based-slot`|A slot number in the past time specified by l1.beacon-based-time|`0`|✓|
|`--l1.beacon-based-time`|Timestamp of a slot specified by l1.beacon-based-slot|`0`|✓|
|`--l1.beacon-slot-time`|Slot time of the L1 beacon chain|`12`||
|`--l1.chain_id`|Chain id of L1 chain endpoint to use|`1`||
|`--l1.epoch-poll-interval`|Poll interval for retrieving new L1 epoch updates such as safe and finalized block changes. Disabled if 0 or negative.|`6m24s`||
|`--l1.min-duration-blobs-request`|Min duration for blobs sidecars request|`1572864`||
|`--l1.rpc`|Address of L1 User JSON-RPC endpoint to use (eth namespace required)||✓|
|`--l2.chain_id`|Chain id of L2 chain endpoint to use|`3333`||
|`--log.color`|Color the log output if in terminal mode|||
|`--log.format`|Format the log output. Supported formats: 'text', 'terminal', 'logfmt', 'json', 'json-pretty',|`text`||
|`--log.level`|The lowest log level that will be output|`info`||
|`--metrics.enable`|Enable metrics|||
|`--miner.enabled`|Storage mining enabled|||
|`--miner.gas-price`|Gas price for mining transactions|||
|`--miner.priority-gas-price`|Priority gas price for mining transactions|||
|`--miner.threads-per-shard`|Number of threads per shard|`1`||
|`--miner.zk-working-dir`|Path to the snarkjs folder|`ethstorage\prover`||
|`--miner.zkey`|zkey file name which should be put in the snarkjs folder|`blob_poseidon.zkey`||
|`--network`|Predefined L1 network selection. Available networks: devnet|||
|`--p2p.advertise.ip`|The IP address to advertise in Discv5, put into the ENR of the node. This may also be a hostname / domain name to resolve to an IP.|||
|`--p2p.advertise.tcp`|The TCP port to advertise in Discv5, put into the ENR of the node. Set to p2p.listen.tcp value if 0.|`0`||
|`--p2p.advertise.udp`|The UDP port to advertise in Discv5 as fallback if not determined by Discv5, put into the ENR of the node. Set to p2p.listen.udp value if 0.|`0`||
|`--p2p.ban.peers`|Enables peer banning. This should ONLY be enabled once certain peer scoring is working correctly.|||
|`--p2p.bootnodes`|Comma-separated base64-format ENR list. Bootnodes to start discovering other node records from.|||
|`--p2p.disable`|Completely disable the P2P stack|||
|`--p2p.discovery.path`|Discovered ENRs are persisted in a database to recover from a restart without having to bootstrap the discovery process again. Set to 'memory' to never persist the peerstore.|`esnode_discovery_db`||
|`--p2p.listen.ip`|IP to bind LibP2P and Discv5 to|`0.0.0.0`||
|`--p2p.listen.tcp`|TCP port to bind LibP2P to. Any available system port if set to 0.|`9222`||
|`--p2p.listen.udp`|UDP port to bind Discv5 to. Same as TCP port if left 0.|`0`||
|`--p2p.nat`|Enable NAT traversal with PMP/UPNP devices to learn external IP.|||
|`--p2p.no-discovery`|Disable Discv5 (node discovery)|||
|`--p2p.peers.grace`|Grace period to keep a newly connected peer around, if it is not misbehaving.|`30s`||
|`--p2p.peers.hi`|High-tide peer count. The node starts pruning peer connections slowly after reaching this number.|`30`||
|`--p2p.peers.lo`|Low-tide peer count. The node actively searches for new peer connections if below this amount.|`20`||
|`--p2p.peerstore.path`|Peerstore database location. Persisted peerstores help recover peers after restarts. Set to 'memory' to never persist the peerstore. Peerstore records will be pruned / expire as necessary. Warning: a copy of the priv network key of the local peer will be persisted here.|`esnode_peerstore_db`||
|`--p2p.priv.path`|Read the hex-encoded 32-byte private key for the peer ID from this txt file. Created if not already exists.Important to persist to keep the same network identity after restarting, maintaining the previous advertised identity.|`esnode_p2p_priv.txt`||
|`--p2p.score.bands`|Sets the peer score bands used primarily for peer score metrics. Should be provided in following format: \<threshold>:\<label>;\<threshold>:\<label>;...For example: -40:graylist;-20:restricted;0:nopx;20:friend;|`-40:graylist;-20:restricted;0:nopx;20:friend;`||
|`--p2p.scoring.peers`|Sets the peer scoring strategy for the P2P stack. Can be one of: none or light.Custom scoring strategies can be defined in the config file.|`none`||
|`--p2p.scoring.topics`|Sets the topic scoring strategy. Can be one of: none or light.Custom scoring strategies can be defined in the config file.|`none`||
|`--p2p.sequencer.key`|Hex-encoded private key for signing off on p2p application messages as sequencer.|||
|`--p2p.static`|Comma-separated multiaddr-format peer list. Static connections to make and maintain, these peers will be regarded as trusted.|||
|`--p2p.test.simple-sync.end`|Start of simple sync|`0`||
|`--p2p.test.simple-sync.start`|Start of simple sync|`0`||
|`--rollup.config`|Rollup chain parameters|||
|`--rpc.addr`|RPC listening address|`127.0.0.1`||
|`--rpc.escall-url`|RPC EsCall URL|`http://127.0.0.1:8545`||
|`--rpc.port`|RPC listening port|`9545`||
|`--signer.address`|Address the signer is signing transactions for|||
|`--signer.endpoint`|Signer endpoint the client will connect to|||
|`--signer.hdpath`|HDPath is the derivation path used to obtain the private key for mining transactions|||
|`--signer.mnemonic`|The HD seed used to derive the wallet private keys for mining. Must be used in conjunction with HDPath.|||
|`--signer.private-key`|The private key to sign a mining transaction|||
|`--storage.files`|File paths where the data are stored||✓|
|`--storage.l1contract`|Storage contract address on l1||✓|
|`--storage.miner`|Miner's address to encode data and receive mining rewards||✓|

--------------------------------------------------------------------------------
