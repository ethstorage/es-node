#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ES_NODE_CONTRACT_ADDRESS=<your_contract_address> ./run-l2-it.sh
# for one zk proof per sample (if the storage contract supports):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ES_NODE_CONTRACT_ADDRESS=<your_contract_address> ./run-l2-it.sh --miner.zk-prover-mode 1 --l1.rpc <el_rpc> --l1.beacon <cl_rpc>

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

# ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples.
zkp_mode=
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

if [ -n "$zkp_mode" ] && [ "$zkp_mode" != 1 ] && [ "$zkp_mode" != 2 ]; then
  echo "Error: zk prover mode can only be 1 or 2."
  exit 1  
fi 

if [ -n "$zkp_mode" ]; then
  echo "The zk prover mode has been overridden to $zkp_mode"
fi 

# download zkey if not yet
zkey_name="blob_poseidon2.zkey"
zkey_size=560301223
zkey_url="https://es-node-zkey.s3.us-west-1.amazonaws.com/blob_poseidon2_testnet1.zkey"
if [ "$zkp_mode" = 1 ]; then
  zkey_name="blob_poseidon.zkey"
  zkey_size=280151245
  zkey_url="https://drive.usercontent.google.com/download?id=1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-&export=download&confirm=t&uuid=16ddcd58-2498-4d65-8931-934df3d0065c"
fi
zkey_file="./build/bin/snark_lib/$zkey_name"
if [ ! -e  ${zkey_file} ] || [ $(wc -c <  ${zkey_file}) -ne ${zkey_size} ]; then
  echo "Start downloading ${zkey_file}..." 
  curl $zkey_url -o ${zkey_file}
  if [ ! -e  ${zkey_file} ]; then
    echo "Error: The zkey file was not downloaded. Please try again."
    exit 1
  fi
  if [ $(wc -c <  ${zkey_file}) -ne ${zkey_size} ]; then
    echo "Error: The zkey file was not downloaded correctly. You can check the file content for more information."
    exit 1
  fi
fi


# ZK prover implementation, 1: snarkjs, 2: go-rapidsnark.
zkp_impl=
i=1
while [ $i -le $# ]; do
    if [ "${!i}" = "--miner.zk-prover-impl" ]; then
        j=$((i+1))
        zkp_impl="${!j}"
        break
    else
        if echo "${!i}" | grep -qE -- "--miner\.zk-prover-impl=([0-9]+)"; then
            zkp_impl=$(echo "${!i}" | sed -E 's/.*=([0-9]+)/\1/')
            break
        fi
    fi
    i=$((i+1))
done


if [ -n "$zkp_impl" ] && [ "$zkp_impl" != 1 ] && [ "$zkp_impl" != 2 ]; then
  echo "miner.zk-prover-impl can only be 1 or 2"
  exit 1
fi 

if [ -n "$zkp_impl" ]; then
  echo "The zk prover implementation has been overridden to $zkp_impl"
fi 

if [ "$zkp_impl" = 1 ]; then

  if ! [ -x "$(command -v node)" ]; then
    echo 'Error: Node.js is not installed.'
    exit 1
  fi

  # check node js version
  node_version=$(node -v)
  major_version=$(echo $node_version | cut -d'v' -f2 | cut -d'.' -f1)

  if [ "$major_version" -lt 16 ]; then
      echo "Error: Node.js version is too old."
      exit 1
  fi

  # install snarkjs if not
  if ! [ "$(command -v snarkjs)" ]; then
      echo "snarkjs not found, start installing..."
      snarkjs_install=$(npm install -g snarkjs 2>&1)
      if [ $? -eq 0 ]; then
        echo "snarkjs installed successfully."
      else
        echo "Error: snarkjs install failed with the following error:"
        echo "$snarkjs_install"
        exit 1
      fi
  fi

fi

executable="./build/bin/es-node"
data_dir="./es-data-it"
storage_file_0="$data_dir/shard-0.dat"

if test -d  ${data_dir} ; then
  rm -r ${data_dir}
fi
mkdir ${data_dir}

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.109.20.29:8545 \
  --storage.l1contract $ES_NODE_CONTRACT_ADDRESS \
  --storage.miner $ES_NODE_STORAGE_MINER \
  "

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network integration \
  --miner.enabled \
  --miner.zkey $zkey_name \
  --storage.files $storage_file_0 \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --da.url http://65.109.20.29:8888 \
  --randao.url http://88.99.30.186:8545 \
  --state.upload.url http://127.0.0.1:9096 \
  --l1.block_time 2 \
  --download.thread 32 \
  --rpc.port 9596 \
  --p2p.listen.udp 30396 \
  --p2p.listen.tcp 9296 \
  --p2p.sync.concurrency 32 \
  --p2p.priv.path $data_dir/esnode_p2p_priv.txt \
  --p2p.peerstore.path $data_dir/esnode_peerstore_db \
  --p2p.discovery.path $data_dir/esnode_discovery_db \
  --p2p.bootnodes enr:-Li4QBp6QW2ji7JF-3yijZrQ54PqPZ-Io_xEtMUslxxcmGS5TAXiiU6hypBZbB_atxh2Pc72-MgonzU5_R-_qd_PBXyGAZDucmwzimV0aHN0b3JhZ2XbAYDY15SXhtonBXvE13WNGfkk7Nj9Y4_Qr8GAgmlkgnY0gmlwhFhjHrqJc2VjcDI1NmsxoQJ8KIsZjyfFPHZOR66JORtqr5ax0QU6QmvT6QE0QllVZIN0Y3CCJE-DdWRwgna7 \
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
