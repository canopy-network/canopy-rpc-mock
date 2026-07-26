// lifecycle_test.go
package gen

import "testing"

func TestUnstakeHeightIsDeterministicAndFuture(t *testing.T) {
	addr := HashBytes("validator", 3)
	h1 := UnstakeHeightFor(addr, 1000)
	h2 := UnstakeHeightFor(addr, 1000)
	if h1 != h2 {
		t.Fatalf("expected deterministic unstake height, got %d vs %d", h1, h2)
	}
	if h1 <= 1000 {
		t.Fatalf("expected unstake height scheduled after genesis window, got %d", h1)
	}
}

func TestValidatorStatusAtTransitionsAtScheduledHeight(t *testing.T) {
	addr := HashBytes("validator", 3)
	unstakeAt := UnstakeHeightFor(addr, 1000)
	if ValidatorStatusAt(addr, 1000, unstakeAt-1) != ValidatorActive {
		t.Fatalf("expected active before unstake height")
	}
	if ValidatorStatusAt(addr, 1000, unstakeAt) != ValidatorUnstaked {
		t.Fatalf("expected unstaked at/after unstake height")
	}
}
