package chain

import (
	"testing"

	"github.com/canopy-network/canopy/fsm"
)

func TestDeltaSparsificationOnlyKeepsChangedAccounts(t *testing.T) {
	mc := newMockChain(30, 1)
	blobs, err := mc.BuildIndexerBlobs(20)
	if err != nil {
		t.Fatalf("BuildIndexerBlobs failed: %v", err)
	}
	before := len(blobs.Current.Accounts)

	delta, err := fsm.DeltaIndexerBlobs(blobs)
	if err != nil {
		t.Fatalf("DeltaIndexerBlobs failed: %v", err)
	}
	after := len(delta.Current.Accounts)
	if after >= before {
		t.Fatalf("expected delta to trim unchanged accounts: before=%d after=%d", before, after)
	}
	if !delta.Current.ValidatorsDelta {
		t.Fatalf("expected ValidatorsDelta=true on delta-trimmed current blob")
	}
}
