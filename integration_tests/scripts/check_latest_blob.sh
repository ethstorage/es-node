#!/usr/bin/env bash
set -euo pipefail

# Hardcoded mainnet constants
genesis_time=1606824023
seconds_per_slot=12

# Search back this many blocks for PutBlob events
lookback=2400

ES_CONTRACT_ADDRESS="${ES_CONTRACT_ADDRESS:-0xf0193d6E8fc186e77b6E63af4151db07524f6a7A}"
ARCHIVE_RPC_URL="${ARCHIVE_RPC_URL:-https://archive.mainnet.ethstorage.io:9645}"

EL_RPC_URL="${EL_RPC_URL:-}"
BEACON_API="${BEACON_API:-}"

if [[ -z "${EL_RPC_URL:-}" ]]; then
	echo "Missing EL_RPC_URL" >&2
	exit 2
fi

if [[ -z "${BEACON_API:-}" ]]; then
	echo "Missing BEACON_API" >&2
	exit 2
fi

for bin in cast jq curl; do
	command -v "$bin" >/dev/null 2>&1 || { echo "Missing dependency: $bin" >&2; exit 2; }
done

topic0="$(cast sig-event "PutBlob(uint256,uint256,bytes32)")"

latest_dec="$(cast block-number -r "$EL_RPC_URL")"
if [[ -z "$latest_dec" ]]; then
echo "Failed to get latest block" >&2
exit 1
fi

latest_dec=$((latest_dec - 64))  # adjust to finalized
from_dec=$((latest_dec - lookback))
from_hex="$(cast to-hex "$from_dec")"
to_hex="$(cast to-hex "$latest_dec")"

echo "Searching PutBlob logs"
echo "  contract: $ES_CONTRACT_ADDRESS"
echo "  blocks:    $from_dec..$latest_dec"

params="$(jq -nc --arg from "$from_hex" --arg to "$to_hex" --arg addr "$ES_CONTRACT_ADDRESS" --arg t0 "$topic0" '[{fromBlock:$from,toBlock:$to,address:$addr,topics:[$t0]}]')"
logs_json="$(cast rpc --raw -r "$EL_RPC_URL" eth_getLogs "$params")"

# Handle both possible shapes:
# - raw JSON-RPC: {"jsonrpc":"2.0","id":...,"result":[...]}
# - cast-decoded result: [...]
if echo "$logs_json" | jq -e 'type=="object" and has("error")' >/dev/null 2>&1; then
	echo "EL RPC returned error:" >&2
	echo "$logs_json" | jq -c '.error' >&2
	exit 1
fi

logs_arr="$(echo "$logs_json" | jq -c 'if type=="object" and has("result") then .result elif type=="array" then . else [] end')"

# eth_getLogs result order is ascending by blockNumber/logIndex; take the last entry.
last_log="$(echo "$logs_arr" | jq -c 'last')"
if [[ "$last_log" == "null" || -z "$last_log" ]]; then
	echo "No PutBlob logs found in lookback window" >&2
	exit 1
fi

# PutBlob(uint256 indexed kvIdx, uint256 indexed kvSize, bytes32 indexed dataHash)
dataHash="$(echo "$last_log" | jq -r '.topics[3] // empty')"

if [[ -z "$dataHash" ]]; then
	echo "Missing expected PutBlob topic (need topics[3])" >&2
	echo "$last_log" | jq -c . >&2
	exit 1
fi

echo "PutBlob params"
echo "  dataHash: $dataHash"

block_hex="$(echo "$last_log" | jq -r '.blockNumber')"
tx_hash="$(echo "$last_log" | jq -r '.transactionHash')"

# Get block timestamp
block_json="$(cast rpc --raw -r "$EL_RPC_URL" eth_getBlockByNumber "[\"$block_hex\", false]")"
if echo "$block_json" | jq -e 'type=="object" and has("error")' >/dev/null 2>&1; then
	echo "EL RPC returned error (eth_getBlockByNumber):" >&2
	echo "$block_json" | jq -c '.error' >&2
	exit 1
