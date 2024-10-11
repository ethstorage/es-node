// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

//go:build !ci

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ethstorage/go-ethstorage/ethstorage/archiver"
)

const (
	urlPattern   = "http://%s/eth/v1/beacon/blob_sidecars/%s"
	archiverAddr = "65.108.236.27:9645"
	beaconAddr   = "88.99.30.186:3500"
)

type test struct {
	query           string
	archivedIndices []uint64
	httpCode        int
	msg             string
}

func (t *test) toUrl() string {
	if len(t.archivedIndices) == 0 {
		return fmt.Sprintf(urlPattern, archiverAddr, t.query)
	}
	var strArr []string
	for _, val := range t.archivedIndices {
		strArr = append(strArr, strconv.FormatUint(uint64(val), 10))
	}
	query := fmt.Sprintf("%s?indices=%s", t.query, strings.Join(strArr, ","))
	return fmt.Sprintf(urlPattern, archiverAddr, query)
}

func TestArchiveAPI(t *testing.T) {

	tests := []test{
		{
			query:           "4756895",
			archivedIndices: []uint64{3},
		},
		{
			query:           "4756895?indices=3",
			archivedIndices: []uint64{3},
		},
		{
			query:           "4756895?indices=0,3",
			archivedIndices: []uint64{3},
		},
		{
			query:    "4756895?indices=1",
			httpCode: 404,
			msg:      "Blob not found in EthStorage",
		},
		{
			query: "current",
		},
		{
			query: "8626176",
		},
		{
			query: "4756895?indices=9",
		},
	}

	for i, tt := range tests {
		t.Logf("=== test %d =====\n", i)
		urla := fmt.Sprintf(urlPattern, archiverAddr, tt.query)
		sidecarsa, codea, msga, err := makeQuery(t, urla)
		if err != nil {
			t.Fatalf("Failed to query URL %s: %v", urla, err)
		}
		urlb := fmt.Sprintf(urlPattern, beaconAddr, tt.query)
		sidecarsb, codeb, msgb, err := makeQuery(t, urlb)
		if err != nil {
			t.Fatalf("Failed to query URL %s: %v", urlb, err)
		}
		if codea != tt.httpCode && codea != codeb {
			t.Errorf("Expected HTTP code %d, got %d", codeb, codea)
		}
		if msga != tt.msg && msga != msgb {
			t.Errorf("Expected message %s, got %s", msgb, msga)
		}
		if codea != 200 || tt.archivedIndices == nil {
			continue
		}
		if len(sidecarsa) != len(tt.archivedIndices) {
			t.Errorf("Expected %d blobs, got %d", len(tt.archivedIndices), len(sidecarsa))
			continue
		}
		for _, index := range tt.archivedIndices {
			sidecara, ok := sidecarsa[index]
			if !ok {
				t.Errorf("Expected blob with index %d from archiver, got none", index)
				continue
			}
			sidecarb, ok := sidecarsb[index]
			if !ok {
				t.Errorf("Expected blob with index %d from beacon, got none", index)
				continue
			}
			if sidecara.Index != sidecarb.Index {
				t.Errorf("Index does not match: %d != %d", sidecara.Index, sidecarb.Index)
			}
			if sidecara.KZGCommitment != sidecarb.KZGCommitment {
				t.Errorf("KZGCommitment does not match: %x != %x", sidecara.KZGCommitment, sidecarb.KZGCommitment)
			}
			if sidecara.KZGProof != sidecarb.KZGProof {
				t.Errorf("KZGProof does not match: %x != %x", sidecara.KZGProof, sidecarb.KZGProof)
			}
			if sidecara.Blob != sidecarb.Blob {
				t.Errorf("Blob does not match: %x != %x", sidecara.Blob[:20], sidecarb.Blob[:20])
			}
			t.Logf("Test passed with query %s and index %d", tt.query, index)
		}
	}
}

func makeQuery(t *testing.T, url string) (map[uint64]*archiver.BlobSidecar, int, string, error) {
	t.Logf("Querying %s", url)
	client := http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		t.Errorf("Failed to query URL %s: %v", url, err)
		return nil, 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed to read response body from URL %s: %v", url, err)
		return nil, 0, "", err
	}
	if resp.StatusCode != 200 {
		var res struct {
			code    int
			Message string
		}
		if err := json.Unmarshal(body, &res); err != nil {
			t.Errorf("Failed to unmarshal JSON response %s: %v", string(body), err)
			return nil, 0, "", err
		}
		t.Logf("-> %d %v", resp.StatusCode, res)
		msg := strings.Split(res.Message, ":")[0]
		return nil, resp.StatusCode, msg, nil
	}
	var res archiver.BlobSidecars
	if err := json.Unmarshal(body, &res); err != nil {
		t.Errorf("Failed to unmarshal JSON response from URL %s: %v", url, err)
		return nil, 0, "", err
	}
	// put blob sidecars into a map of indices to blobs
	indexToSidecars := make(map[uint64]*archiver.BlobSidecar)
	for _, blob := range res.Data {
		indexToSidecars[uint64(blob.Index)] = blob
		t.Logf("-> %d: %x", blob.Index, blob.Blob[:10])
	}
	t.Logf("-> blobs=%d", len(res.Data))

	return indexToSidecars, resp.StatusCode, "", nil
}
