package mockserver

import (
	"net/http"
	"testing"
)

func TestNewServerServesHeightRoute(t *testing.T) {
	srv := New(1, WithBlocks(5))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/query/height", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
