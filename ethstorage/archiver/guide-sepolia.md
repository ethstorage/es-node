Here's a revised version of your document with improved formatting and language:

# Running an OP Node to Derive Blocks from Sepolia through EthStorage

## Introduction

This document outlines the process for running an OP node from source code, using Sepolia as Layer 1 (L1) and EthStorage as the blob archiver service. The goal is to verify that the OP node correctly derives Layer 2 (L2) blocks from Sepolia using expired blobs (pruned by the Sepolia Beacon Chain) retrieved from the EthStorage node (es-node).

## Prerequisites

Before proceeding, ensure your environment meets the following requirements:

- **Sepolia L1 RPC** provided by an execution client running in archive mode
- **Sepolia L1 Beacon URL**
- **An es-node** running with the archive service enabled
- A [deployed BatchInbox contract](https://sepolia.etherscan.io/address/0x27504265a9bc4330e3fe82061a60cd8b6369b4dc) on Sepolia L1
- An OP Stack L2 that utilizes EIP-4844 blobs to submit batches to the BatchInbox

It is assumed that the above services and components are functioning properly.

## Starting OP Geth

### Step 1: Build OP-Geth

First, clone the `op-geth` repository and build the execution client:

```bash
git clone https://github.com/ethereum-optimism/op-geth.git
cd op-geth
git checkout v1.101408.0
make geth
```

### Step 2: Create a JWT Secret

Generate a JWT secret to secure communications between `op-geth` and `op-node`:

```bash
openssl rand -hex 32 > jwt.txt
```

### Step 3: Set Environment Variables

Set the data directory for `op-geth`:

```bash
export DATADIR=./datadir
```

### Step 4: Initialize OP-Geth

Download the genesis file and initialize the client:

```bash
curl -o genesis.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_genesis.json
./build/bin/geth init --state.scheme=hash --datadir=$DATADIR ./genesis.json
```

### Step 5: Start OP-Geth

Start the client with the following command:

```bash
./build/bin/geth \
  --datadir $DATADIR \
  --http \
  --http.corsdomain="*" \
  --http.vhosts="*" \
  --http.addr=0.0.0.0 \
  --http.port=9515 \
  --http.api=web3,debug,eth,txpool,net,engine \
  --ws \
  --ws.addr=0.0.0.0 \
  --ws.port=9516 \
  --ws.origins="*" \
  --ws.api=debug,eth,txpool,net,engine \
  --syncmode=full \
  --gcmode=archive \
  --nodiscover \
  --maxpeers=0 \
  --networkid=42069 \
  --port=30903 \
  --authrpc.vhosts="*" \
  --authrpc.addr=0.0.0.0 \
  --authrpc.port=9551 \
  --authrpc.jwtsecret=./jwt.txt \
  --rollup.disabletxpoolgossip=true
```

## Starting the OP Node

### Step 1: Get the Code

Clone the Optimism monorepo and check out the `long-term-da` branch if you haven't already:

```bash
git clone https://github.com/ethstorage/optimism.git
cd optimism
git checkout long-term-da
```

### Step 2: Build OP-Node

Build the `op-node` implementation of the Rollup Node:

```bash
make op-node
```

### Step 3: Copy the JWT Secret

Copy the JWT file created in the `op-geth` repository to the `op-node` folder:

```bash
cp ../op-geth/jwt.txt ./op-node
```

### Step 4: Download the Configuration File

Download the rollup configuration file for `op-node`:

```bash
curl -o rollup.json https://raw.githubusercontent.com/ethstorage/pm/refs/heads/main/L2/assets/testnet_rollup.json
```

### Step 5: Start OP-Node

To start the OP node, execute the following command:

```bash
./op-node/bin/op-node \
  --l2=http://localhost:9551 \
  --l2.jwt-secret=./op-node/jwt.txt \
  --sequencer.enabled=false \
  --p2p.disable \
  --verifier.l1-confs=4 \
  --rollup.config=./rollup.json \
  --rpc.addr=0.0.0.0 \
  --rpc.port=9595 \
  --rpc.enable-admin \
  --l1=http://88.99.30.186:8545 \
  --l1.beacon=http://88.99.30.186:3500 \
  --l1.rpckind=basic \
  --l1.beacon-archiver=http://65.108.236.27:9645
```

**Note:**

1. P2P is disabled to ensure that it only syncs data from L1.
2. The beacon archiver is configured to point to the es-node archive service.


## Verifing the derivition process

After the OP node started, you can observe the logs from the consoles, something like this:

**op-geth**
```log
INFO [11-28|03:32:04.066] Starting work on payload                 id=0x032b2419eae77a19
INFO [11-28|03:32:04.068] Imported new potential chain segment     number=451,055 hash=f8e51d..27bf83 blocks=1 txs=1 mgas=0.046 elapsed=1.147ms      mgasps=40.300   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:04.069] Chain head was updated                   number=451,055 hash=f8e51d..27bf83 root=125b39..cf03e7 elapsed="105.017µs"  age=4mo6d14h
INFO [11-28|03:32:06.118] Starting work on payload                 id=0x037ebfaf591014f7
INFO [11-28|03:32:06.126] Imported new potential chain segment     number=451,056 hash=eb6731..fc3b73 blocks=1 txs=1 mgas=0.060 elapsed=6.349ms      mgasps=9.483    age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.127] Chain head was updated                   number=451,056 hash=eb6731..fc3b73 root=d5d849..dfc990 elapsed="198.442µs"  age=4mo6d14h
INFO [11-28|03:32:06.128] Starting work on payload                 id=0x03893a89c9f2090a
INFO [11-28|03:32:06.130] Imported new potential chain segment     number=451,057 hash=ab6ee3..d04e23 blocks=1 txs=1 mgas=0.046 elapsed="789.932µs"  mgasps=58.520   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.130] Chain head was updated                   number=451,057 hash=ab6ee3..d04e23 root=99324e..4f7189 elapsed="68.248µs"   age=4mo6d14h
INFO [11-28|03:32:06.131] Starting work on payload                 id=0x033cbd94efb3d2d5
INFO [11-28|03:32:06.133] Imported new potential chain segment     number=451,058 hash=ed8d7f..6222a9 blocks=1 txs=1 mgas=0.046 elapsed="831.63µs"   mgasps=55.586   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.133] Chain head was updated                   number=451,058 hash=ed8d7f..6222a9 root=a0fbe8..a7773c elapsed="73.638µs"   age=4mo6d14h
INFO [11-28|03:32:06.134] Starting work on payload                 id=0x030ce6c112473600
INFO [11-28|03:32:06.136] Imported new potential chain segment     number=451,059 hash=8d5805..b47cb9 blocks=1 txs=1 mgas=0.046 elapsed="841.359µs"  mgasps=54.943   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.137] Chain head was updated                   number=451,059 hash=8d5805..b47cb9 root=f4bd7b..90140f elapsed="67.837µs"   age=4mo6d14h
INFO [11-28|03:32:06.138] Starting work on payload                 id=0x03728e9846578a73
INFO [11-28|03:32:06.139] Imported new potential chain segment     number=451,060 hash=d950bf..8940bc blocks=1 txs=1 mgas=0.046 elapsed="677.771µs"  mgasps=68.204   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.140] Chain head was updated                   number=451,060 hash=d950bf..8940bc root=5fbbaa..29d6bd elapsed="73.589µs"   age=4mo6d14h
INFO [11-28|03:32:06.141] Starting work on payload                 id=0x0320593f631e71a5
INFO [11-28|03:32:06.142] Imported new potential chain segment     number=451,061 hash=34a268..8e575a blocks=1 txs=1 mgas=0.046 elapsed="673.483µs"  mgasps=68.639   age=4mo6d14h snapdiffs=2.52MiB    triedirty=0.00B
INFO [11-28|03:32:06.143] Chain head was updated                   number=451,061 hash=34a268..8e575a root=a2ffe7..c39c96 elapsed="146.495µs"  age=4mo6d14h
```
**op-node**
```log
INFO [11-28|03:32:04.064] Generating next batch                    epoch=c6e371..583306:6367781 timestamp=1,721,827,390
INFO [11-28|03:32:04.065] generated attributes in payload queue    txs=1 timestamp=1,721,827,390
INFO [11-28|03:32:04.069] Inserted block                           hash=f8e51d..27bf83 number=451,055 state_root=125b39..cf03e7 timestamp=1,721,827,390 parent=2576aa..f6f9b5 prev_randao=0eb431..244662 fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=66d32f..d593ba:6374981
INFO [11-28|03:32:04.500] Advancing bq origin                      origin=1768f8..177a85:6374982 originBehind=false
INFO [11-28|03:32:04.501] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,392
INFO [11-28|03:32:06.117] generated attributes in payload queue    txs=1 timestamp=1,721,827,392
INFO [11-28|03:32:06.126] Inserted block                           hash=eb6731..fc3b73 number=451,056 state_root=d5d849..dfc990 timestamp=1,721,827,392 parent=f8e51d..27bf83 prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:06.127] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,394
INFO [11-28|03:32:06.128] generated attributes in payload queue    txs=1 timestamp=1,721,827,394
INFO [11-28|03:32:06.130] Inserted block                           hash=ab6ee3..d04e23 number=451,057 state_root=99324e..4f7189 timestamp=1,721,827,394 parent=eb6731..fc3b73 prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:06.130] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,396
INFO [11-28|03:32:06.131] generated attributes in payload queue    txs=1 timestamp=1,721,827,396
INFO [11-28|03:32:06.133] Inserted block                           hash=ed8d7f..6222a9 number=451,058 state_root=a0fbe8..a7773c timestamp=1,721,827,396 parent=ab6ee3..d04e23 prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:06.134] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,398
INFO [11-28|03:32:06.134] generated attributes in payload queue    txs=1 timestamp=1,721,827,398
INFO [11-28|03:32:06.136] Inserted block                           hash=8d5805..b47cb9 number=451,059 state_root=f4bd7b..90140f timestamp=1,721,827,398 parent=ed8d7f..6222a9 prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:06.137] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,400
INFO [11-28|03:32:06.137] generated attributes in payload queue    txs=1 timestamp=1,721,827,400
INFO [11-28|03:32:06.139] Inserted block                           hash=d950bf..8940bc number=451,060 state_root=5fbbaa..29d6bd timestamp=1,721,827,400 parent=8d5805..b47cb9 prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:06.140] Generating next batch                    epoch=e7b478..9fbe3c:6367782 timestamp=1,721,827,402
INFO [11-28|03:32:06.140] generated attributes in payload queue    txs=1 timestamp=1,721,827,402
INFO [11-28|03:32:06.142] Inserted block                           hash=34a268..8e575a number=451,061 state_root=a2ffe7..c39c96 timestamp=1,721,827,402 parent=d950bf..8940bc prev_randao=f99bdd..62ef4c fee_recipient=0x4200000000000000000000000000000000000011 txs=1 last_in_span=true  derived_from=1768f8..177a85:6374982
INFO [11-28|03:32:07.078] Advancing bq origin                      origin=7383e1..3b1940:6374983 originBehind=false
INFO [11-28|03:32:07.078] Generating next batch                    epoch=75c275..69e44e:6367783 timestamp=1,721,827,404

**es-node**
```log
t=2024-11-28T03:28:34+0000 lvl=info msg="Blob archiver API request"             url="/eth/v1/beacon/blob_sidecars/5516563?indices=3"
t=2024-11-28T03:28:35+0000 lvl=info msg="Query el block number and kzg"         took(s)=0.335
t=2024-11-28T03:28:35+0000 lvl=info msg="BeaconID to execution block number"    beaconID=5516563 elBlock=6,374,791
t=2024-11-28T03:28:35+0000 lvl=info msg="Parsing event"                         blobHash=0x01159877319d2555dde3710b8c10f08b14b839e2cd36dcc40e9e9c6e3eb08c24 event="0 of 1"
t=2024-11-28T03:28:35+0000 lvl=info msg="Blobhash matched"                      blobhash=0x01159877319d2555dde3710b8c10f08b14b839e2cd36dcc40e9e9c6e3eb08c24 index=3 kvIndex=2,006,657
t=2024-11-28T03:28:35+0000 lvl=info msg="Build sidecar"                         took(s)=0.063
t=2024-11-28T03:28:35+0000 lvl=info msg="Sidecar built"                         index=3 sidecar="{\n  \"index\": \"3\",\n  \"kzg_commitment\": \"0x929953756be0e6acc0648352d17344c9f6c19dc8ca7160af87929d21988e4beeecd3cda5a50bb2c25b8332887c34363f\",\n  \"kzg_proof\": \"0x92838c3e3e426a47a4395a39335027123b4181d4c138ab525306721021be97fa89a1a9b98ad35ab6b84adec98bd49a21\",\n  \"blob\": \"0x0500017a3300c30d5c79f6ca103d96c5d04db106b4fd000000017a1b78daecbb0154546d1b2ebc67186000c5a104146140e95091909ea1bbbb43404a90ee18403f15944e41a44b42ba155040404040a4919010a41bfe3558bcdff7adff3beb9c10fd6b9d73e65afb75edf7d9cebe9fbbaefb7a18ac0113cb80..."
t=2024-11-28T03:28:35+0000 lvl=info msg="Query blob sidecars done"              blobs=1
t=2024-11-28T03:28:35+0000 lvl=info msg="Blob archiver API request handled"     took(s)=0.447
t=2024-11-28T03:28:45+0000 lvl=info msg="Connected to peer"                     peer=16Uiu2HAmHi99uh2tmoPBZ9fmgPkgkssHwNQBsXyVuKtjyaJWDo9T Direction=Outbound addr=/ip4/172.17.0.2/tcp/9222
```

From the logs we can see the op-node keeps converting the batches into payload attributes, calling op-geth to work on the payload to convert it into block, then insert the block as the chain head. Additionally you may notice the age of derived blocks are about more than 4 month, so once for a while a blob archiver API request is handled by es-node, which means the blob is retrieved from EthStorage since it was pruned by the L1 Beacon client.

You can also verify the correctness of the derived rollup blocks by compare the results of the following queries:

```
cast block 451057 -f hash -r http://127.0.0.1:9515
cast block 451057 -f hash -r http://65.109.20.29:8545
```

## Conclusion

By following these instructions, you'll permissionlessly launch an OP Stack rollup node that retrieves blobs from EthStorage, which have been pruned by the Sepolia Beacon Chain. Additionally, you can verify that the OP node derives L2 blocks correctly from these blobs, demonstrating that EthStorage effectively serves as a decentralized long-term data availability solution. 