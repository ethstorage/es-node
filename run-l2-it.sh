#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ES_NODE_CONTRACT_ADDRESS=<your_contract_address> ./run-l2-it.sh
# for one zk proof per sample (if the storage contract supports):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ES_NODE_CONTRACT_ADDRESS=<your_contract_address> ./run-l2-it.sh --miner.zk-prover-mode 1 --l1.rpc <el_rpc> --l1.beacon <cl_rpc>


executable="./build/bin/es-node"
data_dir="./es-data-it"
storage_file_0="$data_dir/shard-0.dat"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}

./init-l2.sh \
  --datadir $data_dir \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS


./run-l2.sh \
  --network integration \
  --datadir $data_dir \
  --storage.files $storage_file_0 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --state.upload.url http://127.0.0.1:9096 \
  --rpc.port 9596 \
  --p2p.listen.udp 30396 \
  --p2p.listen.tcp 9296 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --p2p.bootnodes enr:-Li4QBp6QW2ji7JF-3yijZrQ54PqPZ-Io_xEtMUslxxcmGS5TAXiiU6hypBZbB_atxh2Pc72-MgonzU5_R-_qd_PBXyGAZDucmwzimV0aHN0b3JhZ2XbAYDY15SXhtonBXvE13WNGfkk7Nj9Y4_Qr8GAgmlkgnY0gmlwhFhjHrqJc2VjcDI1NmsxoQJ8KIsZjyfFPHZOR66JORtqr5ax0QU6QmvT6QE0QllVZIN0Y3CCJE-DdWRwgna7

