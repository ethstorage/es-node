#!/bin/bash

set -e


test_string=abcdefg
test_string1=112233445566

./es-utils create --filename test.dat --miner=0x0000000000000000000000000000000000001234 --len=1024
echo $test_string | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=16
a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

rm test.dat

# Test with encoding
./es-utils create --filename test.dat --miner=0x0000000000000000000000000000000000001234 --len=1024 --encode_type=1
echo $test_string | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=16
a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

echo $test_string | ./es-utils shard_write --filename test.dat --kv_idx=1 --kv_entries=16 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] && (echo "cmp failed" && exit 1)
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=7 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

rm test.dat

echo "Testing Ethash encdec"

./es-utils create --filename test.dat --miner=0x0000000000000000000000000000000000001234 --len=1024 --encode_type=2
echo $test_string | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=16
a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

echo $test_string1 | ./es-utils shard_write --filename test.dat --kv_idx=1 --kv_entries=16 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=12)
[[ $test_string1 == $a ]] && (echo "cmp failed" && exit 1)
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=12 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa)
[[ $test_string1 == $a ]] || (echo "cmp failed" && exit 1)

a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

echo "Testing BLOB Poseidon encdec"

./es-utils create --filename test.dat --miner=0x0000000000000000000000000000000000001234 --len=1024 --encode_type=3
echo $test_string | ./es-utils shard_write --filename test.dat --kv_idx=0 --kv_entries=16
a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

echo $test_string1 | ./es-utils shard_write --filename test.dat --kv_idx=1 --kv_entries=16 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=12)
[[ $test_string1 == $a ]] && (echo "cmp failed" && exit 1)
a=$(./es-utils shard_read --filename test.dat --kv_idx=1 --kv_entries=16 --readlen=12 --commit=0x00000000000000000000000000000000000000000000000000000000000000aa)
[[ $test_string1 == $a ]] || (echo "cmp failed" && exit 1)

a=$(./es-utils shard_read --filename test.dat --kv_idx=0 --kv_entries=16 --readlen=7)
[[ $test_string == $a ]] || (echo "cmp failed" && exit 1)

echo "All tests passed"
