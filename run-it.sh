#!/bin/bash

data_dir="./es-data-it"
storage_file_0="$data_dir/shard-0.dat"
storage_file_1="$data_dir/shard-1.dat"
zkey_file="./build/bin/snark_lib/zkey/blob_poseidon2.zkey"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}

./init.sh \
  --l1.rpc http://127.0.0.1:32003 \
  --shard_index 0 \
  --shard_index 1 \
  --datadir $data_dir \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS


exec ./build/bin/es-node \
  --chain_id 3339 \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.files $storage_file_1 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --miner.enabled \
  --miner.zkey $zkey_file \
  --l1.rpc http://127.0.0.1:32003 \
  --l1.beacon http://127.0.0.1:33001 \
  --state.upload.url http://127.0.0.1:9096 \
  --rpc.port 9596 \
  --p2p.listen.udp 30396 \
  --p2p.listen.tcp 9296 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --p2p.bootnodes enr:-Lu4QGrri5Jy5gVEw_wpZn6TbIk8SjJgDrj-E4_RE4JISe2PYYjRjkJPtDHdrLZTuMOK-bC_b2ek08FHfzue_5J9A4WGAZl5vlySimV0aHN0b3JhZ2Xegg0LgNnYlMyj-ancoZauPJjmScNd2U-WEO0rwgGAgmlkgnY0gmlwhEFtWqCJc2VjcDI1NmsxoQJ8KIsZjyfFPHZOR66JORtqr5ax0QU6QmvT6QE0QllVZIN0Y3CCJE-DdWRwgna7
