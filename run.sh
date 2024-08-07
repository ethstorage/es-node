#!/bin/bash

# usage 1:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
# usage 2 (overriding rpc urls):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh --l1.rpc <el_rpc> --l1.beacon <cl_rpc>

# Note: currently only zk prover mode 2 is supported
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
  --storage.l1contract 0x804C520d3c084C805E37A35E90057Ac32831F96f \
  --storage.miner $ES_NODE_STORAGE_MINER \
  --l1.rpc http://88.99.30.186:8545 \
  --l1.beacon http://88.99.30.186:3500 \
  --l1.beacon-based-time 1706684472 \
  --l1.beacon-based-slot 4245906 \
  --signer.private-key $ES_NODE_SIGNER_PRIVATE_KEY \
  --miner.enabled \
  --miner.zkey $zkey_file \
  --miner.zk-prover-impl 1 \
  --download.thread 32 \
  --state.upload.url http://metrics.ethstorage.io:8080 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QF3vBkkDQYNLHlVjW5NcEpXAsfNtE1lUVb_LgUQ_Ot2afS8jbDfnYQBDABJud_5Hd1hX_1cNeGVU6Tem06WDlfaGAY1e3vNvimV0aHN0b3JhZ2XbAYDY15SATFINPAhMgF43o16QBXrDKDH5b8GAgmlkgnY0gmlwhEFtP5qJc2VjcDI1NmsxoQK8XODtSv0IsrhBxZmTZBZEoLssb7bTX0YOVl6S0yLxuYN0Y3CCJAaDdWRwgnZh \
$@"

exec $executable $start_flags
