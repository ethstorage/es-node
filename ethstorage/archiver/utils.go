package archiver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var knownIds = []string{"genesis", "finalized", "head"}

func validateBlockID(id string) *httpError {
	if isHash(id) || isSlot(id) || slices.Contains(knownIds, id) {
		return nil
	}
	return newBadRequestError(fmt.Sprintf("Invalid block ID: %s", id))
}

func isHash(s string) bool {
	if len(s) != 66 || !strings.HasPrefix(s, "0x") {
		return false
	}
	_, err := hexutil.Decode(s)
	return err == nil
}

func isSlot(id string) bool {
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}

func indexIncluded(index uint64, indices []uint64) bool {
	if len(indices) == 0 {
		return true
	}
	return slices.Contains(indices, index)
}

func hashIncluded(hash common.Hash, hashes []common.Hash) bool {
	if len(hashes) == 0 {
		return true
	}
	return slices.Contains(hashes, hash)
}

type httpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e httpError) write(w http.ResponseWriter) {
	w.WriteHeader(e.Code)
	_ = json.NewEncoder(w).Encode(e)
}

func (e httpError) Error() string {
	return e.Message
}

var (
	errUnknownBlock = &httpError{
		Code:    http.StatusNotFound,
		Message: "Block not found",
	}
	errBlobNotInES = &httpError{
		Code:    http.StatusNotFound,
		Message: "Blob not found in EthStorage",
	}
	errServerError = &httpError{
		Code:    http.StatusInternalServerError,
		Message: "Internal server error",
	}
	// errUpstreamError covers beacon node transport errors, timeouts, 5xx
	// responses, invalid JSON, and blocks missing required fields. It is a 5xx
	// so that callers (e.g. op-node) retry instead of treating it as "block
	// not found".
	errUpstreamError = &httpError{
		Code:    http.StatusBadGateway,
		Message: "Upstream beacon node error",
	}
	// errUnsupportedBeaconVersion is returned when the beacon node reports a
	// version this archiver cannot decode.
	errUnsupportedBeaconVersion = &httpError{
		Code:    http.StatusBadGateway,
		Message: "Unsupported beacon block version",
	}
	// errExecutionBlockNotAvailable is returned when the beacon block resolved
	// fine but the execution block it points to is not known to the L1 client
	// yet (e.g. a Gloas bid whose payload has not arrived). Retryable 5xx.
	errExecutionBlockNotAvailable = &httpError{
		Code:    http.StatusServiceUnavailable,
		Message: "Execution block not available on L1 yet",
	}
)

// isUnknownBlockErr reports whether an eth_getLogs error means the queried
// block hash is unknown to the execution client (geth answers with
// "unknown block"), as opposed to a transport failure.
func isUnknownBlockErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "unknown block")
}

func newBadRequestError(input string) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: input,
	}
}

func parseVersionedHashes(r *http.Request) ([]common.Hash, *httpError) {
	query := r.URL.Query()
	normalizeQueryValues(query)
	r.URL.RawQuery = query.Encode()
	rawHashes := r.URL.Query()["versioned_hashes"]
	hashes := make([]common.Hash, 0, len(rawHashes))
	for _, raw := range rawHashes {
		hashes = append(hashes, common.HexToHash(raw))
	}
	return hashes, nil
}

// parseIndices filters out invalid and duplicate blob indices
func parseIndices(r *http.Request, max int) ([]uint64, *httpError) {
	query := r.URL.Query()
	normalizeQueryValues(query)
	r.URL.RawQuery = query.Encode()
	rawIndices := r.URL.Query()["indices"]
	indices := make([]uint64, 0, max)
	invalidIndices := make([]string, 0)
loop:
	for _, raw := range rawIndices {
		ix, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		if max > 0 && ix >= uint64(max) {
			invalidIndices = append(invalidIndices, raw)
			continue
		}
		for i := range indices {
			if ix == indices[i] {
				continue loop
			}
		}
		indices = append(indices, ix)
	}

	if len(invalidIndices) > 0 {
		return nil, newBadRequestError(fmt.Sprintf("requested blob indices %v are invalid", invalidIndices))
	}
	return indices, nil
}

// normalizeQueryValues replaces comma-separated values with individual values
func normalizeQueryValues(queryParams url.Values) {
	for key, vals := range queryParams {
		splitVals := make([]string, 0)
		for _, v := range vals {
			splitVals = append(splitVals, strings.Split(v, ",")...)
		}
		queryParams[key] = splitVals
	}
}

func readUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
