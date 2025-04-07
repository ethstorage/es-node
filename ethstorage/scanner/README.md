# EthStorage Scanner

A data verification service periodically checks if the data hashes of the blobs in local storage files align with the key-value hashes in the storage contract. If any mismatch found, the service make a query to a remote EthStorage RPC of the blob, and update the data in the local storage. 

This service offers a lightweight yet effective way to maintain network-wide data consistency.

### Usage

The scanner service is enabled as part of es-node process by default but can be disabled by `--scanner.enabled=false` or `ES_NODE_SCANNER_ENABLED=false`.

The following settings are required if the service is not disabled manually:
- `--scanner.batch-size`             Data scan batch size (default: 4096) [`$ES_NODE_SCANNER_BATCH_SIZE`]
- `--scanner.interval`               Data scan interval in minutes (default: 4) [`$ES_NODE_SCANNER_INTERVAL`]

The RPC configuration is necessary if you need to have the wrong blob fixed automatically:
- `--scanner.es-rpc`                 EthStorage RPC endpoint [`$ES_NODE_SCANNER_ES_RPC`]

