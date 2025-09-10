#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./sync-mainnet.sh --kv_index <index>

./sync.sh \
  --storage.l1contract <> \
  --l1.rpc http://65.108.230.142:8550 \
  --es_rpc <> \
$@
