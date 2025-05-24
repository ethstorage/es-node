# Testing EthStorage Archive Service 

## Table of Contents

1. [Introduction](#introduction)  
2. [Environment Setup](#environment-setup)  
   2.1. [Running a Proxy for the Ethereum Beacon API](#running-a-proxy-for-the-ethereum-beacon-api)  
   2.2. [Running an es-node with the Archive Service Enabled](#running-an-es-node-with-the-archive-service-enabled)  
   2.3. [Set Environment Variables](#set-environment-variables)  
3. [Testing EthStorage Archive Service](#testing-ethstorage-archive-service)  
   3.1. [Upload a Blob](#upload-a-blob)  
   3.2. [Query the Blob Info](#query-the-blob-info)  
   3.3. [Load the Expired Blob from EthStorage](#load-the-expired-blob-from-ethstorage)  
   3.4. [Verify the Blob](#verify-the-blob)  
4. [Conclusion](#conclusion)  

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. Its main objective is to verify that the EIP-4844 blobs can be effectively downloaded, stored, and retrieved from EthStorage, even after they have expired and been pruned from the L1 Beacon client.

## Environment Setup

The tests will be conducted in an environment with the following services operational:

- An Ethereum L1 (Sepolia) RPC from an Execution Client running in archive mode.
- An Ethereum L1 (Sepolia) Beacon API URL.
- A proxy for the Ethereum Beacon API to simulate a short blob retention period.
- An EthStorage node (es-node) with the archive service enabled.

The following sections detail the setup of the Beacon proxy and the es-node.

### Running a Proxy for the Ethereum Beacon API

To download and run a proxy for the Beacon client, execute the following commands:

```bash
git clone https://github.com/ethstorage/beacon-api-wrapper.git
cd beacon-api-wrapper
go run cmd/main.go -r 3 -b http://88.99.30.186:3500
```

This proxy functions like a standard Beacon API, except that it has a much shorter blob retention period - 3 epochs or 96 slots in this case. Consequently, when a `blob_sidecars` request is made for blobs older than this time frame, it will return an empty list: `{"data":[]}`.

Note: The default RPC port for the mocked Beacon API is 3600.

### Running an es-node with the Archive Service Enabled

Follow these steps to download the source code for es-node, build it, initialize it, and run it with the archive service enabled:

```bash
git clone https://github.com/ethstorage/es-node.git
cd es-node 
make
./init-rpc.sh
./run-rpc.sh --l1.beacon http://localhost:3600 --archiver.enabled
```

Note: The default port for the Archive service is 9645.

For additional details and options for running an es-node, please refer to the [EthStorage documentation](https://docs.ethstorage.io/storage-provider-guide/tutorials).

### Set Environment Variables

Set the following environment variables for later use:

```bash
export RPC_URL="http://88.99.30.186:8545"
export BEACON_API="http://88.99.30.186:3500"
export BEACON_API_MOCK="http://localhost:3600"
export ARCHIVE_SERVICE="http://65.108.236.27:9645"
```

## Testing EthStorage Archive Service

The overall testing approach involves uploading a file to L1 via a blob transaction, confirming its successful upload by querying the Beacon API and the proxy. After the mocked retention period has elapsed and the blob has "expired," it will no longer be queryable from the proxy, but it can still be retrieved from the EthStorage archive service. Finally, we will verify that the retrieved blob matches the originally uploaded data.

### Upload a Blob

The `eth-blob-uploader` is a tool designed for uploading EIP-4844 blobs to Ethereum. If you haven't installed it yet, run the following command:

```bash
npm install -g eth-blob-uploader
```

To upload a local file (`hello.txt`) to the EthStorage testnet as an EIP-4844 blob, use `eth-blob-uploader` by executing the following command. This command calls the function `putBlob(bytes32 key, uint256 blobIdx, uint256 length)` from the storage contract deployed on Sepolia:

```bash
eth-blob-uploader -r $RPC_URL \
-p <private-key> \
-f hello.txt \
-t 0x804C520d3c084C805E37A35E90057Ac32831F96f \
-v 1500000000000000 \
-d 0x4581a9201c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000

0xa0a9ad94c0c8facf5b4ba7dd1446ede21e4794aa88b7dcc3366134e5e9ebab9b
```

Please record the transaction hash for future reference.

Options explained:

- `-r`: Provider URL
- `-p`: Private key
- `-f`: File path
- `-t`: Contract address
- `-v`: Amount of ETH for storage fee
- `-d`: Calldata for contract call

### Query the Blob Info

To find the slot value to be used in the Beacon API, use the following script. It also help to log the KZG commitments of the blobs for verification purpose.

Be sure to replace `$TX_HASH` with the transaction hash obtained in the previous step:

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

The output may look like this:

```log
Slot: 6448199
KZG Commitments of blobs in the slot:
0x8893c579b22c64b81700a3d781cb78327e16ae687afa276c9cac4b7d9921c78577c0461e223ad3ca6663f6898fdf8e96
0x8e43d61888613865f5b54a5345b997c830237e116426c7eb779da4bf33ff0e2240fd56a54291607d5c52212f53842f23
0x8f7dfdaf4565296c533592c7293af91bd6544e5c2e2d011c402c945d4718950ea15ae1c0fc4f241416e5a9ade9ea748e
0xad7d15db45e493072105f0297fcf4226b1cc54bc4da2fcb491bce31535f5a04d55fd1ed1e728a732189d3dc7cffc8014
```

Now set `$SLOT` as environment variable for blob queries later:
```bash
export SLOT=6448199 # replace the value with yours
```

Also record the KZG commitments for verification later, for one of the KZG Commitments is associated with the blob we just uploaded.

### Waiting for the Blob Expires on the Beacon

To check for blobs' availability on the Beacon Chain, using the command:

```bash
curl -s "$BEACON_API_MOCK/eth/v1/beacon/blob_sidecars/$SLOT"
```

After waiting 30 minutes for the blob to expire, the above query should return `{"data":[]}`.


### Load the Expired Blob from EthStorage

Next, query the blob from the EthStorage archive service:

```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT"
```

This will return the expired blob, including the index, blob data, KZG commitment, and KZG proofs.

### Verify the Blob

Finally, verify that the retrieved KZG commitment matches one of the KZG commitments previously obtained from the Beacon Chain. This confirms that the blob was correctly stored and retrieved from EthStorage after being pruned by L1.

To specifically query the KZG commitment, execute:

```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT" | jq -r '.data[].kzg_commitment'
```

## Conclusion

This document describes a test procedure for storing a blob, retrieving it after expiration from the Beacon Client, and verifying the accuracy of the blob. 
