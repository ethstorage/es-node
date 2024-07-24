#!/bin/sh

# usage:
# ./run-l2-rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data-bootnode"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.109.20.29:8545 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
"

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network integration \
  --storage.files $storage_file_0 \
  --da.url http://65.109.20.29:8888 \
  --l1.block_time 2 \
  --download.thread 32 \
  --rpc.addr 0.0.0.0 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
"
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
