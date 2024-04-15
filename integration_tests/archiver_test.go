// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/archiver"
)

const (
	urlPattern   = "http://%s/eth/v1/beacon/blob_sidecars/%s"
	archiverAddr = "65.108.236.27:9645"
	beaconAddr   = "88.99.30.186:3500"
)

func TestArchiveAPI(t *testing.T) {

	tests := []struct {
		query  string
		blobsa int
		blobsb int
		err    error
	}{
		{
			query:  "4756895?indices=1",
			blobsa: 0,
			blobsb: 0,
			err:    nil,
		},
		{
			query:  "4756895?indices=2",
			blobsa: 0,
			blobsb: 1,
			err:    nil,
		},
	}

	for _, tt := range tests {
		urla := fmt.Sprintf(urlPattern, archiverAddr, tt.query)
		resa, err := makeQuery(t, urla)
		if err != nil {
			t.Fatalf("Failed to query URL %s: %v", urla, err)
		}
		urlb := fmt.Sprintf(urlPattern, beaconAddr, tt.query)
		resb, err := makeQuery(t, urlb)
		if err != nil {
			t.Fatalf("Failed to query URL %s: %v", urlb, err)
		}
		t.Log("resb", len(resb.Data))
		if len(resa.Data) != tt.blobsa {
			t.Errorf("Expected %d blobs, got %d", tt.blobsa, len(resa.Data))
			continue
		}
		blobs := tt.blobsb
		if blobs > tt.blobsa {
			blobs = tt.blobsa
		}
		for i := 0; i < blobs; i++ {
			if resa.Data[i].Index != resb.Data[i].Index {
				t.Errorf("Index does not match: %d != %d", resa.Data[i].Index, resb.Data[i].Index)
			}
			if resa.Data[i].KZGCommitment != resb.Data[i].KZGCommitment {
				t.Errorf("KZGCommitment does not match: %s != %s", resa.Data[i].KZGCommitment, resb.Data[i].KZGCommitment)
			}
			if resa.Data[i].KZGProof != resb.Data[i].KZGProof {
				t.Errorf("KZGProof does not match: %s != %s", resa.Data[i].KZGProof, resb.Data[i].KZGProof)
			}
			if resa.Data[i].Blob != resb.Data[i].Blob {
				t.Errorf("Blob does not match: %s != %s", resa.Data[i].Blob, resb.Data[i].Blob)
			}
		}
	}
}

func makeQuery(t *testing.T, url string) (*archiver.BlobSidecars, error) {
	client := http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		t.Errorf("Failed to query URL %s: %v", url, err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed to read response body from URL %s: %v", url, err)
		return nil, err
	}

	var res archiver.BlobSidecars
	if err := json.Unmarshal(body, &res); err != nil {
		t.Errorf("Failed to unmarshal JSON response from URL %s: %v", url, err)
		return nil, err
	}
	return &res, nil
}
