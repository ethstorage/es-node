# Es-node Quick Start
This is a practical tutorial to start an es-node instance. For a detailed explanation please refer to the [README](/README.md). 

## Testnet spec
- Layer 1: [dencun-devnet-9](https://dencun-devnet-9.ethpandaops.io/)
- storage-contracts-v1: v0.1.0
- es-node: v0.1.0

## Minimum Hardware Requirements 
 - CPU with 2+ cores
 - 4GB RAM
 - 2GB **free** storage space for the runtime and sync of one data shard
 - 8 MBit/sec download Internet service

## System Environment
 - Ubuntu 20.04+ (has been tested with)
 - (Optional) Docker 24.0.5+ (would simplify the process)
 - (Optional) go 1.20+ and node 16+ (can be installed following the [steps](#1-install-go-120-eg-v1213))

You can choose [how to run es-node](#step-3-run-es-node) according to your current environment.
## Step 1. Prepare miner and signer account
It is suggested to prepare two Ethereum accounts specifically for this test, one of which needs to have some test ETH balance to be used as a transaction signer.

The test ETH can be requested from [https://faucet-2.dencun-devnet-9.ethpandaops.io/](https://faucet-2.dencun-devnet-9.ethpandaops.io/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Step 2. Download source code
```sh
# download source code
git clone git@github.com:ethstorage/es-node.git

# go to the repo
cd es-node
```
## Step 3. Run es-node

### Option 1: With Docker compose
If you have Docker version 24.0.5 or above installed, simply run:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> docker compose up 
```
### Option 2: With Docker in the background
If you want to keep all the logs:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run-docker.sh

# check logs
docker logs -f es 
```
### Option 3: Without Docker

#### 1. Install go 1.20+ (e.g. v1.21.3)

Download a stable go release
```sh
curl -OL https://golang.org/dl/go1.21.3.linux-amd64.tar.gz
```
Extract and install
```sh
tar -C /usr/local -xvf go1.21.3.linux-amd64.tar.gz
```
Edit `~/.profile` and add the following line to the end of it.
```
export PATH=$PATH:/usr/local/go/bin
```
Next, refresh your profile by running the following command:
```sh
source ~/.profile
```
#### 2. Install node 16+ (e.g. v20.*)

Install Node Version Manager
```sh
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | bash
```
Install node using nvm
```sh
nvm install 20
```
Activate the node version
```sh
nvm use 20
```
#### 3. Install `snarkjs`
```sh
npm install -g snarkjs@0.7.0
```
#### 4. Build es-node
```sh
cd cmd/es-node && go build && cd ../..
```
#### 5. Start es-node
```sh
chmod +x run.sh && env ES_NODE_STORAGE_MINER=<miner> ES_NODE_PRIVATE_KEY=<private_key> ./run.sh
```