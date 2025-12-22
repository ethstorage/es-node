# Testing EthStorage Archive Service 

## Table of Contents

1. [Introduction](#introduction)  
2. [Environment Setup](#environment-setup)  
   2.1. [Tools Installation](#tools-installation)  
   2.2. [Running an es-node with the Archive Service Enabled](#running-an-es-node-with-the-archive-service-enabled)  
   2.3. [Set Environment Variables](#set-environment-variables)  
3. [Testing EthStorage Archive Service](#testing-ethstorage-archive-service)  
   3.1. [Upload a Blob](#upload-a-blob)  
   3.2. [Query the Slot Number](#query-the-slot-number)  
   3.3. [Query Versioned Hash of the Blob](#query-versioned-hash-of-the-blob)  
   3.4. [Load and Verify the Blob](#load-and-verify-the-blob)  
4. [Conclusion](#conclusion)  

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. Its main objective is to verify that the EIP-4844 blobs can be effectively downloaded, stored, and retrieved from EthStorage.

## Environment Setup

### Tools Installation

Required tools and installation links:

- ethfs-cli (https://www.npmjs.com/package/ethfs-cli)
- foundry cast (https://getfoundry.sh/introduction/installation/)

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
# Ethereum L1 (Sepolia) RPC from an Execution Client running in archive mode
export RPC_URL="http://65.108.230.142:8545" 
# Ethereum L1 (Sepolia) Beacon API URL
export BEACON_API="http://65.108.230.142:3500"
export ARCHIVE_API="http://65.109.50.145:9645"
```
## Testing EthStorage Archive Service

The overall testing approach involves uploading a file to EthStorage via a blob transaction, capturing the parameters needed to query both the Beacon API and the EthStorage archive API, and then confirming that the blob data returned by both sources matches.

### Upload a Blob

To upload a local file (`hello.txt`) to the EthStorage testnet as an EIP-4844 blob, use the following command which calls the function `putBlob(bytes32 key, uint256 blobIdx, uint256 length)` of the storage contract deployed on Sepolia:

```bash
ethfs-cli upload \
    -f hello.txt \
    -a 0x2351551Ed568f45130eF17cD7118097F7859E03C \
    -c 11155111 \
    -p <private_key> \
    -t blob

0xb82dca74e0c743f013e20867079fac7243c96cbc303cc23d73d443aa01585dad
```

Please record the transaction hash for future reference.

Options explained:

- `-f`: File path
- `-a`: Address of a FlatDictionary contract pre-deployed
- `-c`: Chain ID
- `-t`: Upload type: `calldata` or `blob`
- `-p`: Private key

### Query the Slot Number

To find the slot value to be used in the Beacon API, use the following script. Replace `$TX_HASH` with the transaction hash obtained in the previous step:

```bash
#!/bin/bash

# Replace the transaction hash with yours
TX_HASH="0xb82dca74e0c743f013e20867079fac7243c96cbc303cc23d73d443aa01585dad"
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
```

### Query Versioned Hash of the Blob

With block explorer, you can find the versioned hash of the blob you just uploaded through this link:
```
"https://sepolia.etherscan.io/tx/$TX_HASH#blobs"
```
Export the value for later use:
```bash
export VERSIONED_HASH=0x0158420bd7b1f3c04a097694235c52659f31b479018ac56b2545727fab33712d
```

### Load and Verify the Blob

With the slot number and versioned hash, you can query the blob from the EthStorage archive service even after it is pruned by L1.
```bash
curl -s "$ARCHIVE_API/eth/v1/beacon/blobs/$SLOT?versioned_hash=$VERSIONED_HASH"
```

Finally, verify that the blob retrieved matches the one obtained from the Beacon Chain:

```bash

[ "$(curl -s "$BEACON_API/eth/v1/beacon/blobs/$SLOT?versioned_hash=$VERSIONED_HASH" | jq -r '.data[0]') " \
 = "$(curl -s "$ARCHIVE_API/eth/v1/beacon/blobs/$SLOT?versioned_hash=$VERSIONED_HASH" | jq -r '.data[0]') " ] && echo "✅ Match" || echo "❌ Mismatch"
 ```

This confirms that the blob was correctly stored and retrieved from EthStorage.

## Conclusion

This document describes a test procedure for storing a blob, retrieving it from the Beacon Client, and verifying the accuracy of the blob.
