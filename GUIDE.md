# Es-node Quick Start

This tutorial provides practical steps to start an es-node instance for connecting to the existing EthStorage devnet. 

For setting up a new private EthStorage testnet, please refer to this [guide](/SETUP.md). 

For a detailed explanation for es-node please consult the [README](/README.md). 

## Testnet spec

- Layer 1: [dencun-devnet-12](https://dencun-devnet-12.ethpandaops.io/)
- storage-contracts-v1: v0.1.0
- es-node: v0.1.4

## Minimum Hardware Requirements 

Please refer to [this section](/README.md/#minimum-hardware-requirements) for hardware requirements.

## System Environment
 - Ubuntu 20.04+ (tested and verified)
 - (Optional) Docker 24.0.5+ (would simplify the process)
 - (Optional) Go 1.20+ (can be installed following the [steps](#install-dependencies))

You can choose [the method of running es-node](#options-to-run-es-node) based on your current environment.

_Note: The steps assume the use of the root user for all command line operations. If using a non-root user, you may need to prepend some commands with "sudo"._

## Preparing miner and signer account

It is recommended to prepare two Ethereum accounts specifically for this test. One of these accounts should contain a balance of test ETH to be used as a transaction signer.

The test ETH can be requested from [https://faucet.dencun-devnet-12.ethpandaops.io/](https://faucet.dencun-devnet-12.ethpandaops.io/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Options for running es-node

You can run es-node from a pre-built Docker image, a pre-built executable, or from the source code.

 - [The pre-built es-node executable](#from-pre-built-executables) is recommended if you will run es-node on Linux or MacOS.

 - If you already have Docker installed, or have to run es-node on Windows, you can [use a pre-built Docker image](#from-a-docker-image).
 
 - If you prefer to build [from the source code](#from-source-code), you will need to install Go.

## From pre-built executables

Download the pre-built package suitable for your platform:

Linux x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.4/es-node.v0.1.4.linux-amd64.tar.gz | tar -xz
```
MacOS x86-64 or AMD64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.4/es-node.v0.1.4.darwin-amd64.tar.gz | tar -xz
```
MacOS ARM64:
```sh
curl -L https://github.com/ethstorage/es-node/releases/download/v0.1.4/es-node.v0.1.4.darwin-arm64.tar.gz | tar -xz
```
Run es-node
```
cd es-node.v0.1.4
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
          ghcr.io/ethstorage/es-node:v0.1.4
```

You can check docker logs using the following command:
```sh
docker logs -f es 
```
## From source code

You will need to [install Go](#install-go) to build es-node from source code.

When using Ubuntu, it is necessary to [verify whether `build-essential` and `libomp-dev` are installed](#install-rapidsnark-dependencies).

Download source code and switch to the latest release branch:
```sh
git clone https://github.com/ethstorage/es-node.git
cd es-node
git checkout v0.1.4
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

### Install RapidSNARK dependencies

Check if `build-essential` and `libomp-dev packages` are installed on your Ubuntu system:

```
dpkg -l | grep build-essential
dpkg -l | grep libomp-dev
```
Install the build-essential and libomp-dev packages if no information printed:

```
apt update
apt install build-essential
apt install libomp-dev
```


## FAQ

### Can I change the data file location?

Yes, you can. 

By default, when executed, the run.sh script will generate a data file named `shard-0.dat` to store shard 0 in the data directory specified by `--datadir`, located in the same folder as the run.sh script.

If necessary, you can choose an alternative location for data storage by specifying the full path of the file as the value of the `--storage.files` flag in the run.sh script.

Please refer to [configuration](/README.md#configuration) for more details.
