#!/bin/bash

./run.sh \
  --storage.l1contract 0x64003adbdf3014f7E38FC6BE752EB047b95da89A \
  --l1.rpc http://65.109.20.29:8545 \
  --l1.block_time 2 \
  --da.url http://65.109.20.29:8888 \
  --randao.url http://88.99.30.186:8545 \
  --p2p.bootnodes enr:-Li4QA-fcxSHHu68uzHxsGnR8Q8lnvPir8L3cb5-RSq5fvU7cmxukZinZ9N-XRcnvWauQl6KK2tnlD3RZTwOxI4KgIaGAZC-hjTfimV0aHN0b3JhZ2XbAYDY15RkADrb3zAU9-OPxr51LrBHuV2omsGAgmlkgnY0gmlwhEFtcySJc2VjcDI1NmsxoQNY8raIsHIGPniQ738UiNmIvifax5L6R51YLPoflGzix4N0Y3CCJAaDdWRwgnZh \
 $@