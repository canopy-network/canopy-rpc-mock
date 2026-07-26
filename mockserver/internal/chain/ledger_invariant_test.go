// ledger_invariant_test.go
package chain

import (
	"testing"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/gen"
)

func TestLedgerInvariantsHoldAcrossHeights(t *testing.T) {
	mc := newMockChain(500, 1)
	for h := uint64(1); h <= 500; h++ {
		state := mc.states[h]
		if state == nil {
			t.Fatalf("missing state snapshot at height %d", h)
		}
		var sumBalances uint64
		for _, a := range state.Accounts {
			if int64(a.Amount) < 0 {
				t.Fatalf("negative balance at height %d for %x", h, a.Address)
			}
			sumBalances += a.Amount
		}
		if state.Supply == nil {
			t.Fatalf("missing supply at height %d", h)
		}
		// conservation within small integer-rounding tolerance (accounts count)
		diff := int64(state.Supply.Total) - int64(sumBalances)
		if diff < -int64(len(state.Accounts)) || diff > int64(len(state.Accounts)) {
			t.Fatalf("supply not conserved at height %d: total=%d sum(balances)=%d", h, state.Supply.Total, sumBalances)
		}
		for _, v := range state.Validators {
			status := gen.ValidatorStatusAt(v.Address, 0, h)
			isUnstaked := v.UnstakingHeight != 0
			if (status == gen.ValidatorUnstaked) != isUnstaked {
				t.Fatalf("height %d validator %x: lifecycle status disagreement", h, v.Address)
			}
		}
	}
}
