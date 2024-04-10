
Es-node Archiver Specification

# Requirements

1. As a long-term data availability solution for rollups, the es-node archiver will serve as a fallback source of blob sidecars besides Ethereum Layer 1.  
2. The way to provide blob is the same as the [getBlobSidecars beacon API](https://ethereum.github.io/beacon-APIs/#/Beacon/getBlobSidecars)  beacon API `/eth/v1/beacon/blob_sidecars/{block_id}` , where you can provide `block_id` in path and `indices` as a query string to download blob sidecars as an array in json format. A live example:
```sh
curl -X 'GET'   'http://88.99.30.186:3500/eth/v1/beacon/blob_sidecars/4700280?indices=0,2'   -H 'accept: application/json' 
```
 
3. The difference in returned data between es-node archiver API and beacon API is that for each blob es-node archiver only returns blob content, kzg commitment, and kzg proof. Other elements will be omitted according to how Optimism uses the service.

Result of es-node archiver API:
```json
{
	"data": [
		{
		"blob": "0x...",
		"kzg_commitment": "0x...",
		"kzg_proof": "0x...",
		},
		...
	]
}
```
For comparison, the `/eth/v1/beacon/blob_sidecars/{block_id}` beacon API returns: 
```json
{
	"data": [
		{
		"index": "0",
		"blob": "0x...",
		"signed_block_header": {
		"message": {
		"slot": "4648270",
		"proposer_index": "1378",
		"parent_root": "0x...",
		"state_root": "0x...",
		"body_root": "0x..."
		},
		"signature": "0x..."
		},
		"kzg_commitment": "0x...",
		"kzg_proof": "0x...",
		"kzg_commitment_inclusion_proof": [...]
		},
	...
	]
}
```
4. Es-node archiver should save all blobs managed by the EthStorage contract instead of only binding to Optimism's BatchInbox contract.
5. Es-node archiver may need to provide the hash of the blob transaction for data verification, but this is depend on the clarification of requirements for Portal network integration.

# Solutions

While an HTTP server will be provided by es-node archiver as an external API service to let the consumer read data from `/eth/v1/beacon/blob_sidecars/{block_id}`, the method of retrieving metadata from L1 by es-node will involve different design approaches.

## Solution 1: pre-stored

1. Download:
In downloader, the metadata (`kzg_commitment`, `kzg_proof`) can be downloaded with blob content through the `/eth/v1/beacon/blob_sidecars/{block_id}` beacon API,  and saved with kv index to leveldb as following:
```
BeaconRootHash->{
		"data": [
			{
			"kv_index": 0,
			"kzg_commitment": "0x...",
			"kzg_proof": "0x...",
			},
			...
			]
		}
```
where `BeaconRootHash` can be queried from `/eth/v1/beacon/blocks/{block_id}/root`

2. Sync:
If the blob retention window has expired and the beacon API is no longer available, the metadata should be synced from peers. In p2p, the `BlobPayload` object will include `BeaconRootHash`, `kzg_commitment`, and `kzg_proof` that can be synced and stored to the key-value table mentioned above.

3. Retrieve:
It is convenient to load `kzg_commitment` and `kzg_proof` from the key-value . 
To load blob data, we can we need to compute `dataHash` from `kzg_commitment` and pass `kvIdx` and `dataHash`(as `commit`)  to es-node storage manager:
 ```
 func (s *StorageManager) TryRead(kvIdx  uint64, readLen  int, commit  common.Hash) ([]byte, bool, error)
 ```

 ###  Considerations for Solution 1
- There will be a flag to control the switch of the archive API, but this flag only controls the external reading interface,  while the downloading and syncing of archive data will always be on.
- Blobs in the latest block are not supported to be downloaded immediately. You must wait for 2 epochs and L1 to download after the block is finalized.

## Solution 2: on the fly

1. From slot to EL block number:
Consider `block_id` as the beacon chain slot number or block root, we can obtain the execution layer block number by querying `/eth/v2/beacon/blocks/{block_id}` and accessing `data.message.body.execution_payload.block_number`.

2. Get KV index:
Next, we can filter logs of the `PutBlob` event by the `block_number` to get `kvIdx` and `dataHash` of the blob uploaded to EthStorage contract:
```
event PutBlob(uint256 indexed kvIdx, uint256 indexed kvSize, bytes32 indexed dataHash)
```
 3. Load blob data:
 Then by passing `kvIdx` and `dataHash`(as `commit`)  we can load blob content using es-node storage manager:
 ```
 func (s *StorageManager) TryRead(kvIdx  uint64, readLen  int, commit  common.Hash) ([]byte, bool, error)
 ```
 4. Compute metadata:
Now that we have blob data, we can generate both `kzg_commitment` and `kzg_proof` by referencing the implementation of the following method:
```
func (p *KZGProver) GenerateKZGProof(data []byte, sampleIdx  uint64) ([]byte, error)
```
5. Retrieve:
When the API is queried, the content of blobs is loaded to generate `kzg_commitment` and `kzg_proof` on demand and served by the server.

### Considerations for  Solution 2:
- It is assumed that the beacon API `/eth/v2/beacon/blocks/{block_id}` is always available for any blocks, and the event log data from the execution layer is always available for any blocks.
- If the performance is proven to be a problem, we may need to add a local cache/prefetch layer to save the metadata.
- If the blob transaction hashes are required, they can also be retrieved through standard JSON-RPC API.
 
