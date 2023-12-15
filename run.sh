#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_STORAGE_MINER} -ne 42 ] || case $ES_NODE_STORAGE_MINER in 0x*) false;; *) true;; esac; then
  echo "Error: ES_NODE_STORAGE_MINER should be prefixed with '0x' and have a total length of 42"
  exit 1
fi

if [ -z "$ES_NODE_SIGNER_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_SIGNER_PRIVATE_KEY' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_SIGNER_PRIVATE_KEY} -ne 64 ]; then
  echo "Error: ES_NODE_SIGNER_PRIVATE_KEY should have a length of 64"
  exit 1
fi

# download blob_poseidon.zkey if not yet
zkey_file="./build/bin/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
fi

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.109.115.36:8545 \
  --storage.l1contract 0xb4B46bdAA835F8E4b4d8e208B6559cD267851051 \
  --storage.miner $ES_NODE_STORAGE_MINER \
  $@"

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
# TODO remove --miner.priority-gas-price and --miner.gas-price when gas price query is available
es_node_start=" --network devnet \
  --miner.enabled \
  --storage.files $storage_file_0 \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --l1.beacon http://65.109.115.36:5052 \
  --l1.beacon-based-time 1701262812 \
  --l1.beacon-based-slot 1 \
  --p2p.listen.udp 30305 \
  --download.thread 32 \
  --p2p.max.request.size 4194304 \
  --p2p.max.concurrency 32 \
  --p2p.bootnodes enr:-Li4QPFCNc7mLPqxoVrk1eKB0qa5hb8H75IBwhvdSGGdamx1egKibkKO1v1rtLt7r3pJvoVxv95ITlpSphYCAsunU6qGAYwkwuOpimV0aHN0b3JhZ2XbAYDY15S0tGvaqDX45LTY4gi2VZzSZ4UQUcGAgmlkgnY0gmlwhEFtcySJc2VjcDI1NmsxoQM9rkUZ7qWoJQT2UVrPzDRzmLqDrxCSR4zC4db-lgz1bYN0Y3CCJAaDdWRwgnZh \
"
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  if $executable $es_node_init $common_flags ; then
    echo "Initialized ${storage_file_0} successfully"
  else
    echo "Error: failed to initialize ${storage_file_0}"
    exit 1
  fi
fi

# start es-node
exec $executable $es_node_start $common_flags
