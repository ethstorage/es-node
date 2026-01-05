// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package scanner

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type scanned struct {
	status
	err error
}

type status int

const (
	ok        status = iota
	err_read         // read meta or blob error / not found
	pending          // mismatch detected
	fixed            // by scanner
	recovered        // by downloader
	failed           // failed to fix
)

func (s status) String() string {
	switch s {
	case ok:
		return "ok"
	case err_read:
		return "err_read"
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

// failed() returns all kvIndices that are failed to be fixed
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

// needFix() returns all kvIndices that need to be fixed or at least check again
func (m scannedKVs) needFix() []uint64 {
	var res []uint64
	for kvIndex, scanned := range m {
		if scanned.status == pending || scanned.status == failed || scanned.err != nil {
			res = append(res, kvIndex)
		}
	}
	slices.Sort(res)
	return res
}

type scanLoopRuntime struct {
	mode      scanMode
	nextBatch nextBatchFn
	interval  time.Duration
	batchSize uint64
	nextIndex uint64
}

type scanUpdateFn func(uint64, *scanned)
type nextBatchFn func(uint64, uint64) ([]uint64, uint64)

type scanMarker struct {
	kvIndex uint64
	mark    scanUpdateFn
}

func newScanMarker(kvIndex uint64, fn scanUpdateFn) *scanMarker {
	return &scanMarker{kvIndex: kvIndex, mark: fn}
}

func (m *scanMarker) markError(commit common.Hash, err error) {
	m.mark(m.kvIndex, &scanned{status: err_read, err: fmt.Errorf("commit: %x, error reading kv: %w", commit, err)})
}

func (m *scanMarker) markFailed(commit common.Hash, err error) {
	m.mark(m.kvIndex, &scanned{status: failed, err: fmt.Errorf("commit: %x, error fixing kv: %w", commit, err)})
}

func (m *scanMarker) markMismatched() {
	m.mark(m.kvIndex, &scanned{status: pending, err: nil})
}

func (m *scanMarker) markFixed() {
	m.mark(m.kvIndex, &scanned{status: fixed, err: nil})
}

func (m *scanMarker) markRecovered() {
	m.mark(m.kvIndex, &scanned{status: recovered, err: nil})
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
