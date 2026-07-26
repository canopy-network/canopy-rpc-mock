package main

import (
	"encoding/hex"
	"testing"
)

// sanity check that the mock chain prebuilds deterministic data for each height
func TestMockChainBuildsBlocks(t *testing.T) {
	mc := newMockChain(10, 1)

	for h := uint64(1); h <= 10; h++ {
		if mc.blocks[h] == nil {
			t.Fatalf("missing block at height %d", h)
		}
		if mc.certs[h] == nil {
			t.Fatalf("missing certificate at height %d", h)
		}
		if mc.states[h] == nil {
			t.Fatalf("missing state snapshot at height %d", h)
		}
		if mc.dexBatches[h] == nil || mc.nextDexBatches[h] == nil {
			t.Fatalf("missing dex batches at height %d", h)
		}
	}
}

func TestAccountsFromValidatorKeys(t *testing.T) {
	mc := newMockChain(5, 1)
	seen := make(map[string]struct{})
	if len(mc.accounts) != len(mc.validators) {
		t.Fatalf("expected %d accounts, got %d", len(mc.validators), len(mc.accounts))
	}
	for _, acc := range mc.accounts {
		key := hex.EncodeToString(acc.Address)
		seen[key] = struct{}{}
	}
	for _, v := range mc.validators {
		key := hex.EncodeToString(v.Address)
		if _, ok := seen[key]; !ok {
			t.Fatalf("missing validator address in accounts: %s", key)
		}
	}
}

func TestClosedFormTxCountMatchesGenerator(t *testing.T) {
	mc := newMockChain(50, 1)
	for h := uint64(1); h <= 50; h++ {
		want := txCountAt(mc.chainID, profileForChain(mc.chainID), h)
		got := len(mc.txs[h])
		if got != want {
			t.Fatalf("height %d: block has %d txs, generator says %d", h, got, want)
		}
	}
}

func TestDelayedDexEvents(t *testing.T) {
	mc := newMockChain(25, 1)
	if len(mc.events[22]) == 0 {
		t.Fatalf("expected dex events at height 22 after batch processing")
	}
}

// TestSnapshotStateParity ensures per-height state snapshots still carry
// OrderBooks/NonSigners/DoubleSigners/RetiredCommittees, matching the
// behavior of the old mutable mockState replay.
func TestSnapshotStateParity(t *testing.T) {
	mc := newMockChain(60, 1)
	if len(mc.validators) < 2 {
		t.Fatalf("test requires at least 2 validators, got %d", len(mc.validators))
	}

	foundOrderBooks := false
	foundNonSigners := false
	for h := uint64(1); h <= 60; h++ {
		state := mc.states[h]
		if state == nil {
			t.Fatalf("missing state snapshot at height %d", h)
		}

		if state.OrderBooks != nil {
			foundOrderBooks = true
			if state.OrderBooks != mc.orderBooks {
				t.Fatalf("height %d: state.OrderBooks does not reuse mc.orderBooks", h)
			}
		}

		if len(state.RetiredCommittees) != 1 || state.RetiredCommittees[0] != 42 {
			t.Fatalf("height %d: expected RetiredCommittees [42], got %v", h, state.RetiredCommittees)
		}

		if len(state.NonSigners) > 0 {
			foundNonSigners = true
		}

		if h%30 == 0 {
			if len(state.DoubleSigners) == 0 {
				t.Fatalf("height %d: expected non-empty DoubleSigners (h%%30==0)", h)
			}
			ds := state.DoubleSigners[0]
			if len(ds.Heights) != 2 || ds.Heights[0] != h-1 || ds.Heights[1] != h {
				t.Fatalf("height %d: unexpected DoubleSigner heights %v", h, ds.Heights)
			}
		} else if len(state.DoubleSigners) != 0 {
			t.Fatalf("height %d: expected no DoubleSigners (h%%30!=0), got %v", h, state.DoubleSigners)
		}
	}

	if !foundOrderBooks {
		t.Fatalf("expected at least one height with non-nil OrderBooks")
	}
	if !foundNonSigners {
		t.Fatalf("expected at least one height with populated NonSigners")
	}
}
