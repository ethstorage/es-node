# Testing OP Stack with EthStorage as Archive Service: A Step-by-Step Guide

[Introduction](#introduction)

1. [Preparations](#preparations)
   - [Software Dependencies](#software-dependencies)
   - [Get the Correct Code Branch](#get-the-correct-code-branch)
   - [Source of Gas](#source-of-gas)
   - [Fill Out Environment Variables](#fill-out-environment-variables)
2. [L1 Setup](#l1-setup)
   - [Starting L1](#starting-l1)
   - [Deploying EthStorage Contracts](#deploying-ethstorage-contracts)
   - [Deploy BatchInbox](#deploy-batchinbox)
   - [Running a Proxy of L1 Beacon](#running-a-proxy-of-l1-beacon)
3. [Running EthStorage Node](#running-ethstorage-node)
   - [Installation](#installation)
   - [Initialization](#initialization)
   - [Running ES Node in Archiver Mode](#running-es-node-in-archiver-mode)
4. [L2 Setup](#l2-setup)
   - [Running L2](#running-l2)
   - [Starting OP Geth](#starting-op-geth)
   - [Starting the OP Node](#starting-the-op-node)
5. [Verifying Sync Status](#verifying-sync-status)
6. [Conclusion](#conclusion)

## Introduction

This guide provides detailed steps for setting up a self-contained test environment for the OP Stack rollup, utilizing EthStorage as an archive service. 
The test framework is based on the Bedrock devnet but allows for separate control of Layer 1 (L1) and Layer 2 (L2). The document explains how to configure and start all necessary components, including:
- L1 that serves as RPC endpoint and Beacon API, 
- Rollup services such as op-geth, sequencer, batcher,  proposer, etc., plus an extra rollup node in validator mode on L2, 
- The deployment of EthStorage contracts and BatchInbox contract that help to store batch data into EthStorage. 
- Launch an EthStorage node (es-node) in archiver mode. 

## Preparations

### Software Dependencies

| Dependency | Version | Version Check Command        |
|------------|---------|------------------------------|
| git        | ^2      | `git --version`              |
| go         | ^1.21   | `go version`                 |
| node       | ^20     | `node --version`             |
| foundry    | ^0.2.0  | `forge --version`            |
| make       | ^3      | `make --version`             |
| jq         | ^1.6    | `jq --version`               |
| direnv     | ^2      | `direnv --version`           |
| docker     | ^27     | `docker --version`			  |


### Get the Correct Code Branch

First clone the Optimism monorepo and check out the branch `long-term-da`:

```bash
git clone https://github.com/ethstorage/optimism.git
cd optimism
git checkout long-term-da
```
Now you can run the following command to check the dependencies in your system:

```bash
./packages/contracts-bedrock/scripts/getting-started/versions.sh
```
Especially, if `direnv` is not installed yet:
```bash
apt install direnv
echo 'eval "$(direnv hook bash)"' >> ~/.bashrc
source ~/.bashrc
```

### Source of Gas

Locate the private key that contains enough balance for later transactions in `ops-bedrock/op-batcher-key.txt`, 
and store the private key in your environment:
```bash
export PRIVATE_KEY=bf7604d9d3a1c7748642b1b7b05c2bd219c9faa91458b370f85e5a40f3b03af7
```

### Fill Out Environment Variables

To configure the environment variables, begin by copying the example configuration file:
```
cp .envrc.example .envrc
``` 

Edit the `.envrc` file to include the necessary environment variables:

```bash
export L1_CHAIN_ID=900
export L1_BLOCK_TIME=12

export L2_CHAIN_ID=901
export L2_BLOCK_TIME=2

export L1_RPC_KIND=debug_geth
export L1_RPC_URL=http://localhost:8545

export PRIVATE_KEY=bf7604d9d3a1c7748642b1b7b05c2bd219c9faa91458b370f85e5a40f3b03af7
```

## L1 Setup

### Starting L1

While still in the Optimism monorepo, execute the following command:
```bash
make devnet-up-l1
```
This command will start the following services:

- Container ops-bedrock-l1-1
- Container ops-bedrock-l1-bn-1
- Container ops-bedrock-l1-vc-1

Now, navigate to the parent directory in preparation for the next steps.

### Deploying EthStorage Contracts

Begin by cloning the EthStorage contract repository and install the dependencies:
```bash
git clone https://github.com/ethstorage/storage-contracts-v1.git
cd storage-contracts-v1
git checkout op-devnet
npm install
``` 

Create a `.env` file and populate it with the following content:
```
L1_RPC_URL=http://localhost:8545
PRIVATE_KEY=bf7604d9d3a1c7748642b1b7b05c2bd219c9faa91458b370f85e5a40f3b03af7
```

Now, deploy the contract with the Hardhat framework:
```bash
npx hardhat run scripts/deploy.js --network op_devnet
```
Make sure to save the **storage contract address** for future use. For example:

```hash
export ES_CONTRACT=0x9B75f686F348d18AF9A4b98e0290D24350d742c4  # replace with the actual address
```
Now, navigate to the parent directory in preparation for the next steps.

### Deploy BatchInbox

Clone and build the BatchInbox contract:
```bash
git clone https://github.com/ethstorage/es-op-batchinbox.git
cd es-op-batchinbox
```

Deploy the BatchInbox contract:
```bash
forge create src/BatchInbox.sol:BatchInbox  \
--constructor-args $ES_CONTRACT \
--private-key $PRIVATE_KEY \
--rpc-url http://localhost:8545

Deployer: 0xDe3829A23DF1479438622a08a116E8Eb3f620BB5
Deployed to: 0xb860F42DAeD06Cf3dC9C3b4B8A287523BbdB2B1e
Transaction hash: 0x99f6788e90004a68e67fa2848e47f7592ffb38aaff31b1738bcc163d806a00a5
```

Make sure to save the deployed contract address for future use. For example:

```hash
export BATCH_INBOX=0xb860F42DAeD06Cf3dC9C3b4B8A287523BbdB2B1e  # replace with the actual address
```

Do not forget to fund the batcher in the BatchInbox account, where `0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC` is the batcher address used in the devnet:
```bash
cast send $BATCH_INBOX "deposit(address)" 0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC --value 100ether --private-key $PRIVATE_KEY
```

Finally, navigate to the Optimism monorepo and locate `batchInboxAddress` in the file `packages/contracts-bedrock/deploy-config/devnetL1.json`:

```json
  "batchInboxAddress": "0xff00000000000000000000000000000000000901",
```
Update the value of `batchInboxAddress` with the address of the contract you just deployed. 

Now, navigate to the parent directory in preparation for the next steps.

### Running a Proxy of L1 Beacon

For the convenience of testing, you will start a proxy for the Beacon API with a shorter blob retention period.

First retrieve the beacon genesis time for later use:
```bash
curl -s http://localhost:5052/eth/v1/beacon/genesis | jq -r '.data.genesis_time'

1732529739

export GENESIS_TIME=1732529739 // replace with the actual timestamp
```

The following commands start a proxy to Beacon API with a shorter blobs retension period:
```bash
git clone https://github.com/ethstorage/beacon-api-wrapper.git
cd beacon-api-wrapper
go run cmd/main.go -b http://localhost:5052 -p 3602 -g $GENESIS_TIME -r 1200
```
This setup allows you to test archive service effectively. 
For blob requests, if the request is within the latest 100 slots, the proxy will retrieve blobs from `http://localhost:5052`. For requests older than that, it will return an empty list.

## Running EthStorage Node


### Installation

To set up the es-node, first clone the repository and build it:
```bash
git clone https://github.com/ethstorage/es-node.git
cd es-node
git checkout v0.1.16
make
```

### Initialization

Initialize es-node:
```bash
./init-rpc.sh \
--l1.rpc http://localhost:8545 \
--storage.l1contract $ES_CONTRACT
```

### Running ES Node in Archiver Mode

Finally, run the es-node:

```bash
./run-rpc.sh \
--storage.l1contract $ES_CONTRACT \
--l1.rpc http://localhost:8545 \
--l1.beacon http://localhost:5052 \
--l1.beacon-based-time $GENESIS_TIME \
--l1.beacon-based-slot 0 \
--p2p.listen.udp 30375 \
--p2p.listen.tcp 9733 \
--rpc.port 9745 \
--archiver.port 6678 \
--archiver.enabled 
```

Now, navigate to the parent directory in preparation for the next steps.

## L2 Setup

### Running L2

Once again, enter the Optimism monorepo and start the Layer 2 environment by executing the following command:
```bash
make devnet-up-l2
```

This command will start the following services:
- Container ops-bedrock-l2-1
- Container ops-bedrock-op-batcher-1
- Container ops-bedrock-op-node-1
- Container ops-bedrock-op-proposer-1
- Container ops-bedrock-op-challenger-1
- Container ops-bedrock-artifact-server-1

Now, navigate to the parent directory in preparation for the next steps.

The following steps will add an additional OP node in validator mode, configured to sync expired blob data from the es-node. This aims to verify that the functions of the derivation pipeline are working correctly with the BatchInbox contract and the EthStorage archive service. 

### Starting OP Geth

Clone the OP Geth repository and build it:
```bash
git clone https://github.com/ethereum-optimism/op-geth.git
cd op-geth
git checkout v1.101408.0

make geth
```
Initialize the OP Geth with devnet genesis configuration.
```bash
./build/bin/geth init --state.scheme=hash --datadir=datadir ../optimism/.devnet/genesis-l2.json
```

Copy the test jwt secret:
```bash
cp ../optimism/ops-bedrock/test-jwt-secret.txt jwt.txt
```

Start the OP Geth process:
```bash
./build/bin/geth \
  --datadir ./datadir \
  --http \
  --http.corsdomain="*" \
  --http.vhosts="*" \
  --http.addr=0.0.0.0 \
  --http.port=5545 \
  --http.api=web3,debug,eth,txpool,net,engine \
  --ws \
  --ws.addr=0.0.0.0 \
  --ws.port=5546 \
  --ws.origins="*" \
  --ws.api=debug,eth,txpool,net,engine \
  --syncmode=full \
  --gcmode=archive \
  --nodiscover \
  --maxpeers=0 \
  --networkid=901 \
  --port=30503 \
  --authrpc.vhosts="*" \
  --authrpc.addr=0.0.0.0 \
  --authrpc.port=5551 \
  --authrpc.jwtsecret=./jwt.txt \
  --rollup.disabletxpoolgossip=true
```

Now, navigate to the parent directory in preparation for the next steps.

### Starting the OP Node

Enter the Optimism monorepo and execute the following commands to start op-node as a validator:

```bash
make op-node

./op-node/bin/op-node \
  --l2=http://localhost:5551 \
  --l2.jwt-secret=./ops-bedrock/test-jwt-secret.txt \
  --sequencer.enabled=false \
  --p2p.disable \
  --verifier.l1-confs=4 \
  --rollup.config=.devnet/rollup.json \
  --rpc.addr=0.0.0.0 \
  --rpc.port=9945 \
  --rpc.enable-admin \
  --l1=$L1_RPC_URL \
  --l1.rpckind=$L1_RPC_KIND \
  --l1.beacon=http://localhost:3602 \
  --l1.beacon-archiver http://localhost:6678
```

**Note:**
1. P2P is disabled so that it can only sync data from L1.
2. The L1 beacon URL is directed to the beacon proxy, where blobs expire quickly.
3. The beacon archiver is configured to point to the es-node archive service. 

## Verifying Sync Status


During the synchronization process, you can check that the validator node queries expired blobs from the es-node archive service. You may need to temporarily stop the validator node to allow the blobs to expire.

Additionally, you can verify the correctness of the expired blob data by ensuring that the synced L2 blocks are identical across both nodes.

For example:
```
# query block from the sequancer
cast block 3000 -f hash -r http://127.0.0.1:9545

# query block from the validator
cast block 3000 -f hash -r http://127.0.0.1:5545
```

## Conclusion

By following the instructions above, you will successfully configure and deploy each component of the OP Stack rollup with the EthStorage archive service as its long-term data availability solution. Additionally, you will verify that the EthStorage archive service functions accurately providing correct blob data after the blobs from the Beacon chain have expired. 