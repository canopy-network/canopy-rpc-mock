// eventgen_test.go
package main

import "testing"

func TestEventTypeMixAtBusyIsRewardLed(t *testing.T) {
	counts := map[string]int{}
	for h := uint64(1); h <= 5000; h++ {
		counts[eventTypeMixAt(1, profileBusy, h)]++
	}
	if counts["reward"] <= counts["dexSwap"] {
		t.Fatalf("expected busy profile reward-led, got reward=%d dexSwap=%d", counts["reward"], counts["dexSwap"])
	}
}

func TestEventTypeMixAtQuietIsDexSwapLed(t *testing.T) {
	counts := map[string]int{}
	for h := uint64(1); h <= 5000; h++ {
		counts[eventTypeMixAt(100, profileQuiet, h)]++
	}
	if counts["dexSwap"] <= counts["reward"] {
		t.Fatalf("expected quiet profile dex-swap-led, got reward=%d dexSwap=%d", counts["reward"], counts["dexSwap"])
	}
}

func TestEventCountAtMatchesProfileMean(t *testing.T) {
	// Test busy profile mean (expected: 1.5, tolerance: ±20%)
	const wantBusyMean = 1.5
	const busyLowerBound = wantBusyMean * 0.8
	const busyUpperBound = wantBusyMean * 1.2

	busySum := 0
	const n = 5000
	for h := uint64(1); h <= n; h++ {
		busySum += eventCountAt(1, profileBusy, h)
	}
	busyGot := float64(busySum) / float64(n)
	if busyGot < busyLowerBound || busyGot > busyUpperBound {
		t.Fatalf("busy profile mean drifted: want ~%v, got %v (outside [%v, %v])",
			wantBusyMean, busyGot, busyLowerBound, busyUpperBound)
	}

	// Test quiet profile mean (expected: 0.6, tolerance: ±20%)
	const wantQuietMean = 0.6
	const quietLowerBound = wantQuietMean * 0.8
	const quietUpperBound = wantQuietMean * 1.2

	quietSum := 0
	for h := uint64(1); h <= n; h++ {
		quietSum += eventCountAt(100, profileQuiet, h)
	}
	quietGot := float64(quietSum) / float64(n)
	if quietGot < quietLowerBound || quietGot > quietUpperBound {
		t.Fatalf("quiet profile mean drifted: want ~%v, got %v (outside [%v, %v])",
			wantQuietMean, quietGot, quietLowerBound, quietUpperBound)
	}
}
