package eth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

func TestDAClientDownloadBlobRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := NewDAClient(server.URL).DownloadBlob(common.Hash{})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("DownloadBlob() error = %v, want HTTP status error", err)
	}
}

func TestDAClientDownloadBlobRejectsWrongSize(t *testing.T) {
	var blob kzg4844.Blob
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, len(blob)-1))
	}))
	t.Cleanup(server.Close)

	_, err := NewDAClient(server.URL).DownloadBlob(common.Hash{})
	if err == nil || !strings.Contains(err.Error(), "unexpected blob size") {
		t.Fatalf("DownloadBlob() error = %v, want size error", err)
	}
}
