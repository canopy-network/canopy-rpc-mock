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
