#!/bin/sh

# usage:
# ./run.sh -m "0xyour_miner_address" -k "your_private_key"

while getopts "m:k:" opt; do
  case ${opt} in
    m) miner=${OPTARG} ;;
    k) private_key=${OPTARG} ;;
    \?) echo "Invalid option -$OPTARG" >&2
        exit 1 ;;
  esac
done

if [ -z "$miner" ]; then
  echo "Please provide miner using -m flag"
  exit 1
fi

if [ -z "$private_key" ]; then
  echo "Please provide private_key using -k flag"
  exit 1
fi

# download blob_poseidon.zkey if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  if ! command -v curl &> /dev/null; then
      echo "Installing curl"
      apt-get -y install curl
  fi
  echo "Start downloading zkey..."
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Po '(confirm=[a-zA-Z0-9\-_]+)'`&id=${file_id}" -o ${zkey_file}
  echo "downloaded ${zkey_file}"
fi

# to be compatible with docker
cd ../es-node

executable="./cmd/es-node/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.108.236.27:8545 \
  --storage.l1contract 0xC5af49F2aD56eC383a7948B16D9b7F48A9898aC9 \
  --storage.miner $miner"

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
# TODO remove --miner.priority-gas-price and --miner.gas-price when gas price query is available
es_node_start=" --network devnet \
  --miner.enabled \
  --miner.priority-gas-price 2000000000 \
  --miner.gas-price 3000000000 \
  --storage.files $storage_file_0 \
  --signer.private-key $private_key \
  --l1.beacon http://65.108.236.27:5052 \
  --l1.beacon-based-time 1693820652 \
  --l1.beacon-based-slot 136521 \
  --p2p.listen.udp 30305  \
  --p2p.bootnodes enr:-Li4QLQ6gZVvsu-4m8zmeBCheAWCpQ7PZu2TsyRKKTNAVooRRIa2F9jT0z-MS4WV0BD3mx00FJKWryPWC4PPFQhcvNaGAYrWFtG0imV0aHN0b3JhZ2XbAYDY15TFr0nyrVbsODp5SLFtm39IqYmKycGAgmlkgnY0gmlwhEFtMpGJc2VjcDI1NmsxoQO9fpE3o5lJUiCRGS_7--JCi_-rpzmoWbeBPWkRo4wlpYN0Y3CCJAaDdWRwgnZh \
  "
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags
  echo "initialized ${storage_file_0}"
fi

# start es-node
$executable $es_node_start $common_flags

exec "$@"
