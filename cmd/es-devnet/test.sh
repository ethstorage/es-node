#!/bin/bash

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as environment variable"
  exit 1
fi

if [ -z "$ES_NODE_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_PRIVATE_KEY' as environment variable"
  exit 1
fi


data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

# init shard 0
es_node_init="init --shard_index 0"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.108.236.27:8545 \
  --storage.l1contract 0x882BC290fc22C330592819977c48968a62AE25f4 \
  --storage.miner $ES_NODE_STORAGE_MINER"

$executable $es_node_init $common_flags
echo "initialized ${storage_file_0}"
