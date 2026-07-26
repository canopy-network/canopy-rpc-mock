// lifecycle_test.go
package main

import "testing"

func TestUnstakeHeightIsDeterministicAndFuture(t *testing.T) {
	addr := hashBytes("validator", 3)
	h1 := unstakeHeightFor(addr, 1000)
	h2 := unstakeHeightFor(addr, 1000)
	if h1 != h2 {
		t.Fatalf("expected deterministic unstake height, got %d vs %d", h1, h2)
	}
	if h1 <= 1000 {
		t.Fatalf("expected unstake height scheduled after genesis window, got %d", h1)
	}
}

func TestValidatorStatusAtTransitionsAtScheduledHeight(t *testing.T) {
	addr := hashBytes("validator", 3)
	unstakeAt := unstakeHeightFor(addr, 1000)
	if validatorStatusAt(addr, 1000, unstakeAt-1) != validatorActive {
		t.Fatalf("expected active before unstake height")
	}
	if validatorStatusAt(addr, 1000, unstakeAt) != validatorUnstaked {
		t.Fatalf("expected unstaked at/after unstake height")
	}
}
