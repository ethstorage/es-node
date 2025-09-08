#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./init.sh

executable="./build/bin/es-node"
if [ ! -f "$executable" ]; then
  make
fi
echo "========== build info =================="
$executable --version
echo "========================================"

# ZK prover implementation, 1: snarkjs, 2: go-rapidsnark.
zkp_impl=1
# ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples.
# Note: currently only zk prover mode 2 is supported
zkp_mode=2
data_dir="./es-data"

remaining_args=""
shards="--shard_index 0"
use_miner=1

while [ $# -gt 0 ]; do
    if [[ $1 == --miner.zk-prover-impl ]]; then
        zkp_impl=$2
        shift 2
    elif [[ $1 == --miner.zk-prover-mode ]]; then
        zkp_mode=$2
        shift 2
    elif [[ $1 == --datadir ]]; then
        data_dir=$2
        shift 2
    elif [[ $1 == --shard_index ]]; then
        shards=""
        remaining_args="$remaining_args $1"
        shift
    elif [[ $1 == --encoding_type=0 ]]; then # from init-rpc.sh
        use_miner=0
        remaining_args="$remaining_args $1"
        shift
    else
        remaining_args="$remaining_args $1"
        shift
    fi
done

# Require ES_NODE_STORAGE_MINER if not an RPC
if [[ "$use_miner" -eq 1 && -z "${ES_NODE_STORAGE_MINER:-}" ]]; then
  echo "Missing ES_NODE_STORAGE_MINER."
  exit 1
fi

if [ -n "$zkp_mode" ] && [ "$zkp_mode" != 1 ] && [ "$zkp_mode" != 2 ]; then
  echo "Error: zk prover mode can only be 1 or 2."
  exit 1
fi

if [ $use_miner = 1 ]; then
  zkey_path="./build/bin/snark_lib/zkey"
  mkdir -p $zkey_path
  # download zkey if not yet
  zkey_name="blob_poseidon2.zkey"
  zkey_size=560412712
  zkey_url="https://es-zkey.s3.us-west-2.amazonaws.com/blob_poseidon2_v1.zkey"
  if [ "$zkp_mode" = 1 ]; then
    zkey_name="blob_poseidon.zkey"
    zkey_size=280269776
    zkey_url="https://es-zkey.s3.us-west-2.amazonaws.com/blob_poseidon1_v1.zkey"
  fi
  zkey_file="$zkey_path/$zkey_name"
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
  else
    echo "√ ${zkey_file} already exists."
  fi

  if [ -n "$zkp_impl" ] && [ "$zkp_impl" != 1 ] && [ "$zkp_impl" != 2 ]; then
    echo "Error: miner.zk-prover-impl can only be 1 or 2"
    exit 1
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
        echo "Error: Node.js version is too old: $node_version; must be 16 and above."
        exit 1
    else
      echo "√ Node.js version is compatible."
    fi

    # install snarkjs if not
    if ! [ "$(command -v snarkjs)" ]; then
        echo "snarkjs not found, start installing..."
        snarkjs_install=$(npm install -g snarkjs 2>&1)
        if [ $? -eq 0 ]; then
          echo "√ snarkjs installed successfully."
        else
          echo "Error: snarkjs install failed with the following error:"
          echo "$snarkjs_install"
          exit 1
        fi
    else
        echo "√ snarkjs is already installed."
    fi

  fi

fi

es_node_init="$executable init $shards \
  --datadir $data_dir \
  --l1.rpc http://65.108.230.142:8545 \
  --storage.l1contract 0xAb3d380A268d088BA21Eb313c1C23F3BEC5cfe93 \
$remaining_args"

# es-node will skip init if data files already exist
if $es_node_init ; then
  echo "√ Initialized data files successfully."
else
  echo "Error: failed to initialize data files."
  exit 1
fi
