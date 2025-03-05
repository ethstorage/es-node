#!/bin/bash

./run.sh \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --l1.rpc http://5.9.87.214:8545 \
  --l1.block_time 2 \
  --l2.chain_id 3337 \
  --da.url http://5.9.87.214:8888 \
  --randao.url http://88.99.30.186:8545 \
  --p2p.bootnodes enr:-Lq4QJntX8Wl0CDHg3z32mcILXLiplkRIJnCJynEmGTgEJ4XVjNsum7qQsndJsfc4h_5lWwVU-Nq-Hs3A-JIoO8KVfiGAZP7b4KIimV0aHN0b3JhZ2Xdgg0JgNjXlGQAOtvfMBT344_GvnUusEe5XaiawYCCaWSCdjSCaXCEQW0ykYlzZWNwMjU2azGhAzDiLWWTgSMIP_iiz49KsL3LUGsOKb-iV8II80GhdKRYg3RjcIIkBoN1ZHCCdmE \
 $@