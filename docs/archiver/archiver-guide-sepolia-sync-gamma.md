# Running an op-node to Derive Blocks from Sepolia through EthStorage

## Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)
3. [Running an OP Node](#running-an-op-node)
   - 3.1 [Execution Client](#execution-client)
   - 3.2 [Rollup Node](#rollup-node)
   - 3.3 [Verify OP Node](#verify-op-node)
4. [Test sync with the Archive API](#test-sync-with-the-archive-api)
   - 4.1 [Start a proxy to Beacon API](#start-a-proxy-to-beacon-api)
   - 4.2 [Restart op-node with the Archive API](#restart-op-node-with-the-archive-api)
5. [Verifying the Derivation Process](#verifying-the-derivation-process)
   - 5.1 [Observing the Logs](#observing-the-logs)
   - 5.2 [Comparing Blocks](#comparing-blocks)
6. [Conclusion](#conclusion)

## Introduction

This document outlines the process for running an op-node from source code, using Sepolia as Layer 1 (L1) and EthStorage as the blob archiver service. The goal is to verify that the op-node correctly derives Layer 2 (L2) blocks from Sepolia using expired blobs (pruned by the Sepolia Beacon Chain) retrieved from the EthStorage node (es-node).

## Prerequisites

Before proceeding, ensure your environment meets the following requirements:

- **Sepolia L1 RPC** provided by an execution client running in archive mode
- **An es-node** running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x3fe221a447f350551ff208951098517252018007) on Sepolia L1
- An OP Stack L2 (QuarkChain L2 Gamma testnet in this tutorial) that utilizes EIP-4844 blobs to submit batches to the BatchInbox

## Running an OP Node

### Execution Client

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

### Rollup Node

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

After stated, the op node will finish sync with other node via p2p very quickly.
You can check if the node is correctly synced by comparing the latest block numbers:

```bash
cast bn
cast bn -r https://rpc.gamma.testnet.l2.quarkchain.io:8545
```

If the node is fully synced, mark the current block number, and stop the op-node instance.

## Test sync with the Archive API

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

Restart the op-node with the following command:

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
- The beacon archiver is configured to point to the es-node archive service.

## Verifying the Derivation Process

### Observing the Logs

After starting the op-node, you can observe the logs from the consoles:

**op-node logs:**

```log
t=2025-09-16T09:44:42+0200 lvl=info msg="Advancing bq origin" origin=0xa5db0f9f188b6a51e1e0c01518cc85c54fb1b08fe2e4a09e4769f406906a738a:9207275 originBehind=false
t=2025-09-16T09:44:42+0200 lvl=info msg="Advancing bq origin" origin=0x7d55c34ce8fe2e94edab97f7a79d99716e0c86baf0821d7621dfc9faa0e22d11:9207276 originBehind=false
t=2025-09-16T09:44:42+0200 lvl=info msg="Advancing bq origin" origin=0xd0f4a32d8b4613801f0270d82b06ded009b11a3189cf16d7e89c3f06c76bae16:9207277 originBehind=false
t=2025-09-16T09:44:43+0200 lvl=info msg="Advancing bq origin" origin=0x550d319dd811c72ebd44d88b55c712c59f00c2a4d9efe7acb0c77331baae10e0:9207278 originBehind=false
...
t=2025-09-16T13:15:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1757986822
t=2025-09-16T13:15:32+0200 lvl=info msg="Record safe head" l2=0xdc6e4609ae5fb6528ab8fc5aa7312eb8b89576a9ce276c9fdbb64387e1b57b40:2276962 l1=0x7752fcf783c6bc3d6c55ff676ba3c579ca99f0f0dc84d185f900c36f7e535a32:9213344
t=2025-09-16T13:15:32+0200 lvl=info msg="Inserted new L2 unsafe block" hash=0xfb52e94b7baf680999c7dcd260604596dbb5863d8aa4c599c12cc489c72459e8 number=2276963 state_root=0x6f8d6451bc7c6ac60d7b4c272ea344072fcfa6a0b847f6d50c5bf1dd35ca1294 timestamp=1757986822 parent=0xdc6e4609ae5fb6528ab8fc5aa7312eb8b89576a9ce276c9fdbb64387e1b57b40 prev_randao=0x48fd16fb17500bcbe13146690acd6eaa3e638329521a1d58bfec51192f9deba0 fee_recipient=0x4200000000000000000000000000000000000011 txs=1 build_time=1.336ms insert_time=2.249ms total_time=3.586ms mgas=0.048637 mgasps=13.561241677403777
t=2025-09-16T13:15:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1757986824
t=2025-09-16T13:15:32+0200 lvl=info msg="Record safe head" l2=0xfb52e94b7baf680999c7dcd260604596dbb5863d8aa4c599c12cc489c72459e8:2276963 l1=0x7752fcf783c6bc3d6c55ff676ba3c579ca99f0f0dc84d185f900c36f7e535a32:9213344
t=2025-09-16T13:15:32+0200 lvl=info msg="Inserted new L2 unsafe block" hash=0x4f03259baa62710548cc25aaddc477a93bf081627df5d705a0fd232529c5530e number=2276964 state_root=0xdf6107b1555c08b479e114be32784295919307fc4a706f353467375fdc23ee56 timestamp=1757986824 parent=0xfb52e94b7baf680999c7dcd260604596dbb5863d8aa4c599c12cc489c72459e8 prev_randao=0x48fd16fb17500bcbe13146690acd6eaa3e638329521a1d58bfec51192f9deba0 fee_recipient=0x4200000000000000000000000000000000000011 txs=1 build_time=1.216ms insert_time=1.915ms total_time=3.132ms mgas=0.048637 mgasps=15.526784397428473
t=2025-09-16T13:15:32+0200 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1757986826
t=2025-09-16T13:15:32+0200 lvl=info msg="Record safe head" l2=0x4f03259baa62710548cc25aaddc477a93bf081627df5d705a0fd232529c5530e:2276964 l1=0x7752fcf783c6bc3d6c55ff676ba3c579ca99f0f0dc84d185f900c36f7e535a32:9213344
t=2025-09-16T13:15:32+0200 lvl=info msg="Inserted new L2 unsafe block" hash=0x61c9fc0463ffde42a3f2919d1a2cbde63e139014bddb88a2a2d33e20f5f45c8e number=2276965 state_root=0xc1f3e6231766bbd63733729eb5ea5d436825c87c06e27486c68f5ca14e35ed08 timestamp=1757986826 parent=0x4f03259baa62710548cc25aaddc477a93bf081627df5d705a0fd232529c5530e prev_randao=0x48fd16fb17500bcbe13146690acd6eaa3e638329521a1d58bfec51192f9deba0 fee_recipient=0x4200000000000000000000000000000000000011 txs=1 build_time=1.657ms insert_time=2.112ms total_time=3.769ms mgas=0.048637 mgasps=12.90153668795746
```

**op-geth logs:**
```log
INFO [09-16|13:11:34.578] Imported new potential chain segment     number=2,275,756 hash=1900fd..127eea blocks=1 txs=1 mgas=0.049 elapsed="947.009µs" mgasps=51.359   age=10h11m26s snapdiffs=2.85MiB    triedirty=10.73MiB
INFO [09-16|13:11:34.579] Chain head was updated                   number=2,275,756 hash=1900fd..127eea root=a9eb8c..feea18 elapsed="668.261µs" age=10h11m26s
INFO [09-16|13:11:34.583] Starting work on payload                 id=0x0301f10e4c14fc3c
INFO [09-16|13:11:34.585] Imported new potential chain segment     number=2,275,757 hash=b63896..98e6c0 blocks=1 txs=1 mgas=0.049 elapsed=1.063ms     mgasps=45.734   age=10h11m24s snapdiffs=2.85MiB    triedirty=10.73MiB
INFO [09-16|13:11:34.586] Chain head was updated                   number=2,275,757 hash=b63896..98e6c0 root=a94226..d705f7 elapsed="845.15µs"  age=10h11m24s
```

**es-node logs:**
```log
INFO [09-16|09:26:23.911] Blob archiver API request                url="/eth/v1/beacon/blob_sidecars/8514581?indices=3"
INFO [09-16|09:26:24.367] Blob archiver API request handled        took(s)=0.456
INFO [09-16|10:27:09.649] Blob archiver API request                url="/eth/v1/beacon/blob_sidecars/8518431?indices=1"
INFO [09-16|10:27:10.063] Blob archiver API request handled        took(s)=0.414
INFO [09-16|12:31:19.786] Blob archiver API request                url="/eth/v1/beacon/blob_sidecars/8522289?indices=5"
INFO [09-16|12:31:20.362] Blob archiver API request handled        took(s)=0.576
```

From the logs, we can see that the op-node keeps converting batches into payload attributes, calling the op-geth to work on the payload to convert it into blocks, and inserting the blocks as the chain head.

Additionally, you may notice the age of derived blocks is over 12 hours, and blob archiver API requests were handled by the es-node, meaning the blobs were retrieved from EthStorage since they were pruned by the L1 Beacon client (mocked).

### Comparing Blocks

You can also verify the correctness of the derived rollup blocks by comparing the results of the following commands:

Make sure that the number you pick is greater than the one before the op-node is restarted and lower than the latest.

```bash
cast block 2276963 -f hash -r http://127.0.0.1:8545
cast block 2276963 -f hash -r https://rpc.gamma.testnet.l2.quarkchain.io:8545
```

## Conclusion

By following these instructions, you'll permissionlessly launch an OP Stack rollup node that retrieves blobs from EthStorage, which have been pruned by the Sepolia Beacon Chain. Additionally, you can verify that the op-node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution.
