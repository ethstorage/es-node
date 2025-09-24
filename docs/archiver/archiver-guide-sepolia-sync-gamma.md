# Sync a Node of QuarkChain L2 Gamma Testnet from Sepolia through EthStorage

## Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)
3. [Running an OP Node](#running-an-op-node)
   - 3.1 [Start the Execution Client](#start-the-execution-client)
   - 3.2 [Start the Rollup Node](#start-the-rollup-node)
   - 3.3 [Verify OP Node](#verify-op-node)
4. [Sync data from the Archive API](#sync-data-from-the-archive-api)
   - 4.1 [Start a proxy to Beacon API](#start-a-proxy-to-beacon-api)
   - 4.2 [Restart op-node with the Archive API](#restart-op-node-with-the-archive-api)
5. [Verifying the Derivation Process](#verifying-the-derivation-process)
   - 5.1 [Observing the Logs](#observing-the-logs)
   - 5.2 [Comparing Blocks](#comparing-blocks)
6. [Conclusion](#conclusion)

## Introduction

This document outlines the process for running an OP node from source code, using Sepolia as Layer 1 (L1) and EthStorage as the blob archiver service.

The goal is to verify how an OP node that has fallen behind correctly derives Layer 2 (L2) blocks from Sepolia. It does this using expired blobs (pruned by the Sepolia Beacon Chain) retrieved from the EthStorage node (es-node).

## Prerequisites

Before proceeding, ensure your environment meets the following requirements:

- **Sepolia L1 RPC** provided by an execution client running in archive mode
- **An es-node** running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x3fe221a447f350551ff208951098517252018007) on Sepolia L1
- An OP Stack L2 (QuarkChain L2 Gamma testnet in this tutorial) that utilizes EIP-4844 blobs to submit batches to the BatchInbox

## Running an OP Node

An OP node are composed of two core software services, the Rollup Node and the Execution Client. 

The Execution Client is responsible for executing the block payloads it receives from the Rollup Node over JSON-RPC. The Rollup Node is responsible for deriving L2 block payloads from L1 data and passing those payloads to the Execution Client.

In this section we will start both services to synchronize the OP node to the latest block of the network.

### Start the Execution Client

Clone the `op-geth` repository, build and initialize op-geth:

```bash
git clone -b gamma_testnet https://github.com/QuarkChain/op-geth.git
cd op-geth && make geth

curl -LO https://raw.githubusercontent.com/QuarkChain/pm/main/L2/assets/gamma_testnet_genesis.json
./build/bin/geth init --datadir=datadir --state.scheme hash gamma_testnet_genesis.json
openssl rand -hex 32 > jwt.txt
```

Start the op-geth with the following command:

```bash
./build/bin/geth \
  --datadir ./datadir \
  --http \
  --http.corsdomain="*" \
  --http.vhosts="*" \
  --http.addr=0.0.0.0 \
  --http.api=web3,eth,txpool,net \
  --ws \
  --ws.addr=0.0.0.0 \
  --ws.port=8546 \
  --ws.origins="*" \
  --ws.api=eth,txpool,net \
  --networkid=110011 \
  --authrpc.vhosts="*" \
  --authrpc.port=8551 \
  --authrpc.jwtsecret=./jwt.txt \
  --rollup.disabletxpoolgossip \
  --rollup.sequencerhttp=http://65.109.69.90:8545 \
  --rollup.enabletxpooladmission \
  --bootnodes enode://7c9422be3825257ac80f89968e7e6dd3f64608199640ae6cea07b59d2de57642568908974ed4327f092728a64c7bdc04130ebbeaa607b6a1b95d0d25e9c5330b@65.109.69.90:30303 2>&1 | tee -a geth.log
```

### Start the Rollup Node

Clone the Optimism monorepo and build, initialize op-node:

```bash
git clone -b gamma_testnet https://github.com/QuarkChain/optimism.git
pushd optimism && make op-node && popd

cp op-geth/jwt.txt optimism/op-node 
cd optimism/op-node

curl -LO https://raw.githubusercontent.com/QuarkChain/pm/main/L2/assets/gamma_testnet_rollup.json
mkdir safedb

```

To start the op-node, execute the following command:

```bash
export L1_RPC_URL=http://65.108.230.142:8545
export L1_BEACON_URL=http://65.108.230.142:3500

./bin/op-node --l2=http://localhost:8551 \
  --l2.jwt-secret=./jwt.txt \
  --verifier.l1-confs=4 \
  --rollup.config=./gamma_testnet_rollup.json \
  --rpc.port=8547 \
  --rpc.enable-admin \
  --p2p.static=/ip4/65.109.69.90/tcp/9003/p2p/16Uiu2HAmLiwieHqxRjjvPJtn5hSowjnkwRPExZQyNJgUEn8ZjBDj \
  --p2p.listen.ip=0.0.0.0 \
  --p2p.listen.tcp=9003 \
  --p2p.listen.udp=9003 \
  --p2p.no-discovery \
  --p2p.sync.onlyreqtostatic \
  --l1=$L1_RPC_URL \
  --l1.rpckind=basic \
  --l1.beacon=$L1_BEACON_URL \
  --l1.cache-size=0 \
  --safedb.path=safedb \
  --syncmode=execution-layer | tee -a node.log
```

### Verify OP Node

After starting, the op node will finish syncing with other node via p2p very quickly. You can check if the node is correctly synced by comparing the latest block numbers:

```bash
cast bn -r http://127.0.0.1:8545
cast bn -r https://rpc.gamma.testnet.l2.quarkchain.io:8545
```

If the node is completely synced, mark the current block number, and stop the op-node instance.

## Sync data from the Archive API

In this section, we demonstrate how an OP Node derives L2 blocks using blobs from the EthStorage archive API.

### Start a proxy to Beacon API

The following commands start a service to mock Beacon API with a shorter blob retention period:

```bash
export L1_BEACON_URL=http://65.108.230.142:3500

git clone https://github.com/ethstorage/beacon-api-wrapper.git
cd beacon-api-wrapper
go run cmd/main.go -b $L1_BEACON_URL -p 3602 -r 3
```

If a blob older than 3 epochs (~20 minutes) is requested, the proxy will return an empty list. Other than that, it serves all requests just like a normal Beacon API does.

### Restart op-node with the Archive API

The timing of restarting the stopped op-node is crucial for this test. Generally speaking, waiting 13 hours should be sufficient. If you prefer not to wait that long, simply ensure that:
- At least one batch transaction has been submitted to L1 since the op-node stopped
- The most recent batch has exceeded the mocked retention period

Check the submissions of the batch transactions [here](https://sepolia.etherscan.io/address/0x3fe221A447f350551ff208951098517252018007).

Restart the op-node at the appropriate time with the following command:

```bash
export L1_RPC_URL=http://65.108.230.142:8545
export L1_BEACON_MOCK=http://localhost:3602
export L1_ARCHIVE_API=https://archive.testnet.ethstorage.io:9635

./bin/op-node \
  --l2=http://localhost:8551 \
  --l2.jwt-secret=./jwt.txt \
  --verifier.l1-confs=4 \
  --rollup.config=./gamma_testnet_rollup.json \
  --rpc.port=8547 \
  --sequencer.enabled=false \
  --p2p.disable \
  --rpc.enable-admin \
  --l1=$L1_RPC_URL \
  --l1.rpckind=basic \
  --l1.beacon=$L1_BEACON_MOCK \
  --l1.beacon-archiver=$L1_ARCHIVE_API \
  --safedb.path=safedb | tee -a node.log
```

**Note:**

- P2P is disabled to ensure that it only syncs data from L1.
- The beacon archiver is configured to use the es-node archive service as the fallback source of blobs.

## Verifying the Derivation Process

### Observing the Logs

After starting the op-node, you can observe the logs from the consoles:

**op-node logs:**

```log
t=2025-09-24T10:22:27+0200 lvl=info msg="Advancing bq origin" origin=0x844a9429a59c53c954ee4cbed4fe045bb0d58e53726ee2a7064bed250cfca95d:9263658 originBehind=false
t=2025-09-24T10:22:27+0200 lvl=info msg="Advancing bq origin" origin=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659 originBehind=false
t=2025-09-24T10:22:31+0200 lvl=info msg="created new channel" stage=channel origin=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:31+0200 lvl=info msg="decoded span batch from channel" batch_type=SpanBatch batch_timestamp=1758597326 parent_check=0x7b5b06d6fdc439f9604dff8f7dd96fc0bdc3b320 origin_check=0x5ee498aebee9a36b39fde8638af8fa4b94991bd0 start_epoch_number=9260058 end_epoch_number=9263652 block_count=23011 txs=0 compression_algo=zlib stage_origin=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:31+0200 lvl=info msg="Found next valid span batch" origin=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659 epoch=0x6c8662d4b263ca7ff59eef072e4de677d2576b9e6e52eeafca2a586e2efe72ae:9262264 batch_type=SpanBatch batch_timestamp=1758597326 parent_check=0x7b5b06d6fdc439f9604dff8f7dd96fc0bdc3b320 origin_check=0x5ee498aebee9a36b39fde8638af8fa4b94991bd0 start_epoch_number=9260058 end_epoch_number=9263652 block_count=23011 txs=0
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625538
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625540
t=2025-09-24T10:22:32+0200 lvl=info msg="Record safe head" l2=0xf991363ca31f1b1754ea85542ac75d9a19349cdcae5971d6cf8064faaf67a98f:2596321 l1=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625542
t=2025-09-24T10:22:32+0200 lvl=info msg="Record safe head" l2=0xe1e7269b0f67a15739a1b613cf7a4b0391746ff70271848cfb0c67abd064b028:2596322 l1=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625544
t=2025-09-24T10:22:32+0200 lvl=info msg="Record safe head" l2=0x51bd98649d29d7e4c09a74955a648f4bd99e118c5d0bf15edf1f3d65d1a6bb7f:2596323 l1=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625546
t=2025-09-24T10:22:32+0200 lvl=info msg="Record safe head" l2=0x71811da4a764a5bef91e52d8cd9237d1927bb76aceb100d5a495e900142ea39d:2596324 l1=0x6de121f2d0d020a7ac9bfd31e0e3ec8057080a40b925abcf89abeda1d976e0e7:9263659
t=2025-09-24T10:22:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1758625548
```

From the op-node logs, we can see that the op-node advances batch queue until the batch is found at L1 block 9263659. It then converts the batch into payload attributes, calls the op-geth to generate L2 blocks, and inserts the blocks as the chain head.

**beacon logs:**
```log
2025/09/24 10:22:27 Received request for /eth/v1/beacon/blob_sidecars/8575813
2025/09/24 10:22:27 Block 8575813 is not in the retention window
```
The blob request was unfulfilled by the L1 Beacon client (mocked) since the blob was expired.

**es-node logs:**
```log
INFO [09-24|10:22:28.787] Blob archiver API request                url="/eth/v1/beacon/blob_sidecars/8575813?indices=2"
```
Later the blob request was handled by es-node and the blob was retrieved from EthStorage.

### Comparing Blocks

You can also verify the correctness of the derived blocks by comparing the results of the following commands:

Make sure that the number you pick is greater than the one before the op-node is restarted and lower than the latest.

```bash
cast block 2611953 -f hash -r http://127.0.0.1:8545
cast block 2611953 -f hash -r https://rpc.gamma.testnet.l2.quarkchain.io:8545
```

## Conclusion

By following these instructions, you'll permissionlessly launch an OP Stack rollup node that retrieves blobs from EthStorage, which have been pruned by the Sepolia Beacon Chain. Additionally, you can verify that the op-node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution.
