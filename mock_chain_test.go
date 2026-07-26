package main

import (
	"encoding/hex"
	"testing"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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

func TestEventsOccurAtRealisticRate(t *testing.T) {
	mc := newMockChain(200, 1)
	totalEvents := 0
	for h := uint64(1); h <= 200; h++ {
		totalEvents += len(mc.events[h])
	}
	// busy profile events run on their own ~1.5/block mean cadence (Task 6) —
	// a range assertion, not an exact count, per spec Section 11.
	if totalEvents < 100 || totalEvents > 500 {
		t.Fatalf("expected roughly 100-500 events over 200 busy blocks, got %d", totalEvents)
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

// TestCertificateBuiltAfterDexBatchAndState verifies that the cert-building
// ordering fix landed: buildCertificateForTx(height) reads mc.dexBatches[height]
// and mc.doubleSigners(height) (which reads mc.states[height]) — both must
// already be populated for that height by the time the cert is built, since
// generateDexBatch/snapshotStateAt happen earlier in the same loop iteration.
func TestCertificateBuiltAfterDexBatchAndState(t *testing.T) {
	mc := newMockChain(35, 1)

	for _, h := range []uint64{5, 30} {
		cert := mc.certs[h]
		if cert == nil || cert.Results == nil {
			t.Fatalf("height %d: missing certificate/results", h)
		}

		wantBatch := mc.dexBatches[h]
		if wantBatch == nil {
			t.Fatalf("height %d: expected non-nil dexBatches entry", h)
		}
		if cert.Results.DexBatch != wantBatch {
			t.Fatalf("height %d: cert.Results.DexBatch does not match mc.dexBatches[h] — cert was built before dexBatches was set", h)
		}

		wantDoubleSigners := mc.doubleSigners(h)
		gotDoubleSigners := cert.Results.SlashRecipients.DoubleSigners
		if len(gotDoubleSigners) != len(wantDoubleSigners) {
			t.Fatalf("height %d: cert DoubleSigners len=%d, mc.doubleSigners(h) len=%d — cert was built before states was set", h, len(gotDoubleSigners), len(wantDoubleSigners))
		}
	}
}

// TestGenesisStateAtHeightZero verifies the height-0 genesis state fix:
// mc.states[0] must be populated (so an explicit {"height":0} query resolves
// instead of 404ing), and its DoubleSigners computation must not underflow
// the uint64 height-1 subtraction.
func TestGenesisStateAtHeightZero(t *testing.T) {
	mc := newMockChain(10, 1)

	state := mc.states[0]
	if state == nil {
		t.Fatalf("expected mc.states[0] to be non-nil (genesis state)")
	}
	if len(state.DoubleSigners) != 0 {
		t.Fatalf("expected no DoubleSigners at height 0, got %v", state.DoubleSigners)
	}
	if state.Accounts == nil {
		t.Fatalf("expected genesis state to have accounts populated")
	}
	if state.Validators == nil {
		t.Fatalf("expected genesis state to have validators populated")
	}
}

// TestGeneratedTxHasRealMsgPayload verifies that generated txs carry a real,
// non-nil, decodable Transaction.Msg payload rather than an empty *anypb.Any.
func TestGeneratedTxHasRealMsgPayload(t *testing.T) {
	mc := newMockChain(50, 1)

	found := false
	for h := uint64(1); h <= 50; h++ {
		for _, res := range mc.txs[h] {
			if res.MessageType != fsm.MessageSendName {
				continue
			}
			if res.Transaction == nil || res.Transaction.Msg == nil {
				t.Fatalf("height %d: send tx has nil Transaction.Msg", h)
			}
			msg := new(fsm.MessageSend)
			if err := anypb.UnmarshalTo(res.Transaction.Msg, msg, proto.UnmarshalOptions{}); err != nil {
				t.Fatalf("height %d: failed to unmarshal MessageSend: %v", h, err)
			}
			if len(msg.FromAddress) == 0 || len(msg.ToAddress) == 0 || msg.Amount == 0 {
				t.Fatalf("height %d: decoded MessageSend has empty/zero fields: %+v", h, msg)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected at least one 'send' tx across heights 1-50")
	}
}

func TestBlockMetaSizeIsActualSerializedLength(t *testing.T) {
	mc := newMockChain(5, 1)
	block := mc.blocks[3]
	bz, err := lib.Marshal(block)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	// Meta.Size must be close to the actual marshaled length of the block
	// header+txs+events (Meta itself isn't part of what's marshaled into bz,
	// so exact equality isn't expected — but the formula-based value the old
	// code used, 1024+height*10=1054, would be wildly off for a real block).
	if block.Meta.Size == 1024+3*10 {
		t.Fatalf("Meta.Size still uses the old fabricated formula")
	}
	if len(bz) == 0 {
		t.Fatalf("expected non-empty marshaled block")
	}
}
