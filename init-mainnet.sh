#!/bin/bash

has_l1_rpc=0
for arg in "$@"; do
  if [[ "$arg" == "--l1.rpc" ]] || [[ "$arg" == --l1.rpc=* ]]; then
    has_l1_rpc=1
    break
  fi
done

# Exit to avoid using testnet RPC in init.sh if --l1.rpc is not provided
if [[ $has_l1_rpc -eq 0 ]]; then
  echo "Error: --l1.rpc is required for mainnet initialization"
  echo "Usage: $0 --l1.rpc <mainnet_rpc_url> [other_options]"
  exit 1
fi

./init.sh \
  --storage.l1contract 0xf0193d6E8fc186e77b6E63af4151db07524f6a7A \
  "$@"
