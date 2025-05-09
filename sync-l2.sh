#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./sync-l2.sh --kv_index <index>

./sync.sh \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --l1.rpc http://5.9.87.214:8545 \
  --es_rpc http://65.109.115.36:9596 \
$@
