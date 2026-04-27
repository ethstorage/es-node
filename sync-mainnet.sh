#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./sync-mainnet.sh --kv_index <index>

./sync.sh \
  --storage.l1contract 0xf0193d6E8fc186e77b6E63af4151db07524f6a7A \
  --l1.rpc <mainnet_rpc_url> \
  --es_rpc <es_rpc_url> \
$@
