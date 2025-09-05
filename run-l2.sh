#!/bin/bash

./run.sh \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --l1.rpc http://5.9.87.214:8545 \
  --l1.block_time 2 \
  --chain_id 3337 \
  --da.url http://5.9.87.214:8888 \
  --randao.url http://65.108.230.142:8545 \
  --p2p.bootnodes enr:-Lq4QEXYjk9uxm2_r-JXoXnpqMu54YIIrTFYd7sfQKAgFQL1fiq4mkUHGkV6mIas6sh7klavj5z6sFZYRfvtldSq6LOGAZb2MuHbimV0aHN0b3JhZ2Xdgg0JgNjXlGQAOtvfMBT344_GvnUusEe5XaiawYCCaWSCdjSCaXCEisl6PYlzZWNwMjU2azGhAgsmoUh0dDjPTG5kiyxCDwG5VNK_d1WCSJ4KkVJ6-57zg3RjcIIkBoN1ZHCCdmE \
 $@
