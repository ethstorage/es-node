#!/bin/sh

# usage:
# env ES_NODE_CONTRACT_ADDRESS=<your_contract_address>  ./run-l2-it-rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data-it-bootnode"
storage_file_0="$data_dir/shard-0.dat"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}
echo "8714eb2672bb7ab01089a1060150b30bc374a3b00e18926460f169256d126339" > "${data_dir}/esnode_p2p_priv.txt"

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
  --rpc.port 9595 \
  --p2p.listen.udp 30395 \
  --p2p.listen.tcp 9295 \
  --p2p.sync.concurrency 32 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
"
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
