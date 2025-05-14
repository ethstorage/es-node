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

func shortPrt(nums []uint64) string {
	if len(nums) == 0 {
		return ""
	}
	var (
		res        []string
		start, end = nums[0], nums[0]
	)
	for i := 1; i < len(nums); i++ {
		if nums[i] == end+1 {
			end = nums[i]
		} else {
			res = append(res, formatRange(start, end))
			start, end = nums[i], nums[i]
		}
	}
	res = append(res, formatRange(start, end))
	return strings.Join(res, ",")
}

func summaryLocalKvs(shards []uint64, kvEntries, lastKvIdx uint64) string {
	var res []string
	for _, shard := range shards {
		if shard*kvEntries > lastKvIdx {
			// skip empty shards
			break
		}
		lastEntry := shard*kvEntries + kvEntries - 1
		if shard == (lastKvIdx)/kvEntries {
			lastEntry = lastKvIdx
		}
		shardView := fmt.Sprintf("shard%d%s", shard, formatRange(shard*kvEntries, lastEntry))
		res = append(res, shardView)
	}
	return strings.Join(res, ",")
}

func formatRange(start, end uint64) string {
	if start == end {
		return fmt.Sprintf("[%d]", start)
	} else if end == start+1 {
		return fmt.Sprintf("[%d,%d]", start, end)
	} else {
		return fmt.Sprintf("[%d-%d]", start, end)
	}
}
