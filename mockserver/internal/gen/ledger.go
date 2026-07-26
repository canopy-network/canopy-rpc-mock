package gen

import "math"

// Log-normal parameters fit from the chain_1 balance percentiles in the spec
// (p25=20M, median=87.9M, p75=398M). p99=12B is a sanity check only (not
// solved for directly — fitLogNormal uses the more tail-robust IQR).
var balanceMu, balanceSigma = fitLogNormal(20_000_000, 87_900_000, 398_000_000)

// baseValue is the average per-account "raw" balance weight,
// independent of height — the actual per-height balance is this raw value
// renormalized against TotalSupplyAt() for conservation.
func baseValue(addr []byte) float64 {
	rng := RngForAddrHeight(addr, 0)
	return sampleLogNormal(rng, balanceMu, balanceSigma)
}

// rawValue is baseValue(addr) perturbed by per-height noise. drift is 0 by
// design (spec: modeling balance-distribution drift over time is out of
// scope until measured/confirmed as significant) — the shape stays constant
// across height, individual balances still vary height-to-height via noise.
// Noise is gated behind addrActiveAtHeight so most addresses are unchanged
// most heights, matching real ledger churn (most accounts don't transact
// every block) — this is what gives fsm.DeltaIndexerBlobs something to trim.
func rawValue(addr []byte, height uint64) float64 {
	if !addrActiveAtHeight(addr, height) {
		return baseValue(addr)
	}
	const sigma = 0.15
	rng := RngForAddrHeight(addr, height)
	noise := StandardNormal(rng)
	return baseValue(addr) * math.Exp(sigma*noise)
}

// addrActiveAtHeight gives each address a small deterministic per-height
// chance of being "touched" this block — most addresses are unchanged most
// heights, matching real accounts_latest churn and making delta
// sparsification meaningful.
func addrActiveAtHeight(addr []byte, height uint64) bool {
	rng := RngForAddrHeight(addr, height)
	return rng.Float64() < 0.02
}

// TotalSupplyAt is a simple closed-form monotonic growth curve — deterministic,
// O(1), no replay. inflationRate is a placeholder magnitude (not measured);
// retune if the ledger-invariant test's supply-conservation check needs it.
const (
	baseSupply    = 1_400_000_000.0 // ~sum of chain_1's measured account total + staked, spec Section "Real-world grounding"
	inflationRate = 0.0000005       // per-block growth, tiny and monotonic

	// supplyStepBlocks quantizes the height used for the inflation calculation
	// so TotalSupplyAt (and therefore SnapshotBalancesAt's renormalization
	// divisor) is IDENTICAL across a whole window of consecutive heights,
	// rather than changing every single block. SnapshotBalancesAt normalizes
	// every balance as total*raw_i/sum, so if `total` moves every height, an
	// inactive account's normalized balance moves every height too — even
	// though addrActiveAtHeight only perturbs the noise on ~2% of addresses.
	// Quantizing the height fixes `total` (and thus raw_i/sum's scale) for
	// supplyStepBlocks consecutive heights, so an untouched account's
	// normalized balance is now genuinely byte-identical across that window,
	// which is what makes delta-sparsification (Task 18) actually trim
	// anything. Supply still grows monotonically overall, just in steps.
	supplyStepBlocks = 100
)

func TotalSupplyAt(chainID uint64, height uint64) uint64 {
	scale := 1.0
	if chainID != 1 {
		scale = 0.05 // appchains have far fewer accounts/smaller supply (spec table)
	}
	quantizedHeight := (height / supplyStepBlocks) * supplyStepBlocks
	return uint64(baseSupply * scale * (1 + inflationRate*float64(quantizedHeight)))
}

// BalanceAt normalizes addr's rawValue against the full address set's raw
// values so per-height balances always sum to TotalSupplyAt (conservation by
// construction, no ledger replay). Cost is O(len(addrs)) per call — callers
// generating a whole snapshot should call rawValue once per addr and do the
// division themselves rather than calling BalanceAt in a loop (see
// SnapshotBalancesAt below for the O(n) batch form).
func BalanceAt(addr []byte, addrs [][]byte, chainID uint64, height uint64) uint64 {
	balances := SnapshotBalancesAt(addrs, chainID, height)
	for i, a := range addrs {
		if string(a) == string(addr) {
			return balances[i]
		}
	}
	return 0
}

// SnapshotBalancesAt computes every address's balance at height in one pass —
// O(numAccounts), independent of H, per spec Section 3.
func SnapshotBalancesAt(addrs [][]byte, chainID uint64, height uint64) []uint64 {
	raws := make([]float64, len(addrs))
	sum := 0.0
	for i, a := range addrs {
		raws[i] = rawValue(a, height)
		sum += raws[i]
	}
	total := float64(TotalSupplyAt(chainID, height))
	out := make([]uint64, len(addrs))
	if sum == 0 {
		return out
	}
	for i, r := range raws {
		out[i] = uint64(total * r / sum)
	}
	return out
}

// StakeAt mirrors rawValue/BalanceAt but scoped to the validator set, using a
// separate log-normal fit — stakes and balances are different distributions.
//
// The original p75 anchor here (1_510_000_000) was ~1622x the median
// (931_000) — almost certainly a data-entry error where a total-staked or
// max-stake figure got used where a real p75 percentile was needed. That
// blew sigma out to ~5.94 (vs. the balance fit's sane ~2.22), which in
// practice meant a single validator could hold ~25% of Supply.Total among
// just 10 validators, and risked uint64-overflow-adjacent sums at `load`
// scale (5000 validators).
//
// With no better staging data available for stake percentiles, this is a
// deliberate recalibration (not a redesign): pick a p75 anchor that's a
// plausible multiple of the median (10x, vs. the balance fit's ~4.5x
// p75/median ratio of 398_000_000/87_900_000) — this keeps p25 unchanged
// and lands sigma at ~2.17, right in the balance fit's ~2.22 neighborhood,
// instead of the original's degenerate ~5.94.
var stakeMu, stakeSigma = fitLogNormal(500_000, 931_000, 931_000*10) // p75 ~10x median -> sigma ~2.17, in line with the balance fit's ~2.22

func StakeAt(addr []byte, chainID uint64, height uint64) uint64 {
	rng := RngForAddrHeight(append(append([]byte{}, addr...), byte(chainID)), height)
	const sigma = 0.1
	base := sampleLogNormal(RngForAddrHeight(addr, 0), stakeMu, stakeSigma)
	noise := StandardNormal(rng)
	return uint64(base * math.Exp(sigma*noise))
}
