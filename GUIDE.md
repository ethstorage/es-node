# Es-node Quick Start

This tutorial provides practical steps to start an es-node instance for connecting to the existing EthStorage devnet. 

For setting up a new private EthStorage testnet, please refer to this [guide](/SETUP.md). 

For a detailed explanation for es-node please consult the [README](/README.md). 

## Testnet spec

- Layer 1: [Goerli](https://goerli.net/)
- storage-contracts-v1: v0.1.0
- es-node: v0.1.5

## Minimum Hardware Requirements 

Please refer to [this section](/README.md/#minimum-hardware-requirements) for hardware requirements.

## System Environment
 - Ubuntu 20.04+ (tested and verified)
 - (Optional) Docker 24.0.5+ (would simplify the process)
 - (Optional) Go 1.20+ and Node.js 16+ (can be installed following the [steps](#install-dependencies))

You can choose [the method of running es-node](#options-to-run-es-node) based on your current environment.

_Note: The steps assume the use of the root user for all command line operations. If using a non-root user, you may need to prepend some commands with "sudo"._

## Preparing miner and signer account

It is recommended to prepare two Ethereum accounts specifically for this test. One of these accounts should contain a balance of test ETH to be used as a transaction signer.

The test ETH can be requested from [https://goerlifaucet.com/](https://goerlifaucet.com/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Options for running es-node

You can run es-node from a pre-built Docker image, a pre-built executable, or from the source code.

 - If you choose [the pre-built es-node executable](#from-pre-built-executables), you will need to manually install some dependencies such as Node.js and snarkjs.

 - If you have Docker version 24.0.5 or above installed, the quickest way to get started is by [using a pre-built Docker image](#from-a-docker-image).
 
 - If you prefer to build [from the source code](#from-source-code), you will also need to install Go besides Node.js and snarkjs.

## From pre-built executables

Before running es-node from the pre-built executables, ensure that you have installed [Node.js](#install-nodejs) and [snarkjs](#install-snarkjs).

Download the pre-built package suitable for your platform:

Linux x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.5/es-node.v0.1.5.linux-amd64.tar.gz | tar -xz
```
MacOS x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.5/es-node.v0.1.5.darwin-amd64.tar.gz | tar -xz
```
MacOS ARM64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.5/es-node.v0.1.5.darwin-arm64.tar.gz | tar -xz
```
Run es-node
```
cd es-node.v0.1.5
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
          -p 9222:9222/udp \
          --entrypoint /es-node/run.sh \
          ghcr.io/ethstorage/es-node:v0.1.5
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
git checkout v0.1.5
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

Download a stable Go release, e.g., go1.21.4. 
```sh
curl -OL https://golang.org/dl/go1.21.4.linux-amd64.tar.gz
```
Extract and install
```sh
tar -C /usr/local -xf go1.21.4.linux-amd64.tar.gz
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
npm install -g snarkjs
```

## Two phases after es-node launch

After the launch of ES node, it basically goes through two main stages.

### Data sync phase

The es-node will synchronize data from other peers in the network. You can check from the console the number of peers the node is connected to and, more importantly, the estimated syncing time.

During this phase, the CPUs are expected to be fully occupied for data processing. If not, please refer to [the FAQ](#how-to-tune-the-performance-of-syncing) for performance tuning on this area.

A typical log entry in this phase appears as follows:

```
INFO [01-18|09:13:32.564] Storage sync in progress progress=85.50% peerCount=2 syncTasksRemain=1@0 blobsSynced=1@128.00KiB blobsToSync=0 fillTasksRemain=30 emptyFilled=3,586,176@437.77GiB emptyToFill=608,127   timeUsed=1h48m7.556s  eta=18m20.127s

```
### Sampling phase 

Once data synchronization is complete, the es-node will enter the sampling phase, also known as mining.

When you see "The nonces are exhausted in this slot...", it indicates that your node has successfully completed all the sampling tasks within a slot. 
The "samplingTime" value informs you of the duration, in seconds, it took to complete the sampling process.

If the es-node doesn't have enough time to complete sampling within a slot, the log will display "Mining tasks timed out". For further actions, please refer to [the FAQ](#what-can-i-do-about-mining-tasks-timed-out).

A typical log entry of sampling during a slot looks like this:

```
INFO [01-18|11:52:18.766] Mining info retrieved                    shard=0 LastMineTime=1,705,545,915 Difficulty=0 proofsSubmitted=2
INFO [01-18|11:52:18.766] Mining tasks assigned                    shard=0 threads=64 block=10,394,055 nonces=1,048,576
INFO [01-18|11:52:21.539] The nonces are exhausted in this slot, waiting for the next block samplingTime=2.8s shard=0 block=10,394,055

```

## FAQ

### Can I change the data file location?

Yes, you can. 

By default, when executed, the run.sh script will generate a data file named `shard-0.dat` to store shard 0 in the data directory specified by `--datadir`, located in the same folder as the run.sh script.

If necessary, you can choose an alternative location for data storage by specifying the full path of the file as the value of the `--storage.files` flag in the run.sh script.

Please refer to [configuration](/README.md#configuration) for more details.

### How to tune the performance of syncing? 

To enhance syncing performance, you can adjust the values of the p2p.sync.concurrency and p2p.fill-empty.concurrency flags.

- The `--p2p.sync.concurrency` flag determines the number of threads that simultaneously retrieve shard data, with a default setting of 16.
- The` --p2p.fill-empty.concurrency` flag specifies the number of threads used to concurrently fill encoded empty blobs, with a default setting of the CPU number minus 2.

### What can I do about "Mining tasks timed out"? 

When you see the message "Mining tasks timed out", it indicates that the sampling task couldn't be completed within a slot. 

You can check the IOPS of the disk to determine if the rate of data read has reached the IO capacity. 
If not, using the flag `--miner.threads-per-shard` can specify the number of threads to perform sampling for each shard, thereby helping in accomplishing additional sampling.
