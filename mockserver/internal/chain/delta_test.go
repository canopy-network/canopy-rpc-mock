package chain

import (
	"testing"

	"github.com/canopy-network/canopy/fsm"
)

// TestDeltaSparsificationOnlyKeepsChangedAccounts sweeps a range of heights
// (not just one cherry-picked height) instead of hardcoding height 20, which
// used to pass "by luck" (2/10 unchanged at that one height) while ~25% of
// heights would have failed an "after < before" check if swept (see Fix 2 in
// the final review).
//
// The test's test chain only has 10 accounts (one per hardcoded validator
// seed), so at that population size a single active account's noise
// (addrActiveAtHeight gates ~2% of addresses per height, sigma=0.15) can
// still shift the renormalization sum enough to flip several other
// accounts' rounded uint64 balances — a small-N artifact that wouldn't
// happen on a real chain with hundreds/thousands of accounts. That means a
// strict per-height "did at least one account get trimmed" check is noisy at
// n=10 and doesn't reliably distinguish fixed-vs-broken behavior here.
// Instead, this asserts on the AGGREGATE fraction of accounts trimmed across
// the whole sweep, which is the metric the review itself used to characterize
// the bug ("unchanged-account counts range 0-30% ... not the intended
// ~98%"): pre-fix (TotalSupplyAt changing every block) this ran ~12% trimmed
// on this test chain; post-fix (TotalSupplyAt quantized to a stable 100-block
// window) it runs ~55-65%. 50% is comfortably below the fixed behavior and
// comfortably above the broken one, so it can't pass by luck.
func TestDeltaSparsificationOnlyKeepsChangedAccounts(t *testing.T) {
	mc := newMockChain(30, 1)

	const startHeight, endHeight = 10, 25
	var totalAccounts, trimmedAccounts int
	for h := uint64(startHeight); h <= endHeight; h++ {
		blobs, err := mc.BuildIndexerBlobs(h)
		if err != nil {
			t.Fatalf("BuildIndexerBlobs(%d) failed: %v", h, err)
		}
		before := len(blobs.Current.Accounts)

		delta, err := fsm.DeltaIndexerBlobs(blobs)
		if err != nil {
			t.Fatalf("DeltaIndexerBlobs(%d) failed: %v", h, err)
		}
		after := len(delta.Current.Accounts)

		totalAccounts += before
		trimmedAccounts += before - after
		if !delta.Current.ValidatorsDelta {
			t.Fatalf("height %d: expected ValidatorsDelta=true on delta-trimmed current blob", h)
		}
	}

	const minTrimFraction = 0.5
	trimFraction := float64(trimmedAccounts) / float64(totalAccounts)
	if trimFraction < minTrimFraction {
		t.Fatalf("expected delta-trimming to remove at least %.0f%% of accounts across heights %d-%d, got %d/%d (%.0f%%)",
			minTrimFraction*100, startHeight, endHeight, trimmedAccounts, totalAccounts, trimFraction*100)
	}
}
