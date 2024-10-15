# EthStorage Archive Service Testing Document

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. It aims to verify that the functionalities meet the specified requirements and is free of critical defects.

## Test Scope

The scope of testing includes:
- Functional Testing: make sure expired blobs can be retreived from EthStorage correctly.
- Integration Testing: make sure OP Stack derivation works correclty with the retrieved blobs from EthStorage.

## Environment Setup

The tests will be conducted in an environment with the following services up and running:
- An Ethereum L1 RPC of an execution client running in archive mode
- An Ethereum L1 Beacon URL
- An EthStorage node (es-node) with the archive service enabled
- A Proxy of Ethereum Beacon RPC to mock a short blob retention period
- An OP node that uses EthStorage archive service

### Running an EthStorage node with the archive service enabled

The following steps download source code of es-node, build, init, and run an es-node as a RPC node with archive service enabled.

```
git clone https://github.com/ethstorage/es-node.git

cd es-node 

make

./init-rpc.sh

./run-rpc.sh --archiver.enabled
```

Noted that an Ethereum EL RPC in archive mode and an Ethereum Beacon URL are used implicitly in the shell scripts in below command.


For details and more options to run an es-node, please refer to  https://docs.ethstorage.io/storage-provider-guide/tutorials.

### Running a Proxy of Ethereum Beacon URL

```
git clone https://github.com/ethstorage/beacon-api-wrapper.git

cd beacon-api-wrapper

go run cmd/main.go 
```

The Proxy works just like a normal Beacon API except the blobs retension period is much shorter, like 3 hours by default. This means when a `blob_sidecars` request (`/eth/v1/beacon/blob_sidecars/{block_id}`)  asking for blobs of more than 900 slot ago, it will return an empty list: `{"data":[]}`.

For details and more options please refer to https://github.com/ethstorage/beacon-api-wrapper#beacon-api-wrapper.

### Running an OP Node

Following the [OP documentation](https://docs.optimism.io/builders/node-operators/tutorials/testnet) to run an op-node from source, and use the following options to make sure that
- the L1 Beacon URL points to the Proxy, and 
- the archive service points to the es-node RPC. 

For example, 
```
--l1.beacon http://65.108.236.27:3600 --l1.beacon-archiver http://65.108.236.27:9645
```

## Test Cases

### Test Case 1: Load an expired blob from EthStorage
- **Objective**: Verify that blobs that is expired by the Beacon Chain can be retrieved from EthStorage archive service.
- **Steps**: 
	1. Upload a blob aimed to be archived by EthStorage. (You may need [eth-blob-uploader](#eth-blob-uploader) for this.)
    2. Record the slot as block_id of the blob for later use.
    3. Query `/eth/v1/beacon/blob_sidecars/{block_id}` from the Beacon Chain URL. For example,
    ```
    curl -X 'GET'   'http://88.99.30.186:3500/eth/v1/beacon/blob_sidecars/6103987'   -H 'accept: application/json' 

    ```
	4. Wait for the blob expires and the above query returns `{"data":[]}`.
    5. Query `/eth/v1/beacon/blob_sidecars/{block_id}` from the EthStorage archive service. For example,
    ```
    curl -X 'GET'   'http://65.108.236.27:9645/eth/v1/beacon/blob_sidecars/6103987'   -H 'accept: application/json' 
    ```
- **Expected Result**: The result of step 3 and 5 are the same.

### Test Case 2: 


## Tools

### eth-blob-uploader

`eth-blob-uploader` is a handy tool to upload EIP-4844 blobs to Ethereum.

The following command demostrates how to use `eth-blob-uploader` to upload a local file `hello.txt` to EthStorage testnet as a blob by calling the function `putBlob(bytes32 key,uint256 blobIdx,uint256 length)` of the storage contract deployed on Sepolia.
```
eth-blob-uploader -r http://88.99.30.186:8545 \
-p <private-key> \
-f hello.txt \
-t 0x804C520d3c084C805E37A35E90057Ac32831F96f \
-v 1500000000000000 \
-d 0x4581a9201c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000
```
Record the transaction hash so that you can find on [EtherScan](https://sepolia.etherscan.io/) which slot the blob is belong to.

Refer to https://github.com/ethstorage/eth-blob-uploader for more details.

## Conclusion
This document serves as the foundation for the software testing framework and process of the blob archiver service. It provides a clear structure for the testing activities to ensure quality outcomes.
