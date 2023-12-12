# Es-node Quick Start

This tutorial provides practical steps to start an es-node instance for connecting to the existing EthStorage devnet. 

For setting up a new private EthStorage testnet, please refer to this [guide](/SETUP.md). 

For a detailed explanation for es-node please consult the [README](/README.md). 

## Testnet spec

- Layer 1: [dencun-devnet-12](https://dencun-devnet-12.ethpandaops.io/)
- storage-contracts-v1: v0.1.0
- es-node: v0.1.2

## Minimum Hardware Requirements 

Please refer to [this section](/README.md/#minimum-hardware-requirements) for hardware requirements.

## System Environment
 - Ubuntu 20.04+ (tested and verified)
 - (Optional) Docker 24.0.5+ (would simplify the process)
 - (Optional) Go 1.20.* (not compatible with Go 1.21 yet) and Node.js 16+ (can be installed following the [steps](#install-dependencies))

You can choose [the method of running es-node](#options-to-run-es-node) based on your current environment.

_Note: The steps assume the use of the root user for all command line operations. If using a non-root user, you may need to prepend some commands with "sudo"._

## Preparing miner and signer account

It is recommended to prepare two Ethereum accounts specifically for this test. One of these accounts should contain a balance of test ETH to be used as a transaction signer.

The test ETH can be requested from [https://faucet.dencun-devnet-12.ethpandaops.io/](https://faucet.dencun-devnet-12.ethpandaops.io/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Options for running es-node

You can run es-node from a pre-built Docker image, a pre-built executable, or from the source code.

 - If you have Docker version 24.0.5 or above installed, the quickest way to get started is by [using a pre-built Docker image](#from-a-docker-image). However, this approach sacrifices the convenience of modifying configurations.

 - If you choose [the pre-built es-node executable](#from-pre-built-executables), you will need to manually install some dependencies such as Node.js, snarkjs, etc. 

 - If you prefer to build [from the source code](#from-source-code), you will also need to install Go. 

## From pre-built executables

Before running es-node from the pre-built executables, ensure that you have installed [Node.js](#install-nodejs) and [snarkjs](#install-snarkjs).

Download the pre-built package suitable for your platform:

Linux x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.2/es-node.v0.1.2.linux-amd64.tar.gz | tar -xz
```
MacOS x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.2/es-node.v0.1.2.darwin-amd64.tar.gz | tar -xz
```
MacOS ARM64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.2/es-node.v0.1.2.darwin-arm64.tar.gz | tar -xz
```
Run es-node
```
cd es-node.v0.1.2
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
```

## From a Docker image

Run an es-node container in one step:
```sh
docker run --name es  -d  \
          -v ./es-data:/es-node/es-data \
          -e ES_NODE_STORAGE_MINER=<miner> \
          -e ES_NODE_SIGNER_PRIVATE_KEY=<private_key> \
          -p 9545:9545 \
          -p 9222:9222 \
          -p 30305:30305/udp \
          --entrypoint /es-node/run.sh \
          ghcr.io/ethstorage/es-node
```

You can check docker logs using the following command:
```sh
docker logs -f es 
```
## From source code

You will need to [install Go](#install-go) to build es-node from source code, and install [Node.js](#install-nodejs) and [snarkjs](#install-snarkjs) to run es-node.

Download source code and switch to the latest release branch:
```sh
git clone https://github.com/ethstorage/es-node.git
cd es-node
git checkout v0.1.2
```
Build es-node:
```sh
make
```

Start es-node
```sh
chmod +x run.sh && env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
```

With source code, you also have the option to build a Docker image by yourself and run an es-node container:

```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> docker compose up 
```
If you want to run Docker container in the background and keep all the logs:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run-docker.sh
```


## Install dependencies

_Please note that not all steps in this section are required; they depend on your [choice](#options-for-running-es-node)._

### Install Go

Download a stable Go release, e.g., go1.20.10 (can't be built on go1.21.* yet). 
```sh
curl -OL https://golang.org/dl/go1.20.10.linux-amd64.tar.gz
```
Extract and install
```sh
tar -C /usr/local -xf go1.20.10.linux-amd64.tar.gz
```
Update `$PATH`
```
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.profile
source ~/.profile
```

### Install Node.js

Install Node Version Manager
```sh
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | bash
```
Close and reopen your terminal to start using nvm or run the following to use it now:
```sh
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion
```
Choose a Node.js version above 16 (e.g. v20.*) and install 
```sh
nvm install 20
```
Activate the Node.js version
```sh
nvm use 20
```
### Install snarkjs
```sh
npm install -g snarkjs@0.7.0
```