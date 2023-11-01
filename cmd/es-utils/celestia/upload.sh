#!/bin/sh

# usage:
# env ES_NODE_SIGNER_PRIVATE_KEY=<private_key> ./upload.sh

if [ -z "$ES_NODE_SIGNER_PRIVATE_KEY" ]; then
  echo "Please provide 'ES_NODE_SIGNER_PRIVATE_KEY' as environment variable"
  exit 1
fi

./elestia upload --blob-file blob.dat \
  --da-rpc http://65.108.236.27:26658 \
  --namespace-id 00000000000000003333 \
  --auth-token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.MaHtzm_HvBw810jMsd1Vr4bz1f4oAMPZExRNsOJ9n1g \
  --l1-eth-rpc http://65.108.236.27:8545 \
  --l1-contract 0x878705ba3f8Bc32FCf7F4CAa1A35E72AF65CF766 \
  --private-key $ES_NODE_SIGNER_PRIVATE_KEY \
