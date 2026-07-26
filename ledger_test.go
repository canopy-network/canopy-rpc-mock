package main

import "testing"

func TestBalanceAtIsDeterministic(t *testing.T) {
	addrs := [][]byte{[]byte("addr-a"), []byte("addr-b")}
	a := balanceAt(addrs[0], addrs, 1, 100)
	b := balanceAt(addrs[0], addrs, 1, 100)
	if a != b {
		t.Fatalf("expected deterministic balance, got %d vs %d", a, b)
	}
}

func TestBalanceAtConservesSupply(t *testing.T) {
	addrs := make([][]byte, 50)
	for i := range addrs {
		addrs[i] = hashBytes("addr", i)
	}
	const height = 42
	var sum uint64
	for _, a := range addrs {
		sum += balanceAt(a, addrs, 1, height)
	}
	total := totalSupplyAt(1, height)
	// integer rounding across 50 accounts can drift by a few units, not a %
	if diff := int64(total) - int64(sum); diff < -int64(len(addrs)) || diff > int64(len(addrs)) {
		t.Fatalf("expected sum(balances) ~= totalSupply(%d), got sum=%d total=%d", total, sum, total)
	}
}

func TestBalanceAtNeverNegative(t *testing.T) {
	addrs := make([][]byte, 20)
	for i := range addrs {
		addrs[i] = hashBytes("addr", i)
	}
	for h := uint64(0); h < 200; h++ {
		for _, a := range addrs {
			if int64(balanceAt(a, addrs, 1, h)) < 0 {
				t.Fatalf("negative balance at height %d", h)
			}
		}
	}
}
