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
# A contract that will be update with new blob uploaded for the KZG test
if [ -z "$ES_NODE_STORAGE_L1CONTRACT_KZG" ]; then
  export ES_NODE_STORAGE_L1CONTRACT_KZG=0x1ba144ad60008A66956e8C00AB2057a7db2c8d55
fi
# A contract address that clef server checks against before signing the miner transaction
if [ -z "$ES_NODE_STORAGE_L1CONTRACT_CLEF" ]; then
  export ES_NODE_STORAGE_L1CONTRACT_CLEF=0xb4B46bdAA835F8E4b4d8e208B6559cD267851051
fi
# A newly deployed contract is required for each run for miner test
if [ -z "$ES_NODE_STORAGE_L1CONTRACT" ]; then
  export ES_NODE_STORAGE_L1CONTRACT=0xbE3c2303f263C24d91D0fA3a169bb542E89FBdDa
fi
# The commonly used l1 eth rpc endpoint
if [ -z "$ES_NODE_L1_ETH_RPC" ]; then
  export ES_NODE_L1_ETH_RPC=http://65.109.115.36:8545
fi
# The clef endpoint that the miner will use to sign the transaction
if [ -z "$ES_NODE_CLEF_RPC" ]; then
  export ES_NODE_CLEF_RPC="http://65.108.236.27:8550"
fi

echo ES_NODE_L1_ETH_RPC = $ES_NODE_L1_ETH_RPC
echo ES_NODE_STORAGE_L1CONTRACT = $ES_NODE_STORAGE_L1CONTRACT
echo ES_NODE_STORAGE_MINER = $ES_NODE_STORAGE_MINER

# download zkeys if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
  rm cookie
fi
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon2.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  file_id="1V3QkMpk5UC48Jc62nHXMgzeMb6KE8JRY"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
  rm cookie
fi

go test -timeout 0 github.com/ethstorage/go-ethstorage/integration_tests -v
