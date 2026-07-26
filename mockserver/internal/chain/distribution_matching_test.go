// distribution_matching_test.go
package chain

import (
	"testing"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/gen"
	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
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
	eventTypeCounts := map[string]int{}
	for h := uint64(1); h <= heights; h++ {
		n := len(mc.txs[h])
		sum += n
		if n == 0 {
			zeroCount++
		}
		for _, tx := range mc.txs[h] {
			typeCounts[tx.MessageType]++
		}
		for _, ev := range mc.events[h] {
			eventTypeCounts[ev.EventType]++
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

	// Busy event-type mix must be reward-led (spec Section 4: busy is
	// reward-led, quiet is dex-swap-led). This is exactly the assertion the
	// slot-0 RNG seed collision (final review Fix 1) would have failed had it
	// existed before: the bug flipped busy-profile plurality between
	// reward/dexSwap.
	rewardCount := eventTypeCounts[string(lib.EventTypeReward)]
	dexSwapCount := eventTypeCounts[string(lib.EventTypeDexSwap)]
	if rewardCount == 0 && dexSwapCount == 0 {
		t.Fatalf("expected some reward/dexSwap events in busy sweep, got none (reward=%d dexSwap=%d)", rewardCount, dexSwapCount)
	}
	if rewardCount <= dexSwapCount {
		t.Fatalf("busy event mix should be reward-led: reward=%d dexSwap=%d", rewardCount, dexSwapCount)
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
	eventTypeCounts := map[string]int{}
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
		for _, ev := range mc.events[h] {
			eventTypeCounts[ev.EventType]++
		}
	}
	zeroPct := float64(zeroCount) / float64(heights) * 100
	if zeroPct < 53.7*0.8 || zeroPct > 53.7*1.2 {
		t.Fatalf("quiet zero-tx-block pct out of tolerance: want ~53.7%%, got %.1f%%", zeroPct)
	}

	// Quiet tx-type mix: dexLiqDeposit (64.6% target) must be the plurality
	// over editStake (34.3%) and dexLimitOrder (1.1%). This is exactly the
	// assertion the slot-0 RNG seed collision (final review Fix 1) broke: the
	// bug inverted the quiet mix so editStake outnumbered dexLiqDeposit
	// (measured ~56% vs ~41% before the fix).
	dexLiqDepositCount := typeCounts[fsm.MessageDexLiquidityDepositName]
	editStakeCount := typeCounts[fsm.MessageEditStakeName]
	dexLimitOrderCount := typeCounts[fsm.MessageDexLimitOrderName]
	if dexLiqDepositCount == 0 && editStakeCount == 0 {
		t.Fatalf("expected some dexLiqDeposit/editStake txs in quiet sweep, got none")
	}
	if dexLiqDepositCount <= editStakeCount {
		t.Fatalf("quiet tx-type mix should be dexLiqDeposit-led: dexLiqDeposit=%d editStake=%d", dexLiqDepositCount, editStakeCount)
	}
	if dexLiqDepositCount <= dexLimitOrderCount {
		t.Fatalf("quiet tx-type mix should be dexLiqDeposit-led: dexLiqDeposit=%d dexLimitOrder=%d", dexLiqDepositCount, dexLimitOrderCount)
	}
	dexLiqDepositPct := float64(dexLiqDepositCount) / float64(total) * 100
	if dexLiqDepositPct < 64.6*0.7 || dexLiqDepositPct > 64.6*1.3 {
		t.Fatalf("quiet dexLiqDeposit tx-type proportion out of tolerance: want ~64.6%%, got %.1f%%", dexLiqDepositPct)
	}

	// Quiet event-type mix must be dex-swap-led (spec Section 4), the
	// opposite plurality from busy. Same bug class as above but on the events
	// path (EventTypeMixAt/EventCountAt share a seed the same way
	// TxTypeMixAt/TxCountAt do).
	dexSwapCount := eventTypeCounts[string(lib.EventTypeDexSwap)]
	rewardCount := eventTypeCounts[string(lib.EventTypeReward)]
	if dexSwapCount == 0 && rewardCount == 0 {
		t.Fatalf("expected some dexSwap/reward events in quiet sweep, got none")
	}
	if dexSwapCount <= rewardCount {
		t.Fatalf("quiet event mix should be dexSwap-led: dexSwap=%d reward=%d", dexSwapCount, rewardCount)
	}
}
