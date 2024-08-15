#!/bin/sh

data_dir="./es-data-it-bootnode"
storage_file_0="$data_dir/shard-0.dat"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}
echo "8714eb2672bb7ab01089a1060150b30bc374a3b00e18926460f169256d126339" > "${data_dir}/esnode_p2p_priv.txt"

./init-l2.sh \
  --encoding_type=0 \
  --datadir $data_dir \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS


exec ./build/bin/es-node \
  --network integration \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --miner.enabled=false \
  --miner.zkey $zkey_file \
  --l1.block_time 2 \
  --l1.rpc http://65.109.20.29:8545 \
  --da.url http://65.109.20.29:8888 \
  --randao.url http://88.99.30.186:8545 \
  --rpc.port 9595 \
  --p2p.listen.udp 30395 \
  --p2p.listen.tcp 9295 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --rpc.addr 0.0.0.0

