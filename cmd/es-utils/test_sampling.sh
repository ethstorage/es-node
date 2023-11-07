#!/bin/bash

set -e

./es-utils create --filename test.dat --miner=0xabcd000000000000000000000000000000000000 --kv_len=1 --kv_size 131072 --chunk_size 131072 --encode_type=3
# write with data hash obtained from TestEthStorageContract.sol
cat blob_8k.dat | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=1 --kv_size 131072 --chunk_size 131072 --commit 0x0107231cf9721e9e4b44f360798ec498ddaf20bb7a7790380000000000000000
./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=1 --readlen=8192 --kv_size 131072 --chunk_size 131072 --commit 0x0107231cf9721e9e4b44f360798ec498ddaf20bb7a7790380000000000000000 > blob.tmp
cmp blob_8k.dat blob.tmp  || (echo "cmp failed" && exit 1)

[[ $(./es-utils meta_read --filename test.dat --kv_idx=0  --kv_entries=1 --kv_size 131072 --chunk_size 131072) == 0107231cf9721e9e4b44f360798ec498ddaf20bb7a7790380000000000000000 ]] || (echo "meta read failed" && exit 1)

a=$(./es-utils sample_read --filename test.dat --sample_idx=84 --kv_entries=1 --kv_size 131072 --chunk_size 131072 | xxd -p -c 32)
# obtain from zk-decoder with 128K blob
[[ $a == 26a667ded2e63fc1ebb6824c863e5b120df5d28c3f20cc9a3dd04294e738fb5c ]] || (echo "cmp failed" && exit 1)

rm blob.tmp test.dat

echo "All tests passed"
