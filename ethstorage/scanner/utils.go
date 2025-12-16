// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

type scanErrors map[uint64]error

func (s scanErrors) add(kvIndex uint64, err error) {
	s[kvIndex] = err
}

func (s scanErrors) nil(kvIndex uint64) {
	s[kvIndex] = nil
}

func (s scanErrors) merge(errs scanErrors) {
	for k, v := range errs {
		if v != nil {
			s[k] = v
		} else {
			delete(s, k)
		}
	}
}

type status int

const (
	pending   status = iota // first-time detected
	fixed                   // by scanner
	recovered               // by downloader
	failed                  // failed to fix
)

func (s status) String() string {
	switch s {
	case pending:
		return "pending"
	case recovered:
		return "recovered"
	case fixed:
		return "fixed"
	case failed:
		return "failed"
	default:
		return "unknown"
	}
}

type mismatchTracker map[uint64]status

func (m mismatchTracker) String() string {
	var items []string
	keys := make([]uint64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, kvIndex := range keys {
		status := m[kvIndex]
		items = append(items, fmt.Sprintf("%d(%s)", kvIndex, status))
	}
	return "[" + strings.Join(items, ",") + "]"
}

func (m mismatchTracker) markPending(kvIndex uint64) {
	m[kvIndex] = pending
}

func (m mismatchTracker) markRecovered(kvIndex uint64) {
	m[kvIndex] = recovered
}

func (m mismatchTracker) markFixed(kvIndex uint64) {
	m[kvIndex] = fixed
}

func (m mismatchTracker) markFailed(kvIndex uint64) {
	m[kvIndex] = failed
}

func (m mismatchTracker) shouldFix(kvIndex uint64) bool {
	status, exists := m[kvIndex]
	return exists && (status == pending || status == failed)
}

// failed() returns all indices that are still mismatched
// since the first-time do not count as mismatched and the
// second-time will be fixed immediately if possible
func (m mismatchTracker) failed() []uint64 {
	return m.filterByStatus(failed)
}

// fixed() returns only indices that have been fixed by the scanner
// add recovered() to get those fixed by downloader
func (m mismatchTracker) fixed() []uint64 {
	return m.filterByStatus(fixed)
}

// recovered() returns indices fixed by downloader from failed status
// those recovered from pending status are no longer tracked
func (m mismatchTracker) recovered() []uint64 {
	return m.filterByStatus(recovered)
}

func (m mismatchTracker) filterByStatus(s status) []uint64 {
	var res []uint64
	for kvIndex, status := range m {
		if status == s {
			res = append(res, kvIndex)
		}
	}
	slices.Sort(res)
	return res
}

func (m mismatchTracker) clone() mismatchTracker {
	clone := make(mismatchTracker)
	maps.Copy(clone, m)
	return clone
}

type scanLoopState struct {
	mode      scanMode
	nextIndex uint64
}

type stats struct {
	mismatched mismatchTracker // tracks all mismatched indices and their status
	errs       scanErrors      // latest scan errors keyed by kv index
}

func newStats() *stats {
	return &stats{
		mismatched: mismatchTracker{},
		errs:       scanErrors{},
	}
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

func formatRange(start, end uint64) string {
	if start == end {
		return fmt.Sprintf("[%d]", start)
	} else if end == start+1 {
		return fmt.Sprintf("[%d,%d]", start, end)
	} else {
		return fmt.Sprintf("[%d-%d]", start, end)
	}
}

type ScanStats struct {
	MismatchedCount int `json:"mismatched_blob"`
	UnfixedCount    int `json:"unfixed_blob"`
}
