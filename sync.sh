#!/bin/bash

# Sync local data specified by `--kv_index` from RPC
# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./sync.sh --kv_index <index>

executable="./build/bin/es-node"
data_dir="./es-data"
file_flags=""

for file in ${data_dir}/shard-[0-9]*.dat; do 
    if [ -f "$file" ]; then 
        file_flags+=" --storage.files $file"
    fi
done
start_flags=" sync \
  --datadir $data_dir \
  $file_flags \
  --storage.l1contract 0x804C520d3c084C805E37A35E90057Ac32831F96f \
  --l1.rpc http://65.108.230.142:8545 \
  --es_rpc http://65.108.230.142:9545 \
  $@"

exec $executable $start_flags
