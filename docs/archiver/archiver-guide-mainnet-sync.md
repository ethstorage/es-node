# Running a QuarkChain L2 Node to Derive Blocks from EthStorage Mainnet Archive Service

## Table of Contents
1. **[Introduction](#introduction)**
2. **[Running a Mocked Beacon API](#running-a-mocked-beacon-api)**
3. **[Running L2 Node](#running-l2-node)**
4. **[Verifying the Derivation Process](#verifying-the-derivation-process)**
   - 4.1 [Observing the Logs](#observing-the-logs)
   - 4.2 [Comparing Blocks](#comparing-blocks)
5. **[Conclusion](#conclusion)**

## Introduction

QuarkChain L2, built on OP Stack, publishes batches as blobs to its [BatchInbox contract](https://etherscan.io/address/0xf62e8574B92dc8764c5Ad957b5B0311595f5A3f9) on Ethereum mainnet. EthStorage stores those blobs as a long-term data-availability layer.

This guide shows you how to configure a QuarkChain L2 Mainnet node with a mocked [Beacon API](https://ethereum.github.io/beacon-APIs/#/Beacon/getBlobs) to verify that the new node can derive blocks from blobs fetched via EthStorage.

## Running a Mocked Beacon API

The following commands start a service to mock Beacon API with a shorter blob retention period:

```bash
# Ethereum Mainnet L1 Beacon URL provided by a Beacon node
export L1_BEACON_URL=<your_beacon_url>

git clone https://github.com/ethstorage/beacon-api-wrapper.git
cd beacon-api-wrapper
go run cmd/main.go -b $L1_BEACON_URL -p 3600 -r 3
```

If a blob older than 3 epochs (~20 minutes) is requested through `http://localhost:3600/eth/v1/beacon/blobs/{slot}`, the proxy will return an empty list. Other than that, it serves all requests just like a normal Beacon API does.

## Running L2 Node

Refer to [this tutorial](https://github.com/QuarkChain/pm/blob/main/L2/mainnet_new_node.md) to initialize and start op-geth (full sync, archive) and op-node. 

The differences to launch the op-node are highlighted below.

```bash
# The mocked Ethereum Mainnet L1 Beacon URL with short blob retention period
export L1_BEACON_URL_MOCKED=http://localhost:3600

# EthStorage API provided by an es-node with the archive service enabled
export ES_ARCHIVE_API=https://archive.mainnet.ethstorage.io:9645

  ...
  --syncmode=consensus-layer \
  --l1=$L1_RPC_URL \
  --l1.beacon=$L1_BEACON_URL_MOCKED \
  --l1.beacon-archiver=$ES_ARCHIVE_API \
  ...
```

**Note:**
- Consensus-layer sync is used so that op-node reads transaction data from L1 and derives blocks, then inserts them into the execution client. 
- Although P2P is enabled (in the omitted flags), consensus-layer sync does not rely on P2P networking to download state or block data from other L2 nodes.
- The L1 Beacon client is set to the mocked one started earlier, which has a shorter blob retention period.
- The beacon archiver is configured to point to the es-node Mainnet archive service as a fallback endpoint used when the requested blob is expired.

## Verifying the Derivation Process

### Observing the Logs
Once both op-geth and op-node are running, monitor their logs. 

The snippet below specifically shows a blob from slot 13076142 (L1 block 23848173) expiring on the short-retention Beacon endpoint, being fetched from EthStorage instead, and driving L2 block 7931.

**Mocked Beacon API logs:**
```log
2025/11/26 05:05:58 Received request for /eth/v1/beacon/blobs/13076142
2025/11/26 05:05:58 Block 13076142 is not in the retention window
```
A request for a blob older than 3 epochs is made by the op-node, and the proxy returns 404 as expected. Note that the new blobs Beacon API (`/eth/v1/beacon/blobs/...`) is used here.

**es-node logs (if available):**
```log
INFO [11-26|04:05:58.924] Blob archiver API request                from=65.108.236.27   url="/eth/v1/beacon/blob_sidecars/13076142?indices=8"
INFO [11-26|04:05:59.615] BeaconID to execution block number       beaconID=13076142 elBlock=23,848,173
INFO [11-26|04:05:59.788] Parsing event                            blobHash=01af4b..17e752 event="0 of 1"
INFO [11-26|04:05:59.788] Blobhash matched                         blobhash=01af4b..17e752 index=8 kvIndex=73
INFO [11-26|04:05:59.877] Sidecar built                            index=8 sidecar="{\n  \"index\": \"8\",\n  \"kzg_commitment\": \"0x861422d6f73dd7d5d4f9689a70e7619fdcb066b921fd25156c16d80af5d6f955a588076caf3b542fb651a43bc673e578\",\n  \"kzg_proof\": \"0x8c8593f07bde81031d48771f525ba18c84b63b8aaad3557e3d389887dd44b0b68cbf08804e2bed9de250afe0acbc033c\",\n  \"blob\": \"0x21000000b6005d7c347dd9872d68fb1e16b963f512af00000000009e78dada2927c97821f4c9c9f5dc51bf77f1dd78fce9faa72f1be7ea2be97a3c58f0b5e524173d2ba7a58bf8e6aaecce5f7159ef43cab5d233e7b4041c59a8861ca00c06082f118b1a062a5a87cd6aaa20066a1b28e0c8c4a1d044083510..."
INFO [11-26|04:05:59.884] Query blob sidecars done                 blobs=1
```
The op-node falls back to EthStorage’s archive endpoint for the same blob in slot 13076142. The old blob sidecar route (`/eth/v1/beacon/blob_sidecars/...`) is still used in this case, and the EthStorage archive service supports both the legacy and the new blob APIs, so the blob fetching succeeds.

**op-node logs:**
```log
t=2025-11-26T05:06:18+0100 lvl=info msg="Inserted new L2 unsafe block" hash=0x0efedc61d9bed66c521aa4dc41c91bf0554dafedd77d797b2367ad6bf6dc7d91 number=7931 build_time=2.123ms insert_time=3.967ms total_time=6.090ms mgas=0.079809 mgasps=13.103342518867999
t=2025-11-26T05:06:18+0100 lvl=info msg="generated attributes in payload queue" txs=1 timestamp=1763731691
t=2025-11-26T05:06:18+0100 lvl=info msg="Record safe head" l2=0x0efedc61d9bed66c521aa4dc41c91bf0554dafedd77d797b2367ad6bf6dc7d91:7931 l1=0x906cb9b7e101c5a926935ac55fc756c2a4a9bc0f40af832163b10c2c6022f1ef:23848173
```
From the retrieved blob, the op-node converts batches into payload attributes, calling the op-geth to work on the payload to convert it into block. 

**op-geth logs:**
```log
INFO [11-26|05:06:18.702] Starting work on payload                 id=0x03491eaadff9c746
INFO [11-26|05:06:18.705] Imported new potential chain segment     number=7931 hash=0efedc..dc7d91 blocks=1 txs=1 mgas=0.080 elapsed=1.475ms     mgasps=54.081  age=4d14h38m snapdiffs=532.61KiB  triedirty=0.00B
INFO [11-26|05:06:18.706] Chain head was updated                   number=7931 hash=0efedc..dc7d91 root=e797a9..0638aa elapsed=1.162611ms  age=4d14h38m
```
The op-geth works on payload and successfully imports the derived L2 block.

### Comparing Blocks
You can also verify the correctness of the derived rollup blocks by comparing the results of the following commands:

```bash
cast block 7931 -f hash -r http://127.0.0.1:8545
cast block 7931 -f hash -r https://rpc.mainnet.l2.quarkchain.io:8545
```

## Conclusion

By following these instructions, you'll permissionlessly launch a QuarkChain L2 rollup node that retrieves blobs from EthStorage, which have been pruned by the Ethereum Mainnet Beacon Chain. Additionally, you can verify that the op-node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution.
