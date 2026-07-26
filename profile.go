// profile.go
package main

type chainProfile int

const (
	profileBusy chainProfile = iota
	profileQuiet
)

// profileForChain assigns busy/root to chain ID 1 and quiet/appchain to every
// other chain ID, per spec Section 2 — automatic by chain ID, no per-chain config.
func profileForChain(chainID uint64) chainProfile {
	if chainID == 1 {
		return profileBusy
	}
	return profileQuiet
}

// profileParams holds the per-profile tunable constants from spec Section 4,
// fit to the 24h staging aggregate (see docs/superpowers/specs/2026-07-26-realistic-mock-data-design.md).
type profileParams struct {
	// txCountR is the negative-binomial dispersion parameter. Chosen as a
	// starting point (not solved from measured variance — the spec table only
	// gives min/max/mean/median, not variance); the distribution-matching test
	// (Task 10) is the source of truth — retune if it fails tolerance.
	txCountR      float64
	txCountMean   float64
	zeroTxChance  float64 // quiet profile only: P(tx count == 0) before sampling
	txTypeWeights []weightedOption
}

var busyParams = profileParams{
	txCountR:     8,
	txCountMean:  91.69,
	zeroTxChance: 0, // busy profile is never zero (spec Section 4)
	txTypeWeights: []weightedOption{
		{"send", 75.3},
		{"dexLimitOrder", 10.5},
		{"editStake", 9.3},
		{"certResults", 3.3},
		{"dexLiqDeposit", 1.6},
		{"stake", 0.1},
		{"unstake", 0.1},
		{"dexLiqWithdraw", 0.1},
	},
}

var quietParams = profileParams{
	txCountR:     4,
	txCountMean:  0.8, // ~avg of chain_100 (0.80) and chain_101 (0.76)
	zeroTxChance: 0.54,
	txTypeWeights: []weightedOption{
		{"dexLiqDeposit", 64.6},
		{"editStake", 34.3},
		{"dexLimitOrder", 1.1},
	},
}

func paramsForProfile(p chainProfile) profileParams {
	if p == profileBusy {
		return busyParams
	}
	return quietParams
}
