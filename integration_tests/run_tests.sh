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

# download blob_poseidon.zkey if not yet
zkey_file="./ethstorage/prover/snarkjs/blob_poseidon.zkey"
if [ ! -e  ${zkey_file} ]; then
  echo "${zkey_file} not found, start downloading..."
  file_id="1ZLfhYeCXMnbk6wUiBADRAn1mZ8MI_zg-"
  html=`curl -c ./cookie -s -L "https://drive.google.com/uc?export=download&id=${file_id}"`
  curl -Lb ./cookie "https://drive.google.com/uc?export=download&`echo ${html}|grep -Eo 'confirm=[a-zA-Z0-9\-_]+'`&id=${file_id}" -o ${zkey_file}
fi

go test -timeout 0 github.com/ethstorage/go-ethstorage/integration_tests -v
