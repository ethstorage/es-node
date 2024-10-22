# EthStorage Archive Service Testing Document

## Introduction

This document outlines the testing strategy and details for the blob archiver service of EthStorage. It aims to verify that the functionalities meet the specified requirements and are free of critical defects.

## Test Scope

The scope of testing includes:
- **Functional Testing**: Ensure expired blobs can be retrieved from EthStorage correctly.
- **Integration Testing**: Ensure OP Stack derivation works correctly with the retrieved blobs from EthStorage.

## Environment Setup

The tests will be conducted in an environment with the following services up and running:
- An Ethereum L1 RPC of an execution client running in archive mode
- An Ethereum L1 Beacon URL
- An EthStorage node (es-node) with the archive service enabled
- A Proxy of Ethereum Beacon RPC to mock a short blob retention period
- An OP node that uses the EthStorage archive service as the backup source of blobs

### Running an EthStorage Node with the Archive Service Enabled

The following steps download the source code of es-node, build, initialize, and run an es-node as an RPC node with the archive service enabled.

```sh
git clone https://github.com/ethstorage/es-node.git

cd es-node 

make

./init-rpc.sh

./run-rpc.sh --archiver.enabled
```

Note that an Ethereum EL RPC in archive mode and an Ethereum Beacon URL are used implicitly in the shell scripts in the above commands.


For details and more options to run an es-node, please refer to the [EthStorage documentation](https://docs.ethstorage.io/storage-provider-guide/tutorials).

### Running a Proxy of Ethereum Beacon URL

```sh
git clone https://github.com/ethstorage/beacon-api-wrapper.git

cd beacon-api-wrapper

go run cmd/main.go 
```

The Proxy works just like a normal Beacon API except the blobs retension period is much shorter, like 3 hours by default. This means when a `blob_sidecars` request (`/eth/v1/beacon/blob_sidecars/{block_id}`)  asking for blobs of more than 900 slot ago, it will return an empty list: `{"data":[]}`.

For details and more options please refer to this [repo](https://github.com/ethstorage/beacon-api-wrapper#beacon-api-wrapper).

### Running an OP Stack rollup

Following the [OP documentation](https://docs.optimism.io/builders/node-operators/tutorials/testnet) to run an op-node from source, and use the following options to make sure that:
- The BatchInbox is deployed on L1 and used as batch receiver. 

## Test Cases

### Test Case 1: Load an expired blob from EthStorage

- **Objective**: 
    - Verify that the blobs expired by the Beacon Chain can be retrieved from EthStorage archive service.
    - Verify that the blobs retrieved from EthStorage archive service are the same as from Beacon Chain in terms of blob data and some key properties.
- **Steps**: 
	1. Upload a blob aimed to be archived by EthStorage. (You may need [eth-blob-uploader](#eth-blob-uploader) for this.)
    2. Find the slot number and `Versioned Hash` of the blob from [the block explorer](https://sepolia.etherscan.io/) for later use.
    3. Query blobs by `/eth/v1/beacon/blob_sidecars/{block_id}` from the Beacon Chain URL, replacing `{block_id}` with the slot in step 2. For example,
    ```sh
    curl -X 'GET'   'http://88.99.30.186:3500/eth/v1/beacon/blob_sidecars/6103987'   -H 'accept: application/json' 
    ```
	4. Wait for the blob expire and the above query to return `{"data":[]}`.
    5. Query blobs by `/eth/v1/beacon/blob_sidecars/{block_id}` from the EthStorage archive service, replacing `{block_id}` with the slot, just like in step 3. For example,
    ```sh
    curl -X 'GET'   'http://65.108.236.27:9645/eth/v1/beacon/blob_sidecars/6103987'   -H 'accept: application/json' 
    ```
- **Expected Result**: 
    - The result of step 3 should contain the blob specifed by step 2.
    - The result of step 5 should contain the blob specifed by step 2.
    - The following properties have the same value between step 3 and 5:
        - Blob data
        - KZG Commitment
        - KZG Proof


### Test Case 2: Sync an OP node using blobs from EthStorage 
- **Objective**: 
- **Steps**:
1. Following this [tutorial](https://github.com/ethstorage/pm/blob/main/L2/testnet_new_node.md) to set up an op-node, and try to sycn data from the Beacon Chain.

2. Change configuation of the OP node, making sure that:
    - The L1 Beacon URL points to the Proxy, and 
    - The archive service points to the es-node RPC. 

For example, 
```sh
--l1.beacon http://65.108.236.27:3600 --l1.beacon-archiver http://65.108.236.27:9645
```
- **Expected Result**: 



## Tools

### eth-blob-uploader

`eth-blob-uploader` is a handy tool to upload EIP-4844 blobs to Ethereum.

The following command demostrates how to use `eth-blob-uploader` to upload a local file `hello.txt` to EthStorage testnet as a blob by calling the function `putBlob(bytes32 key,uint256 blobIdx,uint256 length)` of the storage contract deployed on Sepolia.

```sh
eth-blob-uploader -r http://88.99.30.186:8545 \
-p <private-key> \
-f hello.txt \
-t 0x804C520d3c084C805E37A35E90057Ac32831F96f \
-v 1500000000000000 \
-d 0x4581a9201c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000
```
Record the transaction hash so that you can find the detail of the transaction on [the block explorer](https://sepolia.etherscan.io/) such as the `Versioned Hash` of the blob, and which slot the blob is belong to.

Refer to https://github.com/ethstorage/eth-blob-uploader for more details.

## Conclusion
This document serves as the foundation for the testing framework and process of the blob archiver service. 
