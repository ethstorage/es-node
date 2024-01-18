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
  export ES_NODE_STORAGE_L1CONTRACT_KZG=0x1Cf97d51d305e1e84132Ee504F6B20F5162355fD
fi
# A contract address that clef server checks against before signing the miner transaction
if [ -z "$ES_NODE_STORAGE_L1CONTRACT_CLEF" ]; then
  export ES_NODE_STORAGE_L1CONTRACT_CLEF=0x1Cf97d51d305e1e84132Ee504F6B20F5162355fD
fi
# A newly deployed contract is required for each run for miner test, with zkp verifier of mode 2
if [ -z "$ES_NODE_STORAGE_L1CONTRACT" ]; then
  export ES_NODE_STORAGE_L1CONTRACT=0x1Cf97d51d305e1e84132Ee504F6B20F5162355fD
fi
# A contract with zkp verifier of mode 1 (one proof per sample)
if [ -z "$ES_NODE_STORAGE_L1CONTRACT_ZKP1" ]; then
  export ES_NODE_STORAGE_L1CONTRACT_ZKP1=0x90e945b64F5Fe312dDE12F4aaBa8868f2fad2398
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
zkey_file="./ethstorage/prover/snark_lib/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  zkey_url="https://drive.usercontent.google.com/download?id=1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-&export=download&confirm=t&uuid=16ddcd58-2498-4d65-8931-934df3d0065c"
  curl $zkey_url -o ${zkey_file} 
fi
zkey_file="./ethstorage/prover/snark_lib/blob_poseidon2.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  zkey_url="https://drive.usercontent.google.com/download?id=1olfJvXPJ25Rbcjj9udFlIVr08cUCgE4l&export=download&confirm=t&uuid=724a4ed0-c344-4cc1-9078-f50751028725"
  curl $zkey_url -o ${zkey_file} 
fi

go test -tags rapidsnark_asm -timeout 0 github.com/ethstorage/go-ethstorage/integration_tests -v
