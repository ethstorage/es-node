#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
# for one zk proof per sample (if the storage contract supports):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh --miner.zk-prover-mode 1 --l1.rpc <el_rpc> --l1.beacon <cl_rpc>

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

# function to extract a value from command-line arguments
extract_value() {
    local arg_name="$1"
    local arg_value=""
    local i=1
    local skip=0

    while [ $i -le $# ]; do
        if [ "${!i}" = "$arg_name" ]; then
          if [ $skip = 0 ]; then
            skip=1
          else
            j=$((i+1))
            arg_value="${!j}"
            break
          fi
        elif echo "${!i}" | grep -qE -- "$arg_name=(.*)"; then
          arg_value=$(echo ${!i} | cut -d'=' -f2)
          break
        fi
        i=$((i+1))
    done

    echo "$arg_value"
}

# function to remove a specified flag from command-line arguments
remove_flag() {
    local flag="$1"
    local args=("$@")
    local new_args=()

    for arg in "${args[@]}"; do
        if [[ "$arg" == "$flag" ]]; then
            i=$((i+1))
            continue
        elif [[ "$arg" == "$flag"=* ]]; then
            continue
        else
            new_args+=("$arg")
        fi
    done

    echo "${new_args[@]}"
}

# ZK prover implementation, 1: snarkjs, 2: go-rapidsnark.
zkp_impl=$(extract_value "--miner.zk-prover-impl" "$@")

if [ -n "$zkp_impl" ] && [ "$zkp_impl" != 1 ] && [ "$zkp_impl" != 2 ]; then
    echo "Error: zk prover implementation can only be 1 or 2."
    exit 1
fi

if [ -n "$zkp_impl" ]; then
    echo "The zk prover implementation has been overridden to $zkp_impl"
  else
    zkp_impl=1
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

# ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples.
zkp_mode=$(extract_value "--miner.zk-prover-mode" "$@")

if [ -n "$zkp_mode" ] && [ "$zkp_mode" != 1 ] && [ "$zkp_mode" != 2 ]; then
    echo "Error: zk prover mode can only be 1 or 2."
    exit 1
fi

if [ -n "$zkp_mode" ]; then
    echo "The zk prover mode has been overridden to $zkp_mode"
  else
    zkp_mode=2
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

data_dir="./es-data"
init_flags="init"
shard_array=(0)
files_to_init=()
storage_files_flag=()
additional_args=$@
shards=$(extract_value "--shards" "$@")

if [ -n "$shards" ]; then
  IFS=',' read -ra shard_array <<< "$shards"
  additional_args=$(remove_flag "--shards" "$@")
fi

for shard in "${shard_array[@]}"; do
  storage_file="$data_dir/shard-$shard.dat"
  if [ ! -e $storage_file ]; then
    files_to_init+=("$storage_file")
    init_flags+=" --shard_index $shard"
  fi
  storage_files_flag+=("--storage.files $storage_file")
done


common_flags=" --datadir $data_dir \
  --l1.rpc http://88.99.30.186:8545 \
  --storage.l1contract 0x804C520d3c084C805E37A35E90057Ac32831F96f \
  --storage.miner $ES_NODE_STORAGE_MINER \
  "

executable="./build/bin/es-node"

# create data files (init)
if [ ${#files_to_init[@]} -gt 0 ]; then
  if $executable $init_flags $common_flags ; then
    echo "Initialized ${files_to_init[@]} successfully"
  else
    echo "Error: failed to initialize storage files: ${files_to_init[@]}"
    exit 1
  fi
fi

# TODO remove --network
start_flags=" --network devnet \
  --miner.enabled \
  --miner.zkey $zkey_name \
  ${storage_files_flag[@]} \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --l1.beacon http://88.99.30.186:3500 \
  --l1.beacon-based-time 1706684472 \
  --l1.beacon-based-slot 4245906 \
  --download.thread 32 \
  --state.upload.url http://metrics.ethstorage.io:8080 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QF3vBkkDQYNLHlVjW5NcEpXAsfNtE1lUVb_LgUQ_Ot2afS8jbDfnYQBDABJud_5Hd1hX_1cNeGVU6Tem06WDlfaGAY1e3vNvimV0aHN0b3JhZ2XbAYDY15SATFINPAhMgF43o16QBXrDKDH5b8GAgmlkgnY0gmlwhEFtP5qJc2VjcDI1NmsxoQK8XODtSv0IsrhBxZmTZBZEoLssb7bTX0YOVl6S0yLxuYN0Y3CCJAaDdWRwgnZh \
$additional_args"

# start es-node
exec $executable $common_flags $start_flags
