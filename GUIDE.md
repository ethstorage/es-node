# Es-node Quick Start

This tutorial provides practical steps to start an es-node instance for connecting to the existing EthStorage devnet. 

For setting up a new private EthStorage testnet, please refer to this [guide](/SETUP.md). 

For a detailed explanation for es-node please consult the [README](/README.md). 

## Testnet spec

- Layer 1: Sepolia testnet
- storage-contracts-v1: v0.1.0
- es-node: v0.1.6

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

The test ETH can be requested from [https://sepoliafaucet.com/](https://sepoliafaucet.com/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Options for running es-node

You can run es-node from a pre-built Docker image, a pre-built executable, or from the source code.

 - If you choose [the pre-built es-node executable](#from-pre-built-executables), you will need to manually install some dependencies such as Node.js and snarkjs.

 - If you have Docker version 24.0.5 or above installed, the quickest way to get started is by [using a pre-built Docker image](#from-a-docker-image).
 
 - If you prefer to build [from the source code](#from-source-code), you will also need to install Go besides Node.js and snarkjs.

## From pre-built executables

Before running es-node from the pre-built executables, ensure that you have installed [Node.js](#install-nodejs), [snarkjs](#install-snarkjs) and [WSL](https://learn.microsoft.com/en-us/windows/wsl/) for Window. 

Download the pre-built package suitable for your platform:

Linux x86-64 or AMD64 or Windows:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.6/es-node.v0.1.6.linux-amd64.tar.gz | tar -xz
```
MacOS x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.6/es-node.v0.1.6.darwin-amd64.tar.gz | tar -xz
```
MacOS ARM64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.6/es-node.v0.1.6.darwin-arm64.tar.gz | tar -xz
```
Run es-node
```
cd es-node.v0.1.6
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
          ghcr.io/ethstorage/es-node:v0.1.6
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
git checkout v0.1.6
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
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> docker-compose up 
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

### Install wsl 
If you need to run es-node in Windows, you need to run it using WSL. See [this](https://learn.microsoft.com/en-us/windows/wsl/install) to install WSL.

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

A typical log entry of sampling during a slot looks like this:

```
INFO [01-19|05:02:23.210] Mining info retrieved                    shard=0 LastMineTime=1,705,634,628 Difficulty=4,718,592,000 proofsSubmitted=6
INFO [01-19|05:02:23.210] Mining tasks assigned                    shard=0 threads=64 block=10,397,712 nonces=1,048,576
INFO [01-19|05:02:26.050] The nonces are exhausted in this slot, waiting for the next block samplingTime=2.8s shard=0 block=10,397,712

```
When you see "The nonces are exhausted in this slot...", it indicates that your node has successfully completed all the sampling tasks within a slot. 
The "samplingTime" value informs you of the duration, in seconds, it took to complete the sampling process.

If the es-node doesn't have enough time to complete sampling within a slot, the log will display "Mining tasks timed out". For further actions, please refer to [the FAQ](#what-can-i-do-about-mining-tasks-timed-out).

## FAQ

### What should I do when the log frequently shows "i/o deadline reached" during data syncing?

The timeout issue could be due to network reasons. It may be because the amount of data requested relative to network performance is too large, so even a slight delay in individual requests will be magnified.

You can change the size of each request to a smaller value using `--p2p.max.request.size`. 
The default value is `4194304`. You can try adjusting it to `1048576`.

### How to tune the performance of syncing? 

To enhance syncing performance or limit the usage of CPU power, you can adjust the values of the following related flags with higher or lower values:

- The `--p2p.sync.concurrency` flag determines the number of threads that simultaneously retrieve shard data, with a default setting of 16.
- The `--p2p.fill-empty.concurrency` flag specifies the number of threads used to concurrently fill encoded empty blobs, with a default setting of the CPU number minus 2.

### Why is the CPU fully utilized when syncing data? Does this imply that running an es-node requires high-end hardware? Additionally, is there a way to configure it to use less CPU power? I'm asking because I need to run other software on the same machine.

During data synchronization, the CPUs are fully utilized as the es-node is engaged in intensive data processing tasks like encoding and writing.

However, this high-intensity processing occurs primarily when large volumes of data need to be synchronized, such as during the initial startup of the es-node. Once the synchronization is complete, and the es-node enters the mining phase, the CPU usage decreases significantly. So, generaly speaking, the es-node does not require high-end hardware, particularly for the CPU.

If there is a need to conserve CPU power anyway, you can tune down the values of syncing performance related flags, namely `--p2p.sync.concurrency` and `--p2p.fill-empty.concurrency`. See [here](#how-to-tune-the-performance-of-syncing) for detailed information.

### What does it means when the log shows "The nonces are exhausted in this slot"

When you see "The nonces are exhausted in this slot...", it indicates that your node has successfully completed all the sampling tasks within a slot, and you do not need to do anything about it. 

### What can I do about "Mining tasks timed out"? 

When you see the message "Mining tasks timed out", it indicates that the sampling task couldn't be completed within a slot. 

You can check the IOPS of the disk to determine if the rate of data read has reached the IO capacity. 
If not, using the flag `--miner.threads-per-shard` can specify the number of threads to perform sampling for each shard, thereby helping in accomplishing additional sampling.

### How do I know whether I've got a mining reward?

You can observe the logs to see if there are such messages indicating a successful storage proof submission:

```
INFO [01-19|14:41:48.715] Mining transaction confirmed             txHash=62df87..7dbfbc
INFO [01-19|14:41:49.017] "Mining transaction success!      √"     miner=0x534632D6d7aD1fe5f832951c97FDe73E4eFD9a77
INFO [01-19|14:41:49.018] Mining transaction details               txHash=62df87..7dbfbc gasUsed=419,892 effectiveGasPrice=2,000,000,009
INFO [01-19|14:41:49.018] Mining transaction accounting (in ether) reward=0.01013071059 cost=0.0008397840038 profit=0.009290926583

```

You can also visit [the EthStorage dashboard](http://grafana.ethstorage.io/d/es-node-mining-state-dev/ethstorage-monitoring-dev?orgId=2&refresh=5m) for real-time mining statistics.

Finally, pay attention to the balance change of your miner's address which reflects the mining income.

### Can I change the data file location?

Yes, you can. 

By default, when executed, the run.sh script will generate a data file named `shard-0.dat` to store shard 0 in the data directory specified by `--datadir`, located in the same folder as the run.sh script.

If necessary, you can choose an alternative location for data storage by specifying the full path of the file as the value of the `--storage.files` flag in the run.sh script.

Please refer to [configuration](/README.md#configuration) for more details.

### What can I do about "The zkey file was not downloaded" error?

When you see the following message when running **run.sh**. you can manually download the blob_poseidon.zkey/blob_poseidon2.zkey to `./build/bin/snarkjs/` folder and run it again. 
```
zk prover mode is 2
Start downloading ./build/bin/snarkjs/blob_poseidon2.zkey...
... ...
Error: The zkey file was not downloaded. Please try again.
```