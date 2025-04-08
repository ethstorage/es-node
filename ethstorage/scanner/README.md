# EthStorage Scanner

A data verification service periodically checks if the data hashes of the blobs in local storage files align with the key-value hashes in the storage contract. If any mismatch found, the service make a query to a remote EthStorage RPC of the blob, and update the data in the local storage. 

This service offers a lightweight yet effective way to maintain network-wide data consistency.

### Usage

The scanner service is enabled with `check meta` mode by default:
- `--scanner.mode`                   Data scan mode, 0: disabled, 1: check meta, 2: check blob (default: 1)[`ES_NODE_SCANNER_MODE`].

The following settings are required if the service is not disabled manually:
- `--scanner.batch-size`             Data scan batch size (default: 8192) [`$ES_NODE_SCANNER_BATCH_SIZE`]
- `--scanner.interval`               Data scan interval in minutes (default: 3) [`$ES_NODE_SCANNER_INTERVAL`]

The RPC configuration is necessary if you need to have the wrong blob fixed automatically:
- `--scanner.es-rpc`                 EthStorage RPC endpoint [`$ES_NODE_SCANNER_ES_RPC`]

