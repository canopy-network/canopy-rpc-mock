// txgen_test.go
package main

import "testing"

func TestTxCountAtIsDeterministic(t *testing.T) {
	a := txCountAt(1, profileBusy, 12345)
	b := txCountAt(1, profileBusy, 12345)
	if a != b {
		t.Fatalf("expected deterministic tx count, got %d vs %d", a, b)
	}
}

func TestTxCountAtBusyNeverZero(t *testing.T) {
	for h := uint64(1); h <= 500; h++ {
		if txCountAt(1, profileBusy, h) == 0 {
			t.Fatalf("busy profile produced zero txs at height %d", h)
		}
	}
}

func TestTxCountAtQuietOftenZero(t *testing.T) {
	zeros := 0
	const n = 5000
	for h := uint64(1); h <= n; h++ {
		if txCountAt(100, profileQuiet, h) == 0 {
			zeros++
		}
	}
	pct := float64(zeros) / float64(n)
	if pct < 0.44 || pct > 0.64 {
		t.Fatalf("expected ~54%% zero-tx blocks, got %.2f%%", pct*100)
	}
}

func TestTxTypeMixAtIsDeterministic(t *testing.T) {
	a := txTypeMixAt(1, profileBusy, 42)
	b := txTypeMixAt(1, profileBusy, 42)
	if a != b {
		t.Fatalf("expected deterministic tx type, got %q vs %q", a, b)
	}
}
