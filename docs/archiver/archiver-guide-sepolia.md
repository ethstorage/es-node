# Running an op-node to Derive Blocks from Sepolia through EthStorage

## Table of Contents
1. **[Introduction](#introduction)**

2. **[Prerequisites](#prerequisites)**

3. **[Running op-geth](#running-op-geth)**
   - 3.1 [Building op-geth](#building-op-geth)
   - 3.2 [Creating a JWT Secret](#creating-a-jwt-secret)
   - 3.3 [Setting Environment Variables](#setting-environment-variables)
   - 3.4 [Initializing op-geth](#initializing-op-geth)
   - 3.5 [Starting op-geth](#starting-op-geth)

4. **[Running op-node](#running-op-node)**
   - 4.1 [Getting the Code](#getting-the-code)
   - 4.2 [Building the op-node](#building-the-op-node)
   - 4.3 [Copying the JWT Secret](#copying-the-jwt-secret)
   - 4.4 [Downloading the Configuration File](#downloading-the-configuration-file)
   - 4.5 [Starting the op-node](#starting-the-op-node)

5. **[Verifying the Derivation Process](#verifying-the-derivation-process)**
   - 5.1 [Observing the Logs](#observing-the-logs)
     - 5.1.1 [op-geth logs](#op-geth-logs)
     - 5.1.2 [op-node logs](#op-node-logs)
     - 5.1.3 [es-node logs](#es-node-logs)
   - 5.2 [Comparing Blocks](#comparing-blocks)

6. **[Conclusion](#conclusion)**


## Introduction

This document outlines the process for running an op-node from source code, using Sepolia as Layer 1 (L1) and EthStorage as the blob archiver service. The goal is to verify that the op-node correctly derives Layer 2 (L2) blocks from Sepolia using expired blobs (pruned by the Sepolia Beacon Chain) retrieved from the EthStorage node (es-node).

## Prerequisites

Before proceeding, ensure your environment meets the following requirements:

- **Sepolia L1 RPC** provided by an execution client running in archive mode
- **Sepolia L1 Beacon URL**
- **An es-node** running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x27504265a9bc4330e3fe82061a60cd8b6369b4dc) on Sepolia L1
- An OP Stack L2 that utilizes EIP-4844 blobs to submit batches to the BatchInbox

It is assumed that the above services and components are functioning properly.

## Running op-geth

### Building op-geth

Clone the `op-geth` repository and build the execution client:

```bash
git clone https://github.com/ethstorage/op-geth.git
cd op-geth
git checkout testnet
make geth
```

### Creating a JWT Secret

Generate a JWT secret to secure communications between the op-geth and the op-node:

```bash
openssl rand -hex 32 > jwt.txt
```

### Setting Environment Variables

Set the data directory for `op-geth`:

```bash
export DATADIR=./datadir
```

### Initializing op-geth

Download the genesis file and initialize the client:

```bash
curl -o genesis.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_genesis.json
./build/bin/geth init --state.scheme=hash --datadir=$DATADIR ./genesis.json
```

### Starting op-geth

Start the client with the following command:

```bash
./build/bin/geth \
  --datadir $DATADIR \
  --http \
  --http.corsdomain="*" \
  --http.vhosts="*" \
  --http.addr=0.0.0.0 \
  --http.port=9515 \
  --http.api=web3,debug,eth,txpool,net,engine \
  --ws \
  --ws.addr=0.0.0.0 \
  --ws.port=9516 \
  --ws.origins="*" \
  --ws.api=debug,eth,txpool,net,engine \
  --syncmode=full \
  --gcmode=archive \
  --nodiscover \
  --maxpeers=0 \
  --networkid=42069 \
  --port=30903 \
  --authrpc.vhosts="*" \
  --authrpc.addr=0.0.0.0 \
  --authrpc.port=9551 \
  --authrpc.jwtsecret=./jwt.txt \
  --rollup.disabletxpoolgossip=true
```

## Running op-node

### Getting the Code
Clone the Optimism monorepo and check out the `testnet` branch:

```bash
git clone https://github.com/ethstorage/optimism.git
cd optimism
git checkout testnet
```

### Building the op-node

Build the `op-node` component:

```bash
make op-node
```

### Copying the JWT Secret

Copy the JWT file created in the `op-geth` repository to the `op-node` folder:

```bash
cp ../op-geth/jwt.txt ./op-node
```

### Downloading the Configuration File

Download the rollup configuration file for the op-node:

```bash
curl -o rollup.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_rollup.json
```

### Starting the op-node

To start the op-node, execute the following command:

```bash
./op-node/bin/op-node \
  --l2=http://localhost:9551 \
  --l2.jwt-secret=./op-node/jwt.txt \
  --sequencer.enabled=false \
  --p2p.disable \
  --verifier.l1-confs=4 \
  --rollup.config=./rollup.json \
  --rpc.addr=0.0.0.0 \
  --rpc.port=9595 \
  --rpc.enable-admin \
  --l1=http://88.99.30.186:8545 \
  --l1.beacon=http://88.99.30.186:3500 \
  --l1.rpckind=basic \
  --l1.beacon-archiver=http://65.108.236.27:9645
```

**Note:**

- P2P is disabled to ensure that it only syncs data from L1.
- The beacon archiver is configured to point to the es-node archive service.

## Verifying the Derivation Process

### Observing the Logs
After starting the op-node, you can observe the logs from the consoles:

**op-geth logs:**
```log
[11-28|06:22:52.176] Starting work on payload                 id=0x03858bd64d4a64fd
INFO [11-28|06:22:52.181] Imported new potential chain segment     number=51019 hash=004703..bda432 blocks=1 txs=1 mgas=0.077 elapsed=2.486ms mgasps=31.072 age=4mo2w1d snapdiffs=783.92KiB triedirty=0.00B
INFO [11-28|06:22:52.183] Chain head was updated                   number=51019 hash=004703..bda432 root=08f27e..dca4dc elapsed="231.044µs" age=4mo2w1d
INFO [11-28|06:22:52.188] Starting work on payload                 id=0x03fad536b048813e
INFO [11-28|06:22:52.192] Imported new potential chain segment     number=51020 hash=3eb01f..d8a5bd blocks=1 txs=1 mgas=0.046 elapsed=2.351ms     mgasps=19.638 age=4mo2w1d snapdiffs=784.30KiB triedirty=0.00B
INFO [11-28|06:22:52.193] Chain head was updated                   number=51020 hash=3eb01f..d8a5bd root=4fc8dc..b0853f elapsed="256.061µs" age=4mo2w1d
```

**op-node logs:**
```log
t=2024-11-28T06:22:51+0000 lvl=info msg="Advancing bq origin" origin=0xa50d30d5b7f11bda9a2088b865bcd76c54939f25948bd54214925cbef7db36fa:6317197 originBehind=false
t=2024-11-28T06:22:51+0000 lvl=info msg="created new channel" origin=0xa50d30d5b7f11bda9a2088b865bcd76c54939f25948bd54214925cbef7db36fa:6317197 channel=25c3198d163794e69b7a8db518b4585d length=950 frame_number=0 is_last=true
t=2024-11-28T06:22:51+0000 lvl=info msg="Reading channel" channel=25c3198d163794e69b7a8db518b4585d frames=1
t=2024-11-28T06:22:51+0000 lvl=info msg="Found next batch" batch_type=SpanBatch batch_timestamp=1721027318 parent_check=0x12953d01742d50d464ff3a74029e5d3164123126 origin_check=0xae4f85d770d5c50dfcd7413fa8e64e026eb510b8 start_epoch_number=6313597 end_epoch_number=6317190 block_count=25020 txs=0 compression_algo=zlib
t=2024-11-28T06:22:52+0000 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1721027318
t=2024-11-28T06:22:52+0000 lvl=info msg="Inserted block" hash=0x004703b620783b04d93d6842a8ee5e58af5bf2b0777a071109b8b73a99bda432 number=51019 state_root=0x08f27e3061bf9a0d3be04a0f8aace8e68a3d5c474242264c09e4fae469dca4dc timestamp=1721027318 parent=0x12953d01742d50d464ff3a74029e5d3164123126679edd341dcae6f82a03aa25 prev_randao=0xa747f20f795f839862c48254e99589c3a124c87520e8c37db0697dd925de7d9a fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=false derived_from=0xa50d30d5b7f11bda9a2088b865bcd76c54939f25948bd54214925cbef7db36fa:6317197
t=2024-11-28T06:22:52+0000 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1721027320
t=2024-11-28T06:22:52+0000 lvl=info msg="Inserted block" hash=0x3eb01f81601ff6da3731f8a501351e2d416699626dfba9e261bb846d5fd8a5bd number=51020 state_root=0x4fc8dc1857631202e9aeabc3fdac5eab38d9ae10c9bdcff3f322dccba3b0853f timestamp=1721027320 parent=0x004703b620783b04d93d6842a8ee5e58af5bf2b0777a071109b8b73a99bda432 prev_randao=0xa747f20f795f839862c48254e99589c3a124c87520e8c37db0697dd925de7d9a fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=false derived_from=0xa50d30d5b7f11bda9a2088b865bcd76c54939f25948bd54214925cbef7db36fa:6317197
```

**es-node logs:**
```log
t=2024-11-28T06:22:51+0000 lvl=info msg="Blob archiver API request"             url="/eth/v1/beacon/blob_sidecars/5445314?indices=0"
t=2024-11-28T06:22:51+0000 lvl=info msg="Query el block number and kzg"         took(s)=0.160
t=2024-11-28T06:22:51+0000 lvl=info msg="BeaconID to execution block number"    beaconID=5445314 elBlock=6,317,197
t=2024-11-28T06:22:51+0000 lvl=info msg="Parsing event"                         blobHash=0x01336dbdf885172743e110a814d157d589059e3c930d5ea65bc7dd89f29801b7 event="0 of 1"
t=2024-11-28T06:22:51+0000 lvl=info msg="Blobhash matched"                      blobhash=0x01336dbdf885172743e110a814d157d589059e3c930d5ea65bc7dd89f29801b7 index=0 kvIndex=2,006,584
t=2024-11-28T06:22:51+0000 lvl=info msg="Build sidecar"                         took(s)=0.054
t=2024-11-28T06:22:51+0000 lvl=info msg="Sidecar built"                         index=0 sidecar="{\n  \"index\": \"0\",\n  \"kzg_commitment\": \"0xa611a429052363afad5cb4fc4dd3b34ab978839e78d370a682350f567b2aaeaa1976cac10231617ed73748253b4af776\",\n  \"kzg_proof\": \"0x8f6ddcf277707569abfadd484076eaaa440c7256fc60a1ff299842765a8ec8ae6a1406b258f84ca4701c0d0176cc2b03\",\n  \"blob\": \"0x3f000003ce0025c3198d163794e69b7a8db518b4585d0000000003b678daec96298a1d3714c63f29c3658a0d0c5bdd726e935429b60d84685e20c90b04a6d87a39b10fa01dc87d83a44b6a57ee0c36b8301817ee6df60dfc1486351ae948479a24f801fcfdb82c77347775f49dbf7a75f7a3f9f7bfc3f9fdc3..."
t=2024-11-28T06:22:51+0000 lvl=info msg="Query blob sidecars done"              blobs=1
t=2024-11-28T06:22:51+0000 lvl=info msg="Blob archiver API request handled"     took(s)=0.262
```

From the logs, we can see that the op-node keeps converting batches into payload attributes, calling the op-geth to work on the payload to convert it into block, and inserting the block as the chain head. Additionally, you may notice the age of derived blocks is over four months, and a blob archiver API request was handled by the es-node, meaning the blob was retrieved from EthStorage since it was pruned by the L1 Beacon client.

### Comparing Blocks
You can also verify the correctness of the derived rollup blocks by comparing the results of the following commands:

```bash
cast block 51019 -f hash -r http://127.0.0.1:9515
cast block 51019 -f hash -r http://65.109.20.29:8545
```

## Conclusion

By following these instructions, you'll permissionlessly launch an OP Stack rollup node that retrieves blobs from EthStorage, which have been pruned by the Sepolia Beacon Chain. Additionally, you can verify that the op-node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution.