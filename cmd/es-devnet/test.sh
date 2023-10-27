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

generate_data="false"
if [ "$REMOVE_FILES" == "true" ]; then
  generate_data="true"
  # remove old file
  if [ $storage_file_0 ]; then
    rm -r $data_dir
    echo "remove ${data_dir}"
  fi
fi

# create data and upload to contract
generate_test_data=" --datadir $data_dir \
  --l1.rpc http://65.108.236.27:8545 \
  --l1.chainId 7011893059 \
  --storage.l1contract 0xF9581b99C73852547B8d8d4A7d1C505048653f72 \
  --storage.privateKey $ES_NODE_PRIVATE_KEY \
  --storage.miner $ES_NODE_STORAGE_MINER \
  --generateData $generate_data \
  --shardLength 1
  "
$executable $generate_test_data
