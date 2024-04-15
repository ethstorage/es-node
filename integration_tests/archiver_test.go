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
	beaconAddr   = "88.99.30.186:3500"
	archiverAddr = "65.108.236.27:9645"
)

func TestQueryUrls(t *testing.T) {
	tests := []string{
		"4756895?indices=1",
	}

	for i, _ := range tests {
		url := fmt.Sprintf(urlPattern, beaconAddr, tests[i])
		res, err := makeQuery(t, url)
		if err != nil {
			t.Fatalf("Failed to query URL %s: %v", url, err)
		}
		for _, d := range res.Data {
			t.Logf("%+v\n", d)
		}

		t.Logf("Successfully queried and verified data from URL: %s\n", url)
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
