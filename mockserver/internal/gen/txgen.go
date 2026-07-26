// txgen.go
package gen

// TxCountAt returns the deterministic tx count for (chainID, height), drawn
// from a per-height hash-seeded negative binomial (busy) with diurnal mean
// modulation, or a zero-inflated small-mean distribution (quiet).
func TxCountAt(chainID uint64, profile Profile, height uint64) int {
	params := ParamsForProfile(profile)
	rng := RngForHeight(chainID, height)
	if profile == ProfileQuiet {
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

// TxTypeMixAt draws one tx type label for a single tx slot at (chainID, height, index),
// weighted per profile (spec Section 4). Callers building N txs for a block must
// pass a distinct index per tx (e.g. the tx's position in the block) so repeated
// calls at the same height don't all draw identically.
func TxTypeMixAt(chainID uint64, profile Profile, height uint64) string {
	params := ParamsForProfile(profile)
	rng := RngForHeight(chainID, height)
	return sampleCategorical(rng, params.txTypeWeights)
}
