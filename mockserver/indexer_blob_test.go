package mockserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
)

func TestIndexerBlobsEndpointRoundTrips(t *testing.T) {
	srv := New(1, WithBlocks(20))
	defer srv.Close()

	reqBody, _ := json.Marshal(map[string]any{"height": 10, "delta": true})
	resp, err := http.Post(srv.URL+"/v1/query/indexer-blobs", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-protobuf" {
		t.Fatalf("expected application/x-protobuf, got %q", ct)
	}
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	var blobs fsm.IndexerBlobs
	if err := lib.Unmarshal(body, &blobs); err != nil {
		t.Fatalf("protobuf unmarshal failed: %v", err)
	}
	if blobs.Current == nil {
		t.Fatalf("expected Current blob to be populated")
	}
	if blobs.Previous == nil {
		t.Fatalf("expected Previous blob at height 10 (>2, so previous must exist)")
	}
}

func TestIndexerBlobsNoPreviousBelowHeightThree(t *testing.T) {
	srv := New(1, WithBlocks(20))
	defer srv.Close()

	reqBody, _ := json.Marshal(map[string]any{"height": 2})
	resp, err := http.Post(srv.URL+"/v1/query/indexer-blobs", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	var blobs fsm.IndexerBlobs
	if err := lib.Unmarshal(body, &blobs); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if blobs.Previous != nil {
		t.Fatalf("expected no Previous blob at query height 2 (client always queries height+1, so this pairs with committed block 1)")
	}
}
