#!/bin/bash

./run.sh \
  --chain_id 333 \
  --storage.l1contract <> \
  --l1.rpc http://65.108.230.142:8550 \
  --l1.beacon http://65.108.230.142:4202 \
  --p2p.bootnodes <> \
  $@
