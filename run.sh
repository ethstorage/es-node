#!/bin/sh

# to be compatible with docker
cd ../es-node

# download blob_poseidon.zkey if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found. Start downloading..."
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Po '(confirm=[a-zA-Z0-9\-_]+)'`&id=${file_id}" -o ${zkey_file}
  echo "downloaded ${zkey_file}"
fi

private_key="95eb6ffd2ae0b115db4d1f0d58388216f9d026896696a5211d77b5f14eb5badf"
executable="./cmd/es-node/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.108.236.27:8545 \
  --storage.l1contract 0xF83c395c1e0e261578D6732ac277404eeb2f99eA \
  --storage.miner 0xdF8466f277964Bb7a0FFD819403302C34DCD530A"

# init shard 0
es_node_init="init --shard_index 0"

# start node #TODO remove --network
es_node_start=" --network devnet \
  --miner.enabled \
  --miner.zkey ${zkey_file} \
  --miner.priority-gas-price 2000000000 \
  --miner.gas-price 3000000000 \
  --storage.files $storage_file_0 \
  --signer.private-key $private_key \
  --l1.beacon http://65.108.236.27:5052 \
  --l1.beacon-based-time 1693820652 \
  --l1.beacon-based-slot 136521 \
  --p2p.listen.udp 30305  \
  --p2p.bootnodes enr:-LC4QCABjeNlNMLy2Ai7Z9b0lbZAlno64zk5kEHM7sXLFGhwaalCLCcZ7BlrpDdWQRZlcROYXo8fO2WqBIUjsBGPYB-GAYrL92ALimV0aHN0b3JhZ2XbAYDY15T4PDlcHg4mFXjWcyrCd0BO6y-Z6sGAgmlkgnY0iXNlY3AyNTZrMaEDba07XjDdaXyvIQ0DmN-cvhOY5yAZRu4frQNgtKhI9iKDdGNwgiQGg3VkcIJ2YQ id=e770b1ddfe8e9238 \
  "
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags
   echo "initialized $storage_file_0"
fi

# start es-node
$executable $es_node_start $common_flags

exec "$@"
