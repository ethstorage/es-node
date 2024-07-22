#!/bin/bash

# usage 1 (use snarkjs, will check Node.js version and snarkjs installation):
# env ES_NODE_STORAGE_MINER=<miner> ./init.sh
# usage 2 (use go-rapisnark):
# env ES_NODE_STORAGE_MINER=<miner> ./init.sh --miner.zk-prover-impl 2

# Note: currently only zk prover mode is supported

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_STORAGE_MINER} -ne 42 ] || case $ES_NODE_STORAGE_MINER in 0x*) false;; *) true;; esac; then
  echo "Error: ES_NODE_STORAGE_MINER should be prefixed with '0x' and have a total length of 42"
  exit 1
fi

executable="./build/bin/es-node"
echo "========== build info =================="
$executable --version
echo "========================================"

# ZK prover implementation, 1: snarkjs, 2: go-rapidsnark. Default: 1
zkp_impl=1
# ZK prover mode, 1: one proof per sample, 2: one proof for multiple samples. Default: 2
zkp_mode=2
#!/bin/bash

remaining_args=""

while [ $# -gt 0 ]; do
    if [[ $1 == --miner.zk-prover-impl ]]; then
        zkp_impl=$2
        shift 2
    elif [[ $1 == --miner.zk-prover-mode ]]; then
        zkp_mode=$2
        shift 2
    else
        remaining_args="$remaining_args $1"
        shift
    fi
done

if [ -n "$zkp_mode" ] && [ "$zkp_mode" != 1 ] && [ "$zkp_mode" != 2 ]; then
  echo "Error: zk prover mode can only be 1 or 2."
  exit 1  
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
zkey_file="./build/bin/snark_lib/zkey/$zkey_name"
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

echo "zkp_impl is set to $zkp_impl"
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

data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

es_node_init="$executable init --shard_index 0 \
  --datadir $data_dir \
  --l1.rpc http://88.99.30.186:8545 \
  --storage.l1contract 0x804C520d3c084C805E37A35E90057Ac32831F96f \
  --storage.miner $ES_NODE_STORAGE_MINER \
$remaining_args"


echo "$es_node_init"

# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  if $es_node_init ; then
    echo "Initialized ${storage_file_0} successfully"
  else
    echo "Error: failed to initialize ${storage_file_0}"
    exit 1
  fi
else 
  echo "Warning: storage file ${storage_file_0} already exists, skip initialization."
fi