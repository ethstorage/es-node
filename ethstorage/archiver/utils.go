package archiver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

var knownIds = []string{"genesis", "finalized", "head"}

func validateBlockID(id string) *httpError {
	if isHash(id) || isSlot(id) || isKnownIdentifier(id) {
		return nil
	}
	return newBlockIdError(id)
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

func isKnownIdentifier(id string) bool {
	for _, element := range knownIds {
		if element == id {
			return true
		}
	}
	return false
}

func indexIncluded(index uint64, indices []uint64) bool {
	if indices == nil {
		return true
	}
	for _, element := range indices {
		if element == index {
			return true
		}
	}
	return false
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
)

func newBlockIdError(input string) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("Invalid block ID: %s", input),
	}
}

func newIndicesError(input string) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("Invalid index input: %s", input),
	}
}

func newOutOfRangeError(input uint64, blobCount int) *httpError {
	return &httpError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("Invalid index: %d - block contains %d blobs", input, blobCount),
	}
}
