#!/bin/sh

data_dir="./es-data-it-bootnode"
storage_file_0="$data_dir/shard-0.dat"
storage_file_1="$data_dir/shard-1.dat"
zkey_file="./build/bin/snark_lib/zkey/blob_poseidon2.zkey"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}
echo "8714eb2672bb7ab01089a1060150b30bc374a3b00e18926460f169256d126339" > "${data_dir}/esnode_p2p_priv.txt"

./init-l2.sh \
  --shard_index 0 \
  --shard_index 1 \
  --encoding_type=0 \
  --datadir $data_dir \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS


exec ./build/bin/es-node \
  --network integration \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.files $storage_file_1 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --miner.enabled=false \
  --miner.zkey $zkey_file \
  --l1.block_time 2 \
  --l1.rpc http://5.9.87.214:8545 \
  --da.url http://5.9.87.214:8888 \
  --randao.url http://65.108.230.142:8545 \
  --rpc.port 9595 \
  --p2p.listen.udp 30395 \
  --p2p.listen.tcp 9295 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --rpc.addr 0.0.0.0

