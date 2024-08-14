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

./init-l2.sh \
  --datadir $data_dir \
  --encoding_type=0
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS

./run-l2.sh \
  --network integration \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --rpc.port 9595 \
  --p2p.listen.udp 30395 \
  --p2p.listen.tcp 9295 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --rpc.addr 0.0.0.0 \
  --miner.enabled=false


