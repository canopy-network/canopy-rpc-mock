package main

import (
	"testing"

	"github.com/canopy-network/canopy/lib"
)

func TestReproDefaultProfileByteIdentical(t *testing.T) {
	assertReproducible(t, runModeDefault)
}

func TestReproLoadProfileByteIdentical(t *testing.T) {
	assertReproducible(t, runModeLoad)
}

func assertReproducible(t *testing.T, mode runMode) {
	cfg := runModeConfig(mode)
	for _, chainID := range cfg.chainIDs {
		mc1 := newMockChain(20, chainID)
		mc2 := newMockChain(20, chainID)
		for h := uint64(1); h <= 20; h++ {
			b1, err1 := lib.Marshal(mc1.blocks[h])
			b2, err2 := lib.Marshal(mc2.blocks[h])
			if err1 != nil || err2 != nil {
				t.Fatalf("marshal error: %v / %v", err1, err2)
			}
			if string(b1) != string(b2) {
				t.Fatalf("chain %d height %d: block bytes differ between two generations", chainID, h)
			}
		}
	}
}
