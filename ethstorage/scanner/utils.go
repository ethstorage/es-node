// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

type scanned struct {
	status
	err error
}

type status int

const (
	ok        status = iota
	notFound         // not found
	pending          // first-time detected
	fixed            // by scanner
	recovered        // by downloader
	failed           // failed to fix
)

func (s status) String() string {
	switch s {
	case ok:
		return "ok"
	case notFound:
		return "not_found"
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

type scannedKVs map[uint64]scanned

func (m scannedKVs) String() string {
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

func (m scannedKVs) hasError() bool {
	for _, scanned := range m {
		if scanned.err != nil {
			return true
		}
	}
	return false
}

func (m scannedKVs) withErrors() map[uint64]error {
	res := make(map[uint64]error)
	for kvIndex, scanned := range m {
		if scanned.err != nil {
			res[kvIndex] = scanned.err
		}
	}
	return res
}

// failed() returns all indices that are still mismatched
// since the first-time do not count as mismatched and the
// second-time will be fixed immediately if possible
func (m scannedKVs) failed() []uint64 {
	var res []uint64
	for kvIndex, scanned := range m {
		if scanned.status == failed {
			res = append(res, kvIndex)
		}
	}
	slices.Sort(res)
	return res
}

type scanLoopState struct {
	mode      scanMode
	nextIndex uint64
}

type scanUpdateFn func(kvi uint64, m *scanned)

type scanMarker struct {
	kvIndex uint64
	mark    scanUpdateFn
}

func newScanMarker(kvIndex uint64, fn scanUpdateFn) *scanMarker {
	return &scanMarker{kvIndex: kvIndex, mark: fn}
}

func (m *scanMarker) markError(commit common.Hash, err error) {
	m.mark(m.kvIndex, &scanned{status: notFound, err: err})
}

func (m *scanMarker) markPending() {
	m.mark(m.kvIndex, &scanned{status: pending, err: nil})
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
