// distributions_test.go
package main

import "testing"

func TestRngForHeightIsDeterministic(t *testing.T) {
	r1 := rngForHeight(42, 100)
	r2 := rngForHeight(42, 100)
	v1, v2 := r1.Float64(), r2.Float64()
	if v1 != v2 {
		t.Fatalf("expected identical draws for same (seed,height), got %v vs %v", v1, v2)
	}
}

func TestRngForHeightVariesByHeight(t *testing.T) {
	r1 := rngForHeight(42, 100)
	r2 := rngForHeight(42, 101)
	if r1.Float64() == r2.Float64() {
		t.Fatalf("expected different draws for different heights")
	}
}
