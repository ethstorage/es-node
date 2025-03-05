#!/bin/bash

./run.sh \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --l1.rpc http://5.9.87.214:8545 \
  --l1.block_time 2 \
  --l2.chain_id 3337 \
  --da.url http://5.9.87.214:8888 \
  --randao.url http://88.99.30.186:8545 \
  --p2p.bootnodes enr:-Li4QFWB-bdjo4EVuvJmJ4GmMuwa0RcFdtdpA6yUxTdU0QE8aG_q8eZa9B4vu2LGPTbspqFtMBCuJpjG0q178SY7H1yGAZP7b35dimV0aHN0b3JhZ2XbAYDY15RkADrb3zAU9-OPxr51LrBHuV2omsGAgmlkgnY0gmlwhEFtMpGJc2VjcDI1NmsxoQMw4i1lk4EjCD_4os-PSrC9y1BrDim_olfCCPNBoXSkWIN0Y3CCJAaDdWRwgnZh \
 $@