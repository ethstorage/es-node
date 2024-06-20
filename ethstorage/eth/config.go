package eth

type L1EndpointConfig struct {
	L1ChainID                    uint64 // L1 Chain ID
	L1NodeAddr                   string // Address of L1 User JSON-RPC endpoint to use (eth namespace required)
	L1BlockTime                  uint64 // Block time of L1 chain
	L1BeaconURL                  string // L1 beacon chain endpoint
	L1BeaconBasedTime            uint64 // A pair of timestamp and slot number in the past time
	L1BeaconBasedSlot            uint64 // A pair of timestamp and slot number in the past time
	L1BeaconSlotTime             uint64 // Slot duration
	DAURL                        string // Custom DA URL
	L1MinDurationForBlobsRequest uint64 // Min duration for blobs sidecars request
}
