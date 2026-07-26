// distributions.go
package main

import (
	"encoding/binary"
	"math"
	"math/rand"
)

// heightSeed derives a deterministic uint64 seed from a base seed and a height,
// reusing the existing hashBytes() helper (mock_data.go) for consistency with
// blockHash()/hashBytes()-based determinism elsewhere in this package.
func heightSeed(baseSeed, height uint64) uint64 {
	sum := hashBytes(baseSeed, height)
	return binary.BigEndian.Uint64(sum[:8])
}

// addrHeightSeed derives a deterministic seed from an address and a height.
func addrHeightSeed(addr []byte, height uint64) uint64 {
	sum := hashBytes(addr, height)
	return binary.BigEndian.Uint64(sum[:8])
}

func rngForHeight(baseSeed, height uint64) *rand.Rand {
	return rand.New(rand.NewSource(int64(heightSeed(baseSeed, height))))
}

func rngForAddrHeight(addr []byte, height uint64) *rand.Rand {
	return rand.New(rand.NewSource(int64(addrHeightSeed(addr, height))))
}

// standardNormal draws one N(0,1) sample via Box-Muller.
func standardNormal(rng *rand.Rand) float64 {
	u1, u2 := rng.Float64(), rng.Float64()
	if u1 < 1e-12 {
		u1 = 1e-12
	}
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}
