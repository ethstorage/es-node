// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"
	"strings"
)

type scanError struct {
	kvIndex uint64
	err     error
}
type statsType map[uint64]int // kvIndex -> times

func (s *statsType) String() string {
	var items []string
	for kvIndex, times := range *s {
		var str string
		if times == 1 {
			str = fmt.Sprintf("%d", kvIndex)
		} else {
			str = fmt.Sprintf("%d(%d times)", kvIndex, times)
		}
		items = append(items, str)
	}
	return "[" + strings.Join(items, ",") + "]"
}

type statsSum struct {
	total      int       // total number of kv entries stored in local
	mismatched statsType // mismatched indices with times occurred for each
	fixed      statsType // successfully fixed indices with times occurred for each
	failed     statsType // failed fixed indices with times occurred for each
}

func newStatsSum() *statsSum {
	return &statsSum{
		total:      0,
		mismatched: make(statsType),
		fixed:      make(statsType),
		failed:     make(statsType),
	}
}

func (s *statsSum) update(st stats) {
	s.total = st.total

	for _, kvIndex := range st.mismatched {
		s.mismatched[kvIndex]++
	}
	for _, kvIndex := range st.fixed {
		s.fixed[kvIndex]++
	}
	for _, kvIndex := range st.failed {
		s.failed[kvIndex]++
	}
}

type stats struct {
	total      int      // total number of kv entries stored in local
	mismatched []uint64 // kv indices of mismatched
	fixed      []uint64 // kv indices of successful fix
	failed     []uint64 // kv indices of failed fix
}

func shortPrt(arr []uint64) string {
	if len(arr) <= 6 {
		return fmt.Sprintf("%v", arr)
	}
	return fmt.Sprintf("[%d %d %d... %d %d %d]", arr[0], arr[1], arr[2], arr[len(arr)-3], arr[len(arr)-2], arr[len(arr)-1])
}

func previewLocalKvs(shards []uint64, kvEntries, lastKvIdx uint64) string {
	shardStr := make([]string, len(shards))
	for i, shard := range shards {
		if shard*kvEntries > lastKvIdx {
			// skip empty shards
			break
		}
		lastEntry := shard*kvEntries + kvEntries - 1
		if shard == (lastKvIdx)/kvEntries {
			lastEntry = lastKvIdx
		}
		shardStr[i] = fmt.Sprintf("shard%d[%d-%d]", shard, shard*kvEntries, lastEntry)
	}
	return strings.Join(shardStr, ",")
}
