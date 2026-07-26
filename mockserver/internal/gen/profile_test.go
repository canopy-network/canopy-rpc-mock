// profile_test.go
package gen

import "testing"

func TestProfileForChainRootIsBusy(t *testing.T) {
	if ProfileForChain(1) != ProfileBusy {
		t.Fatalf("expected chain 1 to be busy")
	}
}

func TestProfileForChainOtherIsQuiet(t *testing.T) {
	for _, id := range []uint64{2, 100, 101, 999} {
		if ProfileForChain(id) != ProfileQuiet {
			t.Fatalf("expected chain %d to be quiet", id)
		}
	}
}
