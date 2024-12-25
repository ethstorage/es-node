#!/bin/bash

# usage:
# env ES_NODE_STORAGE_MINER=<miner> ./init-l2.sh


./init.sh \
  --l1.rpc http://5.9.87.214:8545 \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
$@