fi

block_obj="$(echo "$block_json" | jq -c 'if type=="object" and has("result") then .result elif type=="object" then . else empty end')"
if [[ -z "$block_obj" || "$block_obj" == "null" ]]; then
	echo "eth_getBlockByNumber returned empty block for $block_hex" >&2
	exit 1
fi

ts_hex="$(echo "$block_obj" | jq -r '.timestamp // empty')"
if [[ -z "$ts_hex" || "$ts_hex" == "null" ]]; then
	echo "Missing block timestamp in eth_getBlockByNumber response" >&2
	echo "$block_obj" | jq -c . >&2
	exit 1
fi

if [[ "$ts_hex" == 0x* || "$ts_hex" == 0X* ]]; then
	ts_dec=$((16#${ts_hex#0x}))
else
	# Some providers may return decimal timestamps.
	ts_dec=$((ts_hex))
fi

if (( ts_dec < genesis_time )); then
	echo "Block timestamp is before genesis_time" >&2
	exit 1
fi

slot=$(((ts_dec - genesis_time) / seconds_per_slot))

echo "Latest PutBlob"
echo "  tx:    $tx_hash"
echo "  block: $(cast to-dec "$block_hex")"
echo "  slot:  $slot"

# Compare `.data` between beacon blobs API and archive service blobs API.
beacon_url="$BEACON_API/eth/v1/beacon/blobs/$slot?versioned_hashes=${dataHash}"
archive_url="$ARCHIVE_RPC_URL/eth/v1/beacon/blobs/$slot?versioned_hashes=${dataHash}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
	echo "beacon_url=$beacon_url" >> "$GITHUB_OUTPUT"
	echo "archive_url=$archive_url" >> "$GITHUB_OUTPUT"
fi

echo "Fetching blobs (filtered by versioned_hashes)"
echo "  beacon:         $beacon_url"
echo "  archiveService: $archive_url"

beacon_resp="$(
	curl --silent --show-error --location \
		--retry 3 --retry-delay 2 --retry-all-errors \
		--connect-timeout 10 --max-time 60 \
		--write-out '\n%{http_code}' \
		"$beacon_url" \
		|| printf '\n000'
)"
beacon_code="${beacon_resp##*$'\n'}"
beacon_body="${beacon_resp%$'\n'*}"
echo "Beacon /blobs status: $beacon_code"
if [[ "$beacon_code" != "200" ]]; then
	echo "Beacon blobs request failed (HTTP $beacon_code)" >&2
	echo "URL: $beacon_url" >&2
	exit 1
fi

archive_resp="$(
	curl --silent --show-error --location \
		--retry 3 --retry-delay 2 --retry-all-errors \
		--connect-timeout 10 --max-time 60 \
		--write-out '\n%{http_code}' \
		"$archive_url" \
		|| printf '\n000'
)"
archive_code="${archive_resp##*$'\n'}"
archive_body="${archive_resp%$'\n'*}"
echo "ArchiveService /blobs status: $archive_code"
if [[ "$archive_code" != "200" ]]; then
	echo "Archive service blobs request failed (HTTP $archive_code)" >&2
	echo "URL: $archive_url" >&2
	exit 1
fi

# Basic sanity: both responses have `.data` as array
if [[ "$(jq -r '.data | type' <<<"$beacon_body")" != "array" ]]; then
	echo "Unexpected beacon /blobs response shape" >&2
	exit 1
fi
if [[ "$(jq -r '.data | type' <<<"$archive_body")" != "array" ]]; then
	echo "Unexpected archive service /blobs response shape" >&2
	exit 1
fi

# Minimal comparison: hash the full `.data` array JSON and compare.
beacon_data_hash="$(jq -c '.data' <<<"$beacon_body" | sha256sum)"
archive_data_hash="$(jq -c '.data' <<<"$archive_body" | sha256sum)"

if [[ "$beacon_data_hash" == "$archive_data_hash" ]]; then
	echo "✅ Match"
	exit 0
fi

echo "❌ Mismatch" >&2
exit 1