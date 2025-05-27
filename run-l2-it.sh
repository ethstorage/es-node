#!/bin/bash

data_dir="./es-data-it"
storage_file_0="$data_dir/shard-0.dat"
storage_file_1="$data_dir/shard-1.dat"
zkey_file="./build/bin/snark_lib/zkey/blob_poseidon2.zkey"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}

./init-l2.sh \
  --shard_index 0 \
  --shard_index 1 \
  --datadir $data_dir \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS


exec ./build/bin/es-node \
  --network integration \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.files $storage_file_1 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --miner.enabled \
  --miner.zkey $zkey_file \
  --l1.block_time 2 \
  --l1.rpc http://5.9.87.214:8545 \
  --da.url http://5.9.87.214:8888 \
  --randao.url http://65.108.230.142:8545 \
  --state.upload.url http://127.0.0.1:9096 \
  --rpc.port 9596 \
  --p2p.listen.udp 30396 \
  --p2p.listen.tcp 9296 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --p2p.bootnodes enr:-Li4QBp6QW2ji7JF-3yijZrQ54PqPZ-Io_xEtMUslxxcmGS5TAXiiU6hypBZbB_atxh2Pc72-MgonzU5_R-_qd_PBXyGAZDucmwzimV0aHN0b3JhZ2XbAYDY15SXhtonBXvE13WNGfkk7Nj9Y4_Qr8GAgmlkgnY0gmlwhFhjHrqJc2VjcDI1NmsxoQJ8KIsZjyfFPHZOR66JORtqr5ax0QU6QmvT6QE0QllVZIN0Y3CCJE-DdWRwgna7
