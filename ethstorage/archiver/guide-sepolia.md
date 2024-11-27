Here's a revised version of your document with improved formatting and language:

# Running an OP Node to Derive Blocks from Sepolia through EthStorage

## Introduction

This document outlines the process for running an OP node from source code, using Sepolia as Layer 1 (L1) and EthStorage as the blob archiver service. The goal is to verify that the OP node correctly derives Layer 2 (L2) blocks from Sepolia using expired blobs (pruned by the Sepolia Beacon Chain) retrieved from the EthStorage node (es-node).

## Prerequisites

Before proceeding, ensure your environment meets the following requirements:

- **Sepolia L1 RPC** provided by an execution client running in archive mode
- **Sepolia L1 Beacon URL**
- **An es-node** running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x27504265a9bc4330e3fe82061a60cd8b6369b4dc) on Sepolia L1
- An OP Stack L2 that utilizes EIP-4844 blobs to submit batches to the BatchInbox

It is assumed that the above services and components are functioning properly.

## Starting OP Geth

### Step 1: Build OP-Geth

First, clone the `op-geth` repository and build the execution client:

```bash
git clone https://github.com/ethereum-optimism/op-geth.git
cd op-geth
git checkout v1.101408.0
make geth
```

### Step 2: Create a JWT Secret

Generate a JWT secret to secure communications between `op-geth` and `op-node`:

```bash
openssl rand -hex 32 > jwt.txt
```

### Step 3: Set Environment Variables

Set the data directory for `op-geth`:

```bash
export DATADIR=./datadir
```

### Step 4: Initialize OP-Geth

Download the genesis file and initialize the client:

```bash
curl -o genesis.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_genesis.json
./build/bin/geth init --state.scheme=hash --datadir=$DATADIR ./genesis.json
```

### Step 5: Start OP-Geth

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

## Starting the OP Node

### Step 1: Get the Code

Clone the Optimism monorepo and check out the `long-term-da` branch if you haven't already:

```bash
git clone https://github.com/ethstorage/optimism.git
cd optimism
git checkout long-term-da
```

### Step 2: Build OP-Node

Build the `op-node` implementation of the Rollup Node:

```bash
make op-node
```

### Step 3: Copy the JWT Secret

Copy the JWT file created in the `op-geth` repository to the `op-node` folder:

```bash
cp ../op-geth/jwt.txt ./op-node
```

### Step 4: Download the Configuration File

Download the rollup configuration file for `op-node`:

```bash
curl -o rollup.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_rollup.json
```

### Step 5: Start OP-Node

To start the OP node, execute the following command:

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

1. P2P is disabled to ensure that it only syncs data from L1.
2. The beacon archiver is configured to point to the es-node archive service.

## Conclusion

By following these instructions, you'll permissionlessly launch an OP Stack rollup node that retrieves blobs from EthStorage, which have been pruned by the Sepolia Beacon Chain. Additionally, you can verify that the OP node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution. 