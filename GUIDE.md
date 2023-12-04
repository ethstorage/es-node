# Es-node Quick Start
This is a practical tutorial to start an es-node instance to connect to the existing EthStorage devnet. 

To setup a new private EthStorage testnet, please refer to this [guide](/SETUP.md). 

For a detailed explanation for es-node please refer to the [README](/README.md). 

## Testnet spec
- Layer 1: [dencun-devnet-11](https://dencun-devnet-11.ethpandaops.io/)
- storage-contracts-v1: v0.1.0
- es-node: v0.1.1

## Minimum Hardware Requirements 

Refer to [this section](/README.md/#minimum-hardware-requirements).

## System Environment
 - Ubuntu 20.04+ (has been tested with)
 - (Optional) Docker 24.0.5+ (would simplify the process)
 - (Optional) go 1.20.* (can't be built on Go 1.21 yet) and node 16+ (can be installed following the [steps](#1-install-go-120-eg-v1213))

You can choose [how to run es-node](#step-3-run-es-node) according to your current environment.

_Note: It is assumed that you are using the `root` _user in all the command_ line operations. You may need to add `sudo` before some of the commands if you are using a non-root user._

## Prepare miner and signer account
It is suggested to prepare two Ethereum accounts specifically for this test, one of which needs to have some test ETH balance to be used as a transaction signer.

The test ETH can be requested from [https://faucet.dencun-devnet-11.ethpandaops.io/](https://faucet.dencun-devnet-11.ethpandaops.io/). 

Remember to use the signer's private key (with ETH balance) to replace `<private_key>` in the following steps. And use the other address to replace `<miner>`.

## Options to run es-node

You can run es-node directly, or in a Docker container.

If you choose to run es-node directly on a machine, you need to manually install some dependencies such as nodejs, snarkjs, etc. 
If you choose to build from source code other than download pre-built, you also need to install go. 
If you choose to run es-node in a Docker container, you will have Docker version 24.0.5 or above installed. 

### Download source code

If you choose to download a pre-built binary, ignore this step.

```sh
# download source code
git clone https://github.com/ethstorage/es-node.git

# checkout branch
cd es-node
git checkout v0.1.1
```
### Option 1: With Docker compose
If you have Docker version 24.0.5 or above installed, simply run:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> docker compose up 
```
### Option 2: With Docker in the background
If you want to keep all the logs:
```sh
env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run-docker.sh

# check logs
docker logs -f es 
```
### Option 3: Without Docker

You have two options for acquiring es-node: build from source code or download pre-built.

#### 1. Install go 1.20+ (e.g. v1.20.10)

If you choose to download a pre-built binary, ignore this step.

Download a stable go release
```sh
curl -OL https://golang.org/dl/go1.20.10.linux-amd64.tar.gz
```
Extract and install
```sh
tar -C /usr/local -xf go1.20.10.linux-amd64.tar.gz
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
Close and reopen your terminal to start using nvm or run the following to use it now:
```sh
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion
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
#### 4. Build or download es-node

- Use the following command to build es-node:
```sh
cd cmd/es-node && go build && cd ../..
```
- Alternatively, you can download the pre-built executable of the specific version from  [the releases page](https://github.com/ethstorage/es-node/releases), such as es-node.v0.1.2.ubuntu, and then rename it using the following command:
```sh
mv es-node.${version}.ubuntu ./cmd/es-node/es-node
```
#### 5. Start es-node
```sh
chmod +x run.sh && env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
```