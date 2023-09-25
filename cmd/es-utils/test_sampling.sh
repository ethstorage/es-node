#!/bin/bash

set -e

./es-utils create --filename test.dat --miner=0xabcd000000000000000000000000000000000000 --len=1 --kv_size 131072 --chunk_size 131072 --encode_type=3
# write with data hash obtained from TestEthStorageContract.sol 
cat blob_8k.dat | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=1 --kv_size 131072 --chunk_size 131072 --commit 0xd2f03eafdd8f3de1aba543e57db4936427703cc09bdb7d000000000000000000
./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=1 --readlen=8192 --kv_size 131072 --chunk_size 131072 --commit 0xd2f03eafdd8f3de1aba543e57db4936427703cc09bdb7d000000000000000000 > blob.tmp
cmp blob_8k.dat blob.tmp  || (echo "cmp failed" && exit 1)

[[ $(./es-utils meta_read --filename test.dat --kv_idx=0  --kv_entries=1 --kv_size 131072 --chunk_size 131072) == d2f03eafdd8f3de1aba543e57db4936427703cc09bdb7d000000000000000000 ]] || (echo "meta read failed" && exit 1)

a=$(./es-utils sample_read --filename test.dat --sample_idx=84 --kv_entries=1 --kv_size 131072 --chunk_size 131072 | xxd -p -c 32)
# obtain from zk-decoder with 128K blob
[[ $a == 220126f58594d237ca2cddc84670a3ebb004e745a57b22acbbaf335d2c13fcd2 ]] || (echo "cmp failed" && exit 1)

rm blob.tmp test.dat

echo "All tests passed"