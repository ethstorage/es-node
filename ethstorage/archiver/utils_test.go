// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package archiver

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func Test_parseIndices(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		expected    []uint64
		expectError *httpError
	}{
		{
			name:        "happy path with comma-separated indices",
			query:       "indices=0,1,2",
			expected:    []uint64{0, 1, 2},
			expectError: nil,
		},
		{
			name:        "happy path with repeated indices parameter",
			query:       "indices=0&indices=1&indices=2",
			expected:    []uint64{0, 1, 2},
			expectError: nil,
		},
		{
			name:        "happy path with duplicate indices within bound and other query parameters ignored",
			query:       "indices=1&indices=2&indices=1&indices=3&bar=bar",
			expected:    []uint64{1, 2, 3},
			expectError: nil,
		},
		{
			name:        "happy path with comma-separated indices within bound and other query parameters ignored",
			query:       "indices=1,2,3,3&bar=bar",
			expected:    []uint64{1, 2, 3},
			expectError: nil,
		},
		{
			name:        "invalid indices with comma-separated indices",
			query:       "indices=1,abc,2&bar=bar",
			expected:    nil,
			expectError: newBadRequestError("requested blob indices [abc] are invalid"),
		},
		{
			name:        "invalid indices with repeated indices",
			query:       "indices=0&indices=abc&indices=2",
			expected:    nil,
			expectError: newBadRequestError("requested blob indices [abc] are invalid"),
		},
		{
			name:        "out of bounds indices throws error",
			query:       "indices=2&indices=7",
			expected:    nil,
			expectError: newBadRequestError("requested blob indices [7] are invalid"),
		},
		{
			name:        "negative indices",
			query:       "indices=-1",
			expected:    nil,
			expectError: newBadRequestError("requested blob indices [-1] are invalid"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/eth/v1/beacon/blob_sidecars/6930317?"+tt.query, nil)

			got, err := parseIndices(req, 6)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseIndices() got = %v, want %v", got, tt.expected)
			}
			if !reflect.DeepEqual(err, tt.expectError) {
				t.Errorf("parseIndices() got err = %v, want %v", err, tt.expectError)
			}
		})
	}
}
