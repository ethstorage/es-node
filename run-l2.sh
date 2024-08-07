#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run-l2.sh

zkey_file="./build/bin/snark_lib/zkey/blob_poseidon2.zkey"

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  echo "Please provide 'ES_NODE_STORAGE_MINER' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_STORAGE_MINER} -ne 42 ] || case $ES_NODE_STORAGE_MINER in 0x*) false;; *) true;; esac; then
  echo "Error: ES_NODE_STORAGE_MINER should be prefixed with '0x' and have a total length of 42"
  exit 1
fi

if [ -z "$ES_NODE_SIGNER_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_SIGNER_PRIVATE_KEY' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_SIGNER_PRIVATE_KEY} -ne 64 ]; then
  echo "Error: ES_NODE_SIGNER_PRIVATE_KEY should have a length of 64"
  exit 1
fi

executable="./build/bin/es-node"
echo "========== build info =================="
$executable --version
echo "========================================"

data_dir="./es-data"
file_flags=""
 
for file in ${data_dir}/shard-[0-9]*.dat; do 
    if [ -f "$file" ]; then 
        file_flags+=" --storage.files $file"
    fi
done

start_flags=" --network devnet \
  --datadir $data_dir \
  $file_flags \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --storage.miner $ES_NODE_STORAGE_MINER \
  --l1.rpc http://65.109.20.29:8545 \
  --l1.block_time 2 \
  --da.url http://65.109.20.29:8888 \
  --randao.url http://88.99.30.186:8545 \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --miner.enabled \
  --miner.zkey $zkey_file \
  --miner.zk-prover-impl 1 \
  --download.thread 32 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QA-fcxSHHu68uzHxsGnR8Q8lnvPir8L3cb5-RSq5fvU7cmxukZinZ9N-XRcnvWauQl6KK2tnlD3RZTwOxI4KgIaGAZC-hjTfimV0aHN0b3JhZ2XbAYDY15RkADrb3zAU9-OPxr51LrBHuV2omsGAgmlkgnY0gmlwhEFtcySJc2VjcDI1NmsxoQNY8raIsHIGPniQ738UiNmIvifax5L6R51YLPoflGzix4N0Y3CCJAaDdWRwgnZh \
 $@"

exec $executable $start_flags