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
)

func TotalSupplyAt(chainID uint64, height uint64) uint64 {
	scale := 1.0
	if chainID != 1 {
		scale = 0.05 // appchains have far fewer accounts/smaller supply (spec table)
	}
	return uint64(baseSupply * scale * (1 + inflationRate*float64(height)))
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

// exchangeRate is the FIXED, height-independent raw-value-to-balance
// conversion rate: TotalSupplyAt at genesis (height 0) divided by the sum of
// every address's height-independent baseValue. It is deliberately NOT
// recomputed from a live per-height sum of raw values — that was the root
// cause of a bug where a single active account's noise moved every other
// account's normalized balance too (renormalizing against a moving,
// all-accounts sum couples every account to every other account's activity
// that block, regardless of population size). Computing the rate once from
// baseValue decouples them: an inactive account's balance now depends only
// on its own (unchanged) raw value and this fixed rate, so it is genuinely
// byte-identical across heights it wasn't touched in — at any account count,
// not just small test populations. See SnapshotBalancesAt for how supply
// growth/drift is reconciled without moving every other account's balance.
func exchangeRate(addrs [][]byte, chainID uint64) float64 {
	var sum float64
	for _, a := range addrs {
		sum += baseValue(a)
	}
	if sum == 0 {
		return 0
	}
	return float64(TotalSupplyAt(chainID, 0)) / sum
}

// residualIndex picks the LARGEST-baseValue address as the residual/treasury
// slot (deterministic, height-independent — computed purely from baseValue).
// Removing the biggest holder from the "priced independently" set, rather
// than an arbitrary position, maximizes the slack the residual has to absorb
// other accounts' noise: at small account counts the balance distribution is
// heavy-tailed enough that a single active account's noise can otherwise
// swing the priced sum past TotalSupplyAt (observed: one whale's noise pushed
// the sum ~40% over target when the residual was picked by position instead
// of by size).
func residualIndex(addrs [][]byte) int {
	best, bestVal := 0, -1.0
	for i, a := range addrs {
		if v := baseValue(a); v > bestVal {
			best, bestVal = i, v
		}
	}
	return best
}

// SnapshotBalancesAt computes every address's balance at height in one pass —
// O(numAccounts), independent of H, per spec Section 3.
//
// Every address except the residual (residualIndex) is priced at the fixed
// exchangeRate against its own rawValue, independent of every other
// address's activity that block — an inactive address's balance is therefore
// byte-identical to the previous height's, which is what gives
// fsm.DeltaIndexerBlobs something to trim (see delta_test.go). The residual
// address's balance is whatever's left of TotalSupplyAt after the others are
// priced, so conservation (sum == TotalSupplyAt) holds exactly by
// construction rather than by rounding-tolerant renormalization. It absorbs
// all inflation growth and all per-account noise drift, so it is expected to
// look unlike a "real" account and to change most heights — that trade-off
// is what makes every OTHER account's sparsification real.
//
// Fallback: on the rare height where the OTHER accounts' combined noise still
// pushes their priced sum above TotalSupplyAt (despite the residual being the
// largest holder), every non-residual balance is scaled down proportionally
// so the total still lands on TotalSupplyAt exactly — conservation is never
// allowed to break, even though this one height briefly re-couples every
// account to every other's activity, same as the design this replaced.
func SnapshotBalancesAt(addrs [][]byte, chainID uint64, height uint64) []uint64 {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]uint64, len(addrs))
	if len(addrs) == 1 {
		// single-address chain: nothing else to price, the one address just
		// gets the full supply.
		out[0] = TotalSupplyAt(chainID, height)
		return out
	}
	residual := residualIndex(addrs)
	rate := exchangeRate(addrs, chainID)
	raws := make([]float64, len(addrs))
	var sumOthers float64
	for i, a := range addrs {
		if i == residual {
			continue
		}
		raws[i] = rawValue(a, height) * rate
		sumOthers += raws[i]
	}
	total := float64(TotalSupplyAt(chainID, height))
	if sumOthers > total {
		// Extremely rare: scale every non-residual balance down
		// proportionally so conservation still holds exactly this height.
		scale := total / sumOthers
		for i := range addrs {
			if i != residual {
				out[i] = uint64(raws[i] * scale)
			}
		}
		return out
	}
	for i := range addrs {
		if i != residual {
			out[i] = uint64(raws[i])
		}
	}
	var sum uint64
	for i, v := range out {
		if i != residual {
			sum += v
		}
	}
	out[residual] = TotalSupplyAt(chainID, height) - sum
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
