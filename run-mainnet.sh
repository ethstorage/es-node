#!/bin/bash

# Check if required mainnet flags are provided
has_l1_rpc=0
has_l1_beacon=0

for arg in "$@"; do
  if [[ "$arg" == "--l1.rpc" ]] || [[ "$arg" == --l1.rpc=* ]]; then
    has_l1_rpc=1
  elif [[ "$arg" == "--l1.beacon" ]] || [[ "$arg" == --l1.beacon=* ]]; then
    has_l1_beacon=1
  fi
done

# Exit to avoid using testnet RPCs in run.sh if mainnet RPCs are not provided
if [[ $has_l1_rpc -eq 0 ]]; then
  echo "Error: --l1.rpc is required for mainnet"
  echo "Usage: $0 --l1.rpc <mainnet_rpc_url> --l1.beacon <mainnet_beacon_api_url> [other_options]"
  exit 1
fi

if [[ $has_l1_beacon -eq 0 ]]; then
  echo "Error: --l1.beacon is required for mainnet"
  echo "Usage: $0 --l1.rpc <mainnet_rpc_url> --l1.beacon <mainnet_beacon_api_url> [other_options]"
  exit 1
fi

./run.sh \
  --chain_id 333 \
  --storage.l1contract 0xf0193d6E8fc186e77b6E63af4151db07524f6a7A \
  --p2p.bootnodes enr:-Lq4QLCzFy9BF47Y4OLjTvuG3r1moav7GWddahRASXja7WOCJlD0zo-I8mIzv1zfhW6toGmMvgPSMdwzO9VDaPZ18mSGAZky3V-fimV0aHN0b3JhZ2XdggFNgNjXlPAZPW6PwYbne25jr0FR2wdST2p6wYCCaWSCdjSCaXCEF1hGrolzZWNwMjU2azGhAuy54NtDdUk7YvhyNJMwCNNR_h1QaXcoB2ewiJ8DocwFg3RjcIIkCoN1ZHCCdmI \
  "$@"
