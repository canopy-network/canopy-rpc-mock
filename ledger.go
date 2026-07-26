package main

import "math"

// Log-normal parameters fit from the chain_1 balance percentiles in the spec
// (p25=20M, median=87.9M, p75=398M). p99=12B is a sanity check only (not
// solved for directly — fitLogNormal uses the more tail-robust IQR).
var balanceMu, balanceSigma = fitLogNormal(20_000_000, 87_900_000, 398_000_000)

// baseValue is the average per-account "raw" balance weight,
// independent of height — the actual per-height balance is this raw value
// renormalized against totalSupplyAt() for conservation.
func baseValue(addr []byte) float64 {
	rng := rngForAddrHeight(addr, 0)
	return sampleLogNormal(rng, balanceMu, balanceSigma)
}

// rawValue is baseValue(addr) perturbed by per-height noise. drift is 0 by
// design (spec: modeling balance-distribution drift over time is out of
// scope until measured/confirmed as significant) — the shape stays constant
// across height, individual balances still vary height-to-height via noise.
func rawValue(addr []byte, height uint64) float64 {
	const sigma = 0.15
	rng := rngForAddrHeight(addr, height)
	noise := standardNormal(rng)
	return baseValue(addr) * math.Exp(sigma*noise)
}

// totalSupplyAt is a simple closed-form monotonic growth curve — deterministic,
// O(1), no replay. inflationRate is a placeholder magnitude (not measured);
// retune if the ledger-invariant test's supply-conservation check needs it.
const (
	baseSupply    = 1_400_000_000.0 // ~sum of chain_1's measured account total + staked, spec Section "Real-world grounding"
	inflationRate = 0.0000005       // per-block growth, tiny and monotonic
)

func totalSupplyAt(chainID uint64, height uint64) uint64 {
	scale := 1.0
	if chainID != 1 {
		scale = 0.05 // appchains have far fewer accounts/smaller supply (spec table)
	}
	return uint64(baseSupply * scale * (1 + inflationRate*float64(height)))
}

// balanceAt normalizes addr's rawValue against the full address set's raw
// values so per-height balances always sum to totalSupplyAt (conservation by
// construction, no ledger replay). Cost is O(len(addrs)) per call — callers
// generating a whole snapshot should call rawValue once per addr and do the
// division themselves rather than calling balanceAt in a loop (see
// snapshotBalancesAt below for the O(n) batch form).
func balanceAt(addr []byte, addrs [][]byte, chainID uint64, height uint64) uint64 {
	balances := snapshotBalancesAt(addrs, chainID, height)
	for i, a := range addrs {
		if string(a) == string(addr) {
			return balances[i]
		}
	}
	return 0
}

// snapshotBalancesAt computes every address's balance at height in one pass —
// O(numAccounts), independent of H, per spec Section 3.
func snapshotBalancesAt(addrs [][]byte, chainID uint64, height uint64) []uint64 {
	raws := make([]float64, len(addrs))
	sum := 0.0
	for i, a := range addrs {
		raws[i] = rawValue(a, height)
		sum += raws[i]
	}
	total := float64(totalSupplyAt(chainID, height))
	out := make([]uint64, len(addrs))
	if sum == 0 {
		return out
	}
	for i, r := range raws {
		out[i] = uint64(total * r / sum)
	}
	return out
}

// stakeAt mirrors rawValue/balanceAt but scoped to the validator set, using a
// separate log-normal fit — stakes and balances are different distributions.
var stakeMu, stakeSigma = fitLogNormal(500_000, 931_000, 1_510_000_000) // chain_100/101 mean stake as rough anchors

func stakeAt(addr []byte, chainID uint64, height uint64) uint64 {
	rng := rngForAddrHeight(append(append([]byte{}, addr...), byte(chainID)), height)
	const sigma = 0.1
	base := sampleLogNormal(rngForAddrHeight(addr, 0), stakeMu, stakeSigma)
	noise := standardNormal(rng)
	return uint64(base * math.Exp(sigma*noise))
}
