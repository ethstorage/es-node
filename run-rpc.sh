#!/bin/sh

# usage:
# ./run_rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://88.99.30.186:8545 \
  --storage.l1contract 0x804C520d3c084C805E37A35E90057Ac32831F96f"

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network devnet \
  --storage.files $storage_file_0 \
  --l1.beacon http://88.99.30.186:3500 \
  --l1.beacon-based-time 1706684472 \
  --l1.beacon-based-slot 4245906 \
  --p2p.max.request.size 4194304 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QFpDtIlnf02Bli8jnZEkVAFyWkOOtaUZL7yKp3ySKmhGNiqRSe4AuUcFip3F4o_YLh30HJUg2UlcmIxx5W-fsK2GAY1eoPcdimV0aHN0b3JhZ2XbAYDY15SATFINPAhMgF43o16QBXrDKDH5b8GAgmlkgnY0gmlwhEFtMpGJc2VjcDI1NmsxoQL0mXwUXANkLHIAjN23dPfnOOhu-jhFUN13jcjHWeIP04N0Y3CCJAaDdWRwgnZh \
"
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
