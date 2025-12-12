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

func TestDelayedDexEvents(t *testing.T) {
	mc := newMockChain(25, 1)
	if len(mc.events[22]) == 0 {
		t.Fatalf("expected dex events at height 22 after batch processing")
	}
}
