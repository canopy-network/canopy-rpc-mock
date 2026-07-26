// distribution_matching_test.go
package chain

import (
	"testing"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/gen"
)

// TestDistributionMatchesCalibratedTargets generates a full 24h-equivalent
// window (4320 blocks at 20s/block) and checks empirical stats land within
// tolerance of the Section 4-5 targets. This is the test that actually proves
// calibration works, not just that generation is deterministic. It's also the
// heaviest test in the suite (two 4320-height generations) — skipped under
// `-short` so canopy-indexer's routine `go test ./...` runs (which don't need
// mock-data calibration re-verified every time) stay fast; the full run
// still needs to happen in whatever CI job gates changes to this repo.
func TestDistributionMatchesCalibratedTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("distribution-matching is a full 24h-equivalent sweep — run without -short")
	}
	const heights = 4320
	mc := newMockChain(heights, 1) // chain 1 == busy profile

	var sum, zeroCount int
	typeCounts := map[string]int{}
	for h := uint64(1); h <= heights; h++ {
		n := len(mc.txs[h])
		sum += n
		if n == 0 {
			zeroCount++
		}
		for _, tx := range mc.txs[h] {
			typeCounts[tx.MessageType]++
		}
	}
	mean := float64(sum) / float64(heights)
	if mean < 91.69*0.85 || mean > 91.69*1.15 {
		t.Fatalf("busy tx/block mean out of tolerance: want ~91.69, got %v", mean)
	}
	if zeroCount != 0 {
		t.Fatalf("busy profile must never produce a zero-tx block, saw %d", zeroCount)
	}
	sendPct := float64(typeCounts["send"]) / float64(sum) * 100
	if sendPct < 75.3*0.8 || sendPct > 75.3*1.2 {
		t.Fatalf("send tx-type proportion out of tolerance: want ~75.3%%, got %.1f%%", sendPct)
	}

	// hour-bucketed mean check (chain_1 is non-stationary; appchains are not tested here)
	hourlyTotals := make([]int, 24)
	hourlyBlocks := make([]int, 24)
	for h := uint64(1); h <= heights; h++ {
		hour := gen.HourOfHeight(h)
		hourlyTotals[hour] += len(mc.txs[h])
		hourlyBlocks[hour]++
	}
	minHourMean, maxHourMean := 1e9, 0.0
	for i := 0; i < 24; i++ {
		if hourlyBlocks[i] == 0 {
			continue
		}
		m := float64(hourlyTotals[i]) / float64(hourlyBlocks[i])
		if m < minHourMean {
			minHourMean = m
		}
		if m > maxHourMean {
			maxHourMean = m
		}
	}
	if minHourMean > 60 || maxHourMean < 150 {
		t.Fatalf("expected hourly means spanning roughly 45-175 (spec 45.4-175.5), got min=%v max=%v", minHourMean, maxHourMean)
	}
}

func TestDistributionMatchesCalibratedTargetsQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("distribution-matching is a full 24h-equivalent sweep — run without -short")
	}
	const heights = 4320
	mc := newMockChain(heights, 100) // appchain == quiet profile

	zeroCount := 0
	typeCounts := map[string]int{}
	total := 0
	for h := uint64(1); h <= heights; h++ {
		n := len(mc.txs[h])
		total += n
		if n == 0 {
			zeroCount++
		}
		for _, tx := range mc.txs[h] {
			typeCounts[tx.MessageType]++
		}
	}
	zeroPct := float64(zeroCount) / float64(heights) * 100
	if zeroPct < 53.7*0.8 || zeroPct > 53.7*1.2 {
		t.Fatalf("quiet zero-tx-block pct out of tolerance: want ~53.7%%, got %.1f%%", zeroPct)
	}
}
