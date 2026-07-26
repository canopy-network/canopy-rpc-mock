package main

type validatorStatus int

const (
	validatorActive validatorStatus = iota
	validatorUnstaked
)

// unstakeHeightFor schedules a validator's unstake transition at genesis —
// a fixed, deterministic future height, never derived from replaying
// stake/unstake transactions (spec Section 3). ~90% of validators never
// unstake within a realistic mock run window (unstakeHeight far in the
// future); the rest unstake at a spread of heights so lifecycle queries have
// something to observe.
func unstakeHeightFor(addr []byte, genesisHeight uint64) uint64 {
	rng := rngForAddrHeight(addr, 0)
	if rng.Float64() < 0.9 {
		return genesisHeight + 1_000_000_000 // effectively never, within any mock run
	}
	return genesisHeight + uint64(100+rng.Intn(9900)) // unstakes somewhere in the next 100-10000 blocks
}

func validatorStatusAt(addr []byte, genesisHeight, height uint64) validatorStatus {
	if height < unstakeHeightFor(addr, genesisHeight) {
		return validatorActive
	}
	return validatorUnstaked
}
