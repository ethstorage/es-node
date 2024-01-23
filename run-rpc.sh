#!/bin/sh

# usage:
# ./run_rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc https://tame-wild-liquid.ethereum-goerli.quiknode.pro/4ae31eb78cb83cafc31140a8acc0841ea197a668 \
  --storage.l1contract 0x9e186c49b487C03e0c529b67BD9Bc9e1e2E713Fc"

# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network devnet \
  --storage.files $storage_file_0 \
  --l1.beacon https://tame-wild-liquid.ethereum-goerli.quiknode.pro/4ae31eb78cb83cafc31140a8acc0841ea197a668 \
  --l1.beacon-based-time 1705546368 \
  --l1.beacon-based-slot 7419864 \
  --p2p.max.request.size 4194304 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QDqL8nUyes92JnNMpXPSeDUlF9rKt1VXiLwdSNg95OdfDK6g0wxt3fpjPfqeiZoblXhFIZQlyyjkbLWL07i_XE-GAY0we7KPimV0aHN0b3JhZ2XbAYDY15TG8wDz9gpYIv1W9liQd8stQJylLsGAgmlkgnY0gmlwhEFtMpGJc2VjcDI1NmsxoQNIbl6CN0q_OiHTc2qON3rAtJwpJh7TByr4tVKp7zHgW4N0Y3CCJAaDdWRwgnZh \
 "
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
