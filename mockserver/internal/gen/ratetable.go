// ratetable.go
package gen

// blocksPerHour is derived from Canopy's ~20s block time (3600/20).
const blocksPerHour = 180

// busyHourlyRateRatioRaw anchors the two documented features of chain_1's
// diurnal pattern (spec Section 5): a dip in the 23:00-01:00 window and a
// spike in the 04:00-05:00 window, scaled so the hourly range roughly matches
// the measured 45.4-175.5 swing around a ~91.69 base mean. These are
// placeholder anchors, not the real 24 measured hourly averages — replace
// with the actual per-hour tx/block averages from staging once available:
//
//	SELECT date_trunc('hour', b.time_column) AS hour,
//	       AVG(b.num_txs) AS avg_txs
//	FROM crosschain.block_summaries b
//	WHERE b.chain_id = 1 AND b.time_column >= now() - interval '24 hours'
//	GROUP BY 1 ORDER BY 1;
//
// (adjust table/column names to whatever canopy-indexer's postgres schema
// actually calls the per-block timestamp and tx-count columns).
var busyHourlyRateRatioRaw = [24]float64{
	0.55, 0.50, 0.60, 0.90, 1.91, 1.85, 1.30, 1.15,
	1.05, 1.00, 0.95, 0.95, 1.00, 1.05, 1.05, 1.00,
	1.05, 1.10, 1.05, 1.00, 0.90, 0.80, 0.65, 0.50,
}

var busyHourlyRateRatio = normalizeRateTable(busyHourlyRateRatioRaw)

// normalizeRateTable rescales a raw 24-hour ratio table so its average is
// exactly 1.0 — guarantees baseMean * table[...] reproduces the measured
// overall mean regardless of how the raw anchors were chosen.
func normalizeRateTable(raw [24]float64) [24]float64 {
	sum := 0.0
	for _, v := range raw {
		sum += v
	}
	avg := sum / 24
	var out [24]float64
	for i, v := range raw {
		out[i] = v / avg
	}
	return out
}

// HourOfHeight is exported because internal/chain's test suite
// (distribution_matching_test.go) buckets tx counts by hour to check the
// diurnal calibration — it isn't reachable from within gen alone once tests
// moved to a separate package.
func HourOfHeight(height uint64) int {
	return int((height / blocksPerHour) % 24)
}

// busyMeanAt reparameterizes the tx-count distribution's mean by hour of day.
// Quiet profile does not call this (spec Section 5: appchain traffic looked
// stationary across sample windows).
func busyMeanAt(height uint64, baseMean float64) float64 {
	return baseMean * busyHourlyRateRatio[HourOfHeight(height)]
}
