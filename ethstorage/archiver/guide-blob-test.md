
# Testing EthStorage Archive Service 

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. It aims to verify that the EIP-4844 blobs can be downloaded and stored by EthStorage, and also can be retrieved from EthStorage correctly while expired and pruned from the L1 Beacon client.

## Environment Setup

The tests will be conducted in an environment with the following services up and running:
- An Ethereum L1 (Sepolia) RPC of an Execution Client running in archive mode
- An Ethereum L1 (Sepolia) Beacon URL
- An EthStorage node (es-node) with the archive service enabled
- A proxy of Ethereum Beacon URL to mock a short blob retention period

The following instructions are about how es-node and the Beacon proxy are set up:

### Running an es-node with the Archive Service Enabled

Execute the following steps to download the source code of es-node, build, initialize, and run an es-node with the archive service enabled.

```bash
git clone https://github.com/ethstorage/es-node.git
cd es-node 
make

./init-rpc.sh

./run-rpc.sh --archiver.enabled
```
Note that Archive service RPC port are defaulted to 9645.

For details and more options to run an es-node, please refer to the [EthStorage documentation](https://docs.ethstorage.io/storage-provider-guide/tutorials).

### Running a Proxy of Ethereum Beacon URL

Execute the following commands to download and run the proxy of Beacon client:
```bash
git clone https://github.com/ethstorage/beacon-api-wrapper.git

cd beacon-api-wrapper

go run cmd/main.go -r 1200
```

The proxy works just like a normal Beacon API except the blobs retension period is much shorter, like 20 minutes in this case. This means when a `blob_sidecars` request (`/eth/v1/beacon/blob_sidecars/{block_id}`)  asking for blobs of more than 900 slot ago, it will return an empty list: `{"data":[]}`.

The RPC port defaults to 3600.

For details and more options please refer to this [repo](https://github.com/ethstorage/beacon-api-wrapper#beacon-api-wrapper).

### Set Environment Variables

Set the environment variables for later use:
```
export RPC_URL="http://88.99.30.186:8545"
export BEACON_API="http://88.99.30.186:3500"
export BEACON_API_MOCK="http://65.108.236.27:3600"
export ARCHIVE_SERVICE="http://65.108.236.27:9645"
```

## Testing EthStorage Archive Service
The general idea is like this: we upload a blob to L1 using a blob transaction, make sure it is uploaded successfully by querying it from the Beacon URL and the proxy. After the mocked retention perod of time when the blob "expired", we cannot query it from the proxy anymore but we can retrieve the blob from EthStorage archive serivce using same query. Then we verify that the blob we retrieved are the same.

### Upload a Blob

`eth-blob-uploader` is a handy tool to upload EIP-4844 blobs to Ethereum. If you have not installed it, execute the following command to install it:
```bash
npm install -g eth-blob-uploader
```
The following command uses `eth-blob-uploader` to upload a local file `hello.txt` to EthStorage testnet as an EIP-4844 blob by calling the function `putBlob(bytes32 key,uint256 blobIdx,uint256 length)` of the storage contract deployed on Sepolia.

```sh
eth-blob-uploader -r $RPC_URL \
-p <private-key> \
-f hello.txt \
-t 0x804C520d3c084C805E37A35E90057Ac32831F96f \
-v 1500000000000000 \
-d 0x4581a9201c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000

0xa0a9ad94c0c8facf5b4ba7dd1446ede21e4794aa88b7dcc3366134e5e9ebab9b
```
Record the transaction hash for later use.

Options explained:
-r	provider url
-p private key
-f	file path
-t	address
-v	amount of eth as storage fee
-d calldata to call a contract

### Query the blob info
 Find the slot value which will be used in the Beacon API using the following script, replacing the value of `$TX_HASH` with the one in the last step.

```bash
#!/bin/bash

# Replace the transaction hash with yours
TX_HASH="0xa0a9ad94c0c8facf5b4ba7dd1446ede21e4794aa88b7dcc3366134e5e9ebab9b"
GENESIS_TIMESTAMP=1655733600

# Get the block number from the transaction
BLOCK_NUMBER=$(cast tx $TX_HASH -r $RPC_URL | grep "blockNumber" | head -n 1 | awk '{print $2}')
if [ -z "$BLOCK_NUMBER" ]; then
    echo "Failed to retrieve block number."
    exit 1
fi
# Get the block timestamp from the block number
TIMESTAMP=$(cast block $BLOCK_NUMBER -f timestamp -r $RPC_URL)
if [ -z "$TIMESTAMP" ]; then
    echo "Failed to retrieve block timestamp."
    exit 1
fi
# Calculate the slot number
export SLOT=$(( (TIMESTAMP - GENESIS_TIMESTAMP) / 12 ))
echo "Slot: $SLOT"
# Load blob info from the Beacon API
echo "KZG Commitments of blobs in the slot:"
curl -s "$BEACON_API/eth/v1/beacon/blob_sidecars/$SLOT" | jq -r '.data[].kzg_commitment'
```
You may record the KZG commentments for verification later.

### Load the expired blob from EthStorage
Query blobs by from the Beacon Chain URL, replacing `$SLOT` with the slot number in the last step. 
```bash
curl "$BEACON_API_MOCK/eth/v1/beacon/blob_sidecars/$SLOT"
```
Wait for the blob to expire and the above query to return `{"data":[]}`.

Query blobs from the EthStorage archive service:
```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT"
```
You can see the blob retrieved with index, blob data, KZG commitment, and KZG proofs.

You can query the KZG commitment specifically by executing:
```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT" | jq -r '.data[].kzg_commitment'
```
And verify that the KZG Commitment is included in one of the KZG Commitments from the Beacon Chain before.  This means the blob has been stored and retrieved correclty from EthStorage that has been pruned by L1. 

## Conclusion
This document reproduce a test to store a blob and retrieve it after the Beacon Client expired it, and also verifies the correctness of the blob.