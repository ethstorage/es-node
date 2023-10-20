#!/bin/bash

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as environment variable"
  exit 1
fi

if [ -z "$ES_NODE_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_PRIVATE_KEY' as environment variable"
  exit 1
fi



executable="./es-devnet"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

# remove old file
if [ $storage_file_0 ]; then
  rm -r $data_dir
  echo "remove ${storage_file_0}"
fi


# init shard 0
init_shard=" --datadir $data_dir \
  --l1.rpc http://65.108.236.27:8545 \
  --storage.l1contract 0x882BC290fc22C330592819977c48968a62AE25f4 \
  --storage.miner $ES_NODE_STORAGE_MINER \
  --shardLength 1
  "

$executable $init_shard
