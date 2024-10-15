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
- A Proxy of Ethereum Beacon RPC to mock a short expiration time
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

### Running a Proxy of Ethereum Beacon URL

```
git clone https://github.com/ethstorage/beacon-api-wrapper.git

cd beacon-api-wrapper

go run cmd/main.go 
```

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
- **Objective**: Verify that blobs over 4096 epochs age that is expired by the Beacon Chain can be retrieved from EthStorage archive service.
- **Preconditions**: 
- **Inputs**: 
- **Expected Result**: 

### Test Case 2: 


## Conclusion
This document serves as the foundation for the software testing framework and process of the blob archiver service. It provides a clear structure for the testing activities to ensure quality outcomes.
