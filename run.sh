#!/bin/sh

miner="<miner>"
private_key="<private_key>"

if [ "$miner" = "<miner>" ]; then
  echo "Please replace <miner> with your own"
  exit 1
fi
if [ "$private_key" = "<private_key>" ]; then
  echo "Please replace <private_key> with your own"
  exit 1
fi

# download blob_poseidon.zkey if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found. Start downloading..."
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
  --storage.l1contract 0xF83c395c1e0e261578D6732ac277404eeb2f99eA \
  --storage.miner $miner"

# init shard 0
es_node_init="init --shard_index 0"

# start node #TODO remove --network
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
  --p2p.bootnodes enr:-Li4QOmTNMkvUcAUcSK4HOvEt4CAqWaSX5XWV2wdgQNIhxz6c43jGbqGTh3sZxCsTjvrtqEfRQ4nzY5dlJ22Fr-4GBuGAYrL92AVimV0aHN0b3JhZ2XbAYDY15T4PDlcHg4mFXjWcyrCd0BO6y-Z6sGAgmlkgnY0gmlwhEFs7BuJc2VjcDI1NmsxoQNtrTteMN1pfK8hDQOY35y-E5jnIBlG7h-tA2C0qEj2IoN0Y3CCJAaDdWRwgnZh \
  "
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags
  echo "initialized ${storage_file_0}"
fi

# start es-node
$executable $es_node_start $common_flags

exec "$@"
