#!/bin/bash

# usage example 1:
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh
# usage example 2 (overriding rpc urls):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh --l1.rpc <el_rpc> --l1.beacon <cl_rpc>
# usage example 3 (overriding zk options, make sure to use the same configuration when running both init.sh and run.sh):
# env ES_NODE_STORAGE_MINER=<miner> ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run.sh --miner.zk-prover-impl 2 --miner.zk-prover-mode 1

# The following is the default zkey file path downloaded by `init.sh`, which is compatible with zk mode 2. 
# You can override the zkey file by using the `--miner.zkey` flag. Just ensure that the provided zkey file is compatible with the zkey mode.
zkey_file="./build/bin/snark_lib/zkey/blob_poseidon2.zkey"

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
  --miner.enabled \
  --miner.zkey $zkey_file \
  --download.thread 32 \
  --state.upload.url http://metrics.ethstorage.io:8080 \
  --p2p.listen.udp 30305 \
  --p2p.sync.concurrency 32 \
  --p2p.bootnodes enr:-Li4QF3vBkkDQYNLHlVjW5NcEpXAsfNtE1lUVb_LgUQ_Ot2afS8jbDfnYQBDABJud_5Hd1hX_1cNeGVU6Tem06WDlfaGAY1e3vNvimV0aHN0b3JhZ2XbAYDY15SATFINPAhMgF43o16QBXrDKDH5b8GAgmlkgnY0gmlwhEFtP5qJc2VjcDI1NmsxoQK8XODtSv0IsrhBxZmTZBZEoLssb7bTX0YOVl6S0yLxuYN0Y3CCJAaDdWRwgnZh \
$@"

exec $executable $start_flags
