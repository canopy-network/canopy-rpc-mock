// txgen.go
package main

// txCountAt returns the deterministic tx count for (chainID, height), drawn
// from a per-height hash-seeded negative binomial (busy) with diurnal mean
// modulation, or a zero-inflated small-mean distribution (quiet).
func txCountAt(chainID uint64, profile chainProfile, height uint64) int {
	params := paramsForProfile(profile)
	rng := rngForHeight(chainID, height)
	if profile == profileQuiet {
		if rng.Float64() < params.zeroTxChance {
			return 0
		}
		return 1 + samplePoisson(rng, params.txCountMean)
	}
	mean := busyMeanAt(height, params.txCountMean)
	count := sampleNegBinomial(rng, params.txCountR, mean)
	if count == 0 {
		count = 1 // busy profile is never zero (spec Section 4)
	}
	return count
}

// txTypeMixAt draws one tx type label for a single tx slot at (chainID, height, index),
// weighted per profile (spec Section 4). Callers building N txs for a block must
// pass a distinct index per tx (e.g. the tx's position in the block) so repeated
// calls at the same height don't all draw identically.
func txTypeMixAt(chainID uint64, profile chainProfile, height uint64) string {
	params := paramsForProfile(profile)
	rng := rngForHeight(chainID, height)
	return sampleCategorical(rng, params.txTypeWeights)
}
