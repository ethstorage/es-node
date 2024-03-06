# EthStorage Public Testnet 1 Onboarding Guide

Welcome aboard the EthStorage Public Testnet 1. The Ethereum Dencun upgrade was completed for the Sepolia testnet on Jan. 30 and will soon be launched on the mainnet. Following the Dencun upgrade, the EthStorage team deployed its public testnet on Sepolia. Here's a brief introduction:

- EthStorage supports multiple storage shards, with the first public testnet comprising a single shard of 512GB. Until now, the team has uploaded over 2 million BLOBs to the testnet. Anyone wishing to build with EthStorage is invited to upload their data as well.
- The target interval between two storage proofs is 3 hours, and the permanent storage cost per BLOB is 1,500,000 Gwei. Thus, if a storage provider can submit a storage proof successfully 3 hours after the last submission, he/she can earn approximately 0.1 ETH as a reward. For more details, check out the [calculator](https://docs.google.com/spreadsheets/d/11DHhSang1UZxIFAKYw6_Qxxb-V40Wh1lsYjY2dbIP5k/edit?usp=sharing).
- View the network status and miner rankings on our [dashboard](https://grafana.ethstorage.io).
- Find the [tutorial](https://docs.ethstorage.io/storage-provider-guide/tutorials) for quickly launching an es-node.
- Learn what to expect after launching an es-node [here](https://docs.ethstorage.io/storage-provider-guide/tutorials#two-phases-after-es-node-launch), and for additional information, consult the [FAQ](https://docs.ethstorage.io/storage-provider-guide/storage-provider-faq).

For those interested in exploring the Dencun upgrade, we've developed a tool named [eth-blob-uploader](https://www.npmjs.com/package/eth-blob-uploader). This tool allows you to upload any local file using EIP-4844 BLOBs, and you can inspect these data on-chain with [blobscan](https://blobscan.com).