#!/bin/sh

# usage:
# ./run-l2-rpc.sh

executable="./build/bin/es-node"
data_dir="./es-data"
storage_file_0="$data_dir/shard-0.dat"

common_flags=" --datadir $data_dir \
  --l1.rpc http://142.132.154.16:8545 \
  --storage.l1contract 0x90a708C0dca081ca48a9851a8A326775155f87Fd \
"
# init shard 0
es_node_init="init --shard_index 0"

# start node 
# TODO remove --network
es_node_start=" --network devnet \
  --storage.files $storage_file_0 \
  --da.url http://142.132.154.16:8888 \
  --l1.block_time 2 \
  --download.thread 32 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QGUAA21O-0pgqnGoBLwvvminrlDjfxhqL6DvXhfOtvNdK871LELAT1Nn-NAa3hUi0Wmb-VIj1qi6fnbyA9yp5RGGAZALHvLnimV0aHN0b3JhZ2XbAYDY15SQpwjA3KCBykiphRqKMmd1FV-H_cGAgmlkgnY0gmlwhEFtMpGJc2VjcDI1NmsxoQJ8_OUONb_H7RMF6kXzZWDut2xriJ5JeKnH2cnb8en0e4N0Y3CCJAaDdWRwgnZh \
"
# create data file for shard 0 if not yet
if [ ! -e $storage_file_0 ]; then
  $executable $es_node_init $common_flags --encoding_type 0
  echo "initialized ${storage_file_0}"
fi

# start es-node
exec $executable $es_node_start $common_flags
