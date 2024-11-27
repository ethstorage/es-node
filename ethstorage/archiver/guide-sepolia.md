

# Running an OP Node to Derive Blocks From Sepolia Through EthStorage

## Introduction

This document outlines the process of running an op-node from source code, which will use Sepolia as L1, and EthStorage as the blob archiver service. It aims to verify that the op-node will derive L2 blocks from Sepolia using the expired blobs (pruned by Sepolia Beacon chain) retrieved from the EthStorage node (es-node).

## Prerequisite

The tests will be conducted in an environment with the following conditions get ready:
- Sepolia L1 RPC provided by an execution client running in archive mode
- Sepolia L1 Beacon URL
- An es-node running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x27504265a9bc4330e3fe82061a60cd8b6369b4dc) on Sepolia L1
- An OP Stack L2 that uses EIP-4844 blobs to submit batchs to the BatchInbox

It is assumed that the above services or components are functionning.


## Starting OP Geth

### Build op-geth
First you're going to build the `op-geth` implementation of the execution client:
```bash
git clone https://github.com/ethereum-optimism/op-geth.git
cd op-geth
git checkout v1.101408.0

make geth
```

###  Create a JWT secret
Run the following command to generate a JWT secret to secure the communication between  `op-geth` and `op-node` :
```bash
openssl rand -hex 32 > jwt.txt
```


### Set environment variables 

Set the data dir of op-geth as a environment variable:
```bash
export DATADIR=./datadir
```

### Initialize op-geth
Then you need to download the genesis file and initialize the client with it:
```bash
curl -o genesis.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_genesis.json

./build/bin/geth init --state.scheme=hash --datadir=$DATADIR ./genesis.json
```

### Start op-geth
Finally start the client with following command:
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
  
### Get Code
First clone the Optimism monorepo and check out the branch `long-term-da` if not already done so:
```bash
git clone https://github.com/ethstorage/optimism.git
cd optimism
git checkout long-term-da
```

### Build op-node
Build the  `op-node`  implementation of the Rollup Node.
```bash
make op-node
```
### Copy the JWT secret
Copy the jwt file created in `op-geth` repo into `op-node` folder:
```bash
cp ../op-geth/jwt.txt ./op-node
```

### Download the configuration file
Download the rollup file as confguration of op-node:
```bash
curl -o rollup.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_rollup.json
```

### Start op-node
To start the op-node, execute the following commands:

```bash
./op-node/bin/op-node \
  --l2=http://localhost:9551 \
  --l2.jwt-secret=./op-node/jwt.txt \
  --sequencer.enabled=false \
  --p2p.disable  \
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
1. P2P is disabled so that it can only sync data from L1.
2. The beacon archiver is configured to point to the es-node archive service.
  

## Conclusion

By following the instructions above, you will permissionlessly launch an OP Stack rollup node retrieving blobs from EthStorage that have been pruned by the Beacon chain of Sepolia. Additionally, you can verify that the op-node derives L2 blocks from those blobs correctly, which demostrates that EthStorage works effectively as a decentralized long-term data availability solution.