// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package pora

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	pora "github.com/ethstorage/go-ethstorage/ethstorage/pora/ethash"
)

var caches = pora.NewLRU("cache", 2, pora.NewCache)

const CHUNK_SIZE = uint64(4096)

func Cache(epoch uint64) *pora.Cache {
	currentI, futureI := caches.Get(epoch)
	current := currentI.(*pora.Cache)

	// Wait for generation finish.
	current.Generate("", 0, false, false)

	// If we need a new future cache, now's a good time to regenerate it.
	if futureI != nil {
		future := futureI.(*pora.Cache)
		go future.Generate("", 0, false, false)
	}
	return current
}

func ToRealHash(hash common.Hash, maxKvSize, idxWithinChunk uint64, realHash []byte, copyHash bool) []byte {
	if len(realHash) != len(hash)+8 {
		realHash = make([]byte, len(hash)+8)
		// always copy hash if newly allocated
		copy(realHash, hash[:])
	} else {
		// otherwise only copy hash if requested
		if copyHash {
			copy(realHash, hash[:])
		}
	}

	binary.BigEndian.PutUint64(realHash[len(hash):], maxKvSize+((idxWithinChunk+1)<<30) /* should be fine as long as MaxKvSize is < 2^30 */)
	return realHash
}

type PhyAddr struct {
	KvIdx  uint64
	KvSize int
	Commit common.Hash
}

func GetMaskDataWithInChunk(epoch uint64, chunkHash common.Hash, maxKvSize uint64, sizeInChunk int, maskBuffer []byte) []byte {

	if sizeInChunk > int(CHUNK_SIZE) {
		panic("sizeInChunk > CHUNK_SIZE")
	}
	if len(maskBuffer) != sizeInChunk {
		maskBuffer = make([]byte, sizeInChunk)
	}

	cache := Cache(epoch)
	size := pora.DatasetSizeForEpoch(epoch)

	realHash := make([]byte, len(chunkHash)+8)
	copy(realHash, chunkHash[:])

	for i := 0; i < sizeInChunk/pora.GetMixBytes(); i++ {
		ToRealHash(chunkHash, maxKvSize, uint64(i), realHash, false)
		mask := pora.HashimotoForMaskLight(size, cache.Cache, realHash)
		if len(mask) != pora.GetMixBytes() {
			panic("#mask != MixBytes")
		}
		copy(maskBuffer[i*pora.GetMixBytes():], mask)
	}

	tailBytes := sizeInChunk % pora.GetMixBytes()
	if tailBytes > 0 {
		i := sizeInChunk / pora.GetMixBytes()
		ToRealHash(chunkHash, maxKvSize, uint64(i), realHash, false)
		mask := pora.HashimotoForMaskLight(size, cache.Cache, realHash)
		if len(mask) != pora.GetMixBytes() {
			panic("#mask != MixBytes")
		}
		copy(maskBuffer[i*pora.GetMixBytes():], mask)
	}

	return maskBuffer
}

func GetMaskData(epoch uint64, encodeKey common.Hash, chunkSize int, maskBuffer []byte) []byte {
	if len(maskBuffer) != chunkSize {
		maskBuffer = make([]byte, chunkSize)
	}

	cache := Cache(epoch)
	size := pora.DatasetSizeForEpoch(epoch)

	realHash := make([]byte, len(encodeKey)+8)
	copy(realHash, encodeKey[:])

	for i := 0; i < (chunkSize+pora.GetMixBytes()-1)/pora.GetMixBytes(); i++ {
		binary.BigEndian.PutUint64(realHash[len(encodeKey):], uint64(i))
		mask := pora.HashimotoForMaskLight(size, cache.Cache, realHash)
		if len(mask) != pora.GetMixBytes() {
			panic("#mask != MixBytes")
		}
		copy(maskBuffer[i*pora.GetMixBytes():], mask)
	}

	return maskBuffer
}
