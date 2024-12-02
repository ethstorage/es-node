# Testing EthStorage Archive Service 

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. Its main objective is to verify that the EIP-4844 blobs can be effectively downloaded, stored, and retrieved from EthStorage, even after they have expired and been pruned from the L1 Beacon client.

## Environment Setup

The tests will be conducted in an environment with the following services operational:

- An Ethereum L1 (Sepolia) RPC from an Execution Client running in archive mode.
- An Ethereum L1 (Sepolia) Beacon API URL.
- An EthStorage node (es-node) with the archive service enabled.
- A proxy for the Ethereum Beacon API to simulate a short blob retention period.

The following sections detail the setup of the es-node and the Beacon proxy.

### Running an es-node with the Archive Service Enabled

Follow these steps to download the source code for es-node, build it, initialize it, and run it with the archive service enabled:

```bash
git clone https://github.com/ethstorage/es-node.git
cd es-node 
make
./init-rpc.sh
./run-rpc.sh --archiver.enabled
```

Note: The default port for the Archive Service RPC is 9645.

For additional details and options for running an es-node, please refer to the [EthStorage documentation](https://docs.ethstorage.io/storage-provider-guide/tutorials).

### Running a Proxy for the Ethereum Beacon API

To download and run a proxy for the Beacon client, execute the following commands:

```bash
git clone https://github.com/ethstorage/beacon-api-wrapper.git
cd beacon-api-wrapper
go run cmd/main.go -r 1800
```

This proxy functions like a standard Beacon API, except that it has a much shorter blob retention period—30 minutes in this case. Consequently, when a `blob_sidecars` request is made for blobs older than 150 slots, it will return an empty list: `{"data":[]}`.

Note: The default RPC port for the mocked Beacon API is 3600.

### Set Environment Variables

Set the following environment variables for later use:

```bash
export RPC_URL="http://88.99.30.186:8545"
export BEACON_API="http://88.99.30.186:3500"
export BEACON_API_MOCK="http://65.108.236.27:3600"
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

To find the slot value to be used in the Beacon API, use the following script. Be sure to replace `$TX_HASH` with the transaction hash obtained in the previous step:

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

Make sure to record the KZG commitments for verification later.

### Load the Expired Blob from EthStorage

To check for blobs using the Beacon Chain URL, replace `$SLOT` with the slot number obtained in the last step:

```bash
curl -s "$BEACON_API_MOCK/eth/v1/beacon/blob_sidecars/$SLOT"
```

After waiting 30 minutes for the blob to expire, the above query should return `{"data":[]}`.

Next, query the blobs from the EthStorage archive service:

```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT"
```

This will return the blob, including the index, blob data, KZG commitment, and KZG proofs.

### Verify the Blob
To specifically query the KZG commitment, execute:

```bash
curl -s "$ARCHIVE_SERVICE/eth/v1/beacon/blob_sidecars/$SLOT" | jq -r '.data[].kzg_commitment'
```

Finally, verify that the retrieved KZG commitment matches one of the KZG commitments previously obtained from the Beacon Chain. This confirms that the blob was correctly stored and retrieved from EthStorage after being pruned by L1.

## Conclusion

This document describes a test procedure for storing a blob, retrieving it after expiration from the Beacon Client, and verifying the accuracy of the blob. 