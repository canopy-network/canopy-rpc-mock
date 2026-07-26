// profile_test.go
package main

import "testing"

func TestProfileForChainRootIsBusy(t *testing.T) {
	if profileForChain(1) != profileBusy {
		t.Fatalf("expected chain 1 to be busy")
	}
}

func TestProfileForChainOtherIsQuiet(t *testing.T) {
	for _, id := range []uint64{2, 100, 101, 999} {
		if profileForChain(id) != profileQuiet {
			t.Fatalf("expected chain %d to be quiet", id)
		}
	}
}
