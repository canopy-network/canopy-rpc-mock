// eventgen.go
package main

// eventWeights holds the per-profile event-type mix (spec Section 4). Events
// run on their own cadence, decoupled from tx volume (key finding 1) — busy
// is reward-led, quiet is dex-swap-led; the two profiles are NOT sharing one
// weighting table.
var busyEventWeights = []weightedOption{
	{"reward", 53.8},
	{"dexSwap", 44.0},
	{"dexLiquidityDeposit", 2.1},
	{"other", 0.1},
}

var quietEventWeights = []weightedOption{
	{"dexSwap", 60.0},
	{"reward", 37.0},
	{"dexLiquidityDeposit", 3.0},
}

func eventWeightsForProfile(p chainProfile) []weightedOption {
	if p == profileBusy {
		return busyEventWeights
	}
	return quietEventWeights
}

// eventTypeMixAt draws one event type for (chainID, height), independent of
// that height's tx count/type (key finding 1 in the spec: dex-swap/reward
// events run on their own protocol cadence, not "N events per tx").
func eventTypeMixAt(chainID uint64, profile chainProfile, height uint64) string {
	// distinct seed component ("event") so this doesn't correlate with
	// txCountAt/txTypeMixAt, which seed from the same (chainID, height) pair.
	rng := rngForHeight(chainID^0x6576656e74, height) // xor tag == "event" ascii-ish salt
	return sampleCategorical(rng, eventWeightsForProfile(profile))
}

// eventCountAt returns how many events fire at this height, on their own
// per-height Poisson-ish cadence (not derived from txCountAt).
func eventCountAt(chainID uint64, profile chainProfile, height uint64) int {
	rng := rngForHeight(chainID^0x6576656e74, height)
	mean := 1.5
	if profile == profileQuiet {
		mean = 0.6
	}
	return samplePoisson(rng, mean)
}
