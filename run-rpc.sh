#!/bin/sh

# usage:
# ./run_rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://65.109.115.36:8545 \
  --storage.l1contract 0xb4B46bdAA835F8E4b4d8e208B6559cD267851051 \

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network devnet \
  --storage.files $storage_file_0 \
  --l1.beacon http://65.109.115.36:5052 \
  --l1.beacon-based-time 1701262812 \
  --l1.beacon-based-slot 1 \
  --p2p.listen.udp 30305 \
  --p2p.max.request.size 4194304 \
  --p2p.max.concurrency 32 \
  --p2p.bootnodes enr:-Li4QPFCNc7mLPqxoVrk1eKB0qa5hb8H75IBwhvdSGGdamx1egKibkKO1v1rtLt7r3pJvoVxv95ITlpSphYCAsunU6qGAYwkwuOpimV0aHN0b3JhZ2XbAYDY15S0tGvaqDX45LTY4gi2VZzSZ4UQUcGAgmlkgnY0gmlwhEFtcySJc2VjcDI1NmsxoQM9rkUZ7qWoJQT2UVrPzDRzmLqDrxCSR4zC4db-lgz1bYN0Y3CCJAaDdWRwgnZh \
 "
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
