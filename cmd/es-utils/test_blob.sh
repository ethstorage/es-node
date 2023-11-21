#!/bin/bash

set -e

./es-utils create --filename test.dat --miner=0xabcd000000000000000000000000000000000000 --kv_len=1 --kv_size 131072 --chunk_size 131072 --encode_type=3
# write with data hash obtained from TestEthStorageContract.sol 
cat blob_8k.dat | ./es-utils blob_write --filename test.dat --kv_idx=0 --kv_entries=1 --kv_size 131072 --chunk_size 131072
./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=1 --readlen=131072 --kv_size 131072 --chunk_size 131072 --commit 0x012b447e85d861d23a6db0a6719129334fe6d20f6db75c14ece0c390436185d1 > blob.tmp

cmp blob_correct.dat blob.tmp  || (echo "cmp failed" && exit 1)

rm test.dat blob.tmp
echo "All tests passed"
