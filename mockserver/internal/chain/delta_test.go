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
// The original fix attempt (quantizing TotalSupplyAt to a stable window)
// only worked at this test's small n=10 account population — it didn't fix
// the actual root cause, which was gen.SnapshotBalancesAt renormalizing
// every account's balance against a LIVE sum of all accounts' raw values.
// Any single active account moved that sum, and therefore every other
// account's balance too, regardless of how many accounts existed (verified:
// at n>=100 the unchanged fraction collapsed to ~1%, since with more
// accounts almost every block has some active account). The real fix
// (gen.SnapshotBalancesAt / gen.exchangeRate) prices every account except a
// designated residual/treasury slot at a FIXED, height-independent exchange
// rate, so an inactive account's balance no longer depends on any other
// account's activity that block, at any population size — verified directly
// at n=10/100/1000/5000, all landing in the 84-96% unchanged range.
//
// This asserts on the AGGREGATE fraction of accounts trimmed across the
// whole sweep (not a strict per-height check, which is noisy at n=10 — a
// single active account can still flip a couple of neighbors' rounded
// uint64 balances via the residual's leftover-supply calculation). 50% is
// comfortably below the ~12% the broken renormalization produced and
// comfortably below the ~84%+ the fixed design produces, so it can't pass by
// luck.
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
