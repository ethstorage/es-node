#!/bin/sh

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
# for one zk proof per sample (if the storage contract supports):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh --miner.zk-prover-mode 1

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

# install snarkjs if not
if ! [ "$(command -v snarkjs)" ]; then
    echo "snarkjs not found, start installing..."
    npm install -g snarkjs
fi

# ZK prover version, 1: one proof per sample, 2: one proof for multiple samples.
zkp_version=2
for arg in "$@"
do
  case "$arg" in
    --miner.zk-prover-mode)
      shift
      zkp_version="$1"
      break
      ;;
    --miner.zk-prover-mode=*)
      zkp_version="${arg#*=}"
      ;;
  esac
done
echo "zk prover version is $zkp_version"

# download zkey if not yet
zkey_name="blob_poseidon2.zkey"
file_id="1V3QkMpk5UC48Jc62nHXMgzeMb6KE8JRY"
if [ "$zkp_version" = 1 ]; then
  zkey_name="blob_poseidon.zkey"
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
fi
zkey_file="./build/bin/snarkjs/$zkey_name"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
  rm cookie
fi

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.109.115.36:8545 \
  --storage.l1contract 0xb4B46bdAA835F8E4b4d8e208B6559cD267851051 \
  --storage.miner $ES_NODE_STORAGE_MINER \
  "

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network devnet \
  --miner.enabled \
  --miner.zkey $zkey_name \
  --storage.files $storage_file_0 \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --l1.beacon http://65.109.115.36:5052 \
  --l1.beacon-based-time 1701262812 \
  --l1.beacon-based-slot 1 \
  --download.thread 32 \
  --p2p.max.request.size 4194304 \
  --p2p.sync.concurrency 32 \
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
exec $executable $common_flags $es_node_start
