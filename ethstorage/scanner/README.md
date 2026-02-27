# EthStorage Scanner

A data verification service that periodically checks whether locally stored KV blobs match the on-chain KV meta hash (commit) from the storage contract.

If a mismatch is detected, the scanner can attempt to repair the local data by re-fetching the blob from the network and rewriting it with meta validation.

## Options

| Flag | Default | Env var | Description |
| --- | --- | --- | --- |
| `--scanner.mode` | `1` | `ES_NODE_SCANNER_MODE` | Data scan mode (bitmask): `0`=disabled, `1`=meta, `2`=blob, `4`=block. Combine via sum/OR (e.g. `3`=`1+2`, `5`=`1+4`, `7`=`1+2+4`). |
| `--scanner.batch-size` | `8192` | `ES_NODE_SCANNER_BATCH_SIZE` | The number of KVs to scan per batch for check-meta and check-blob modes. No impact on check-block mode. |
| `--scanner.interval.meta` | `3` (minutes) | `ES_NODE_SCANNER_INTERVAL_META` | Scan interval for `check-meta`. |
| `--scanner.interval.blob` | `60` (minutes) | `ES_NODE_SCANNER_INTERVAL_BLOB` | Scan interval for `check-blob`. |
| `--scanner.interval.block` | `1440` (minutes) | `ES_NODE_SCANNER_INTERVAL_BLOCK` | Scan interval for `check-block`. |

## Scan modes explained

The flag / env `--scanner.mode` (default: `1`) [`ES_NODE_SCANNER_MODE`] is a bitmask:

- `0`: disabled
- `1`: check-meta (compare local meta with on-chain meta)
- `2`: check-blob (read local blob and validate its commit against on-chain meta)
- `4`: check-block (scan recently updated KVs from finalized blocks, then run check-blob on them)

### Quick comparison

| Name | `--scanner.mode` | What it does | Performance impact | Notes |
| --- | ---: | --- | --- | --- |
| check-meta | `1` | Read local meta and compare with on-chain meta | Low | Minimal impact; may miss some mismatches |
| check-blob | `2` | Compute the commit from a local blob and validate it against on-chain meta | High | Best precision; highest IO/CPU cost when many blobs |
| check-block | `4` | Scan recently finalized blocks for updated KVs, then run `check-blob` on them | High | Ensure newly updated blobs are fetched and verified within the Beacon node retention window |

### More choices

You can combine modes by summing/OR-ing them to get mixed behavior and balance precision, coverage vs performance:

- `3` = `1 + 2` = meta + blob
- `5` = `1 + 4` = meta + block
- `6` = `2 + 4` = blob + block
- `7` = meta + blob + block


> [!TIP] 
> `--scanner.batch-size` and `--scanner.interval.*` control the batch size and frequency of each scan mode, so you can tune the performance impact further based on the amount of data and hardware resources.

> [!NOTE] 
> `--scanner.batch-size` only affects `check-meta` and `check-blob` modes, not `check-block`. 

> [!WARNING] 
> If `--scanner.batch-size` is set higher than the default, it may cause `out of gas` error while querying meta data from the L1 contract.

## Status tracking

When es-node starts, the scanner only starts after the node finishes syncing all shards from the P2P network.

After it starts, the scanner periodically logs summaries and statistics (mismatched/unfixed counts). These counts are also exposed in the node state as `scan_stats`.

## Repair behavior

A background repair loop periodically retries/fixes mismatched KVs by fetching blobs from the p2p network and rewriting them locally.

- If mismatches are detected for the first time, the KV is marked as `pending`, meaning it is scheduled for repair. Sometimes the mismatch is transient (e.g., due to download latency) and may be recovered automatically by the downloader.
- If the KV is repaired successfully or recovered, it is removed from the mismatch list.
- If the repair fails, it remains in the mismatch list and is marked as `failed` for future retries.
