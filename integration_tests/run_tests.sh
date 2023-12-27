#!/bin/sh

# usage:
# env ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./run_tests.sh

if [ -z "$ES_NODE_SIGNER_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_SIGNER_PRIVATE_KEY' as an environment variable"
  exit 1
fi

if [ ${#ES_NODE_SIGNER_PRIVATE_KEY} -ne 64 ]; then
  echo "Error: ES_NODE_SIGNER_PRIVATE_KEY should have a length of 64"
  exit 1
fi

if [ -z "$ES_NODE_STORAGE_MINER" ]; then
  export ES_NODE_STORAGE_MINER=0x534632D6d7aD1fe5f832951c97FDe73E4eFD9a77
fi

if [ -z "$ES_NODE_STORAGE_L1CONTRACT" ]; then
  export ES_NODE_STORAGE_L1CONTRACT=0x8b401c82Eb4001a58f072CE572D568Ca78f4C526
fi

if [ -z "$ES_NODE_L1_ETH_RPC" ]; then
  export ES_NODE_L1_ETH_RPC=http://65.109.115.36:8545
fi

echo ES_NODE_L1_ETH_RPC = $ES_NODE_L1_ETH_RPC
echo ES_NODE_STORAGE_L1CONTRACT = $ES_NODE_STORAGE_L1CONTRACT
echo ES_NODE_STORAGE_MINER = $ES_NODE_STORAGE_MINER

# download blob_poseidon.zkey if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  file_id="1V3QkMpk5UC48Jc62nHXMgzeMb6KE8JRY"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
  rm cookie
fi

go test -timeout 0 github.com/ethstorage/go-ethstorage/integration_tests -v
