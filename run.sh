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

# ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples.
zkp_mode=2 
i=1
while [ $i -le $# ]; do
    if [ "${!i}" = "--miner.zk-prover-mode" ]; then
        j=$((i+1))
        zkp_mode="${!j}"
        break
    else
        if echo "${!i}" | grep -qE -- "--miner\.zk-prover-mode=([0-9]+)"; then
            zkp_mode=$(echo "${!i}" | sed -E 's/.*=([0-9]+)/\1/')
            break
        fi
    fi
    i=$((i+1))
done

if [ "$zkp_mode" != 1 ] && [ "$zkp_mode" != 2 ]; then
  echo "zk prover mode can only be 1 or 2"
  exit 1  
fi

echo "zk prover mode is $zkp_mode"

# download zkey if not yet
zkey_name="blob_poseidon2.zkey"
zkey_url="https://drive.usercontent.google.com/download?id=1V3QkMpk5UC48Jc62nHXMgzeMb6KE8JRY&export=download&confirm=t&uuid=987dbfe5-0dea-4c5b-8e8e-e03c0c43e5d2&at=APZUnTU9wgxwgi0Fua7Ooy6JxbIN:1705315831568"
if [ "$zkp_mode" = 1 ]; then
  zkey_name="blob_poseidon.zkey"
  zkey_url="https://drive.usercontent.google.com/download?id=1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-&export=download&confirm=t&uuid=16ddcd58-2498-4d65-8931-934df3d0065c&at=APZUnTVkjg3SRn910o_UXqeYqOoy:1705311918635"
fi
zkey_file="./build/bin/snarkjs/$zkey_name"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..." 
  curl $zkey_url -o ${zkey_file} 
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
  $@"
  
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
