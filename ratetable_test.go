// ratetable_test.go
package main

import (
	"math"
	"testing"
)

func TestRateTableAveragesToOne(t *testing.T) {
	sum := 0.0
	for _, v := range busyHourlyRateRatio {
		sum += v
	}
	avg := sum / 24
	if avg < 0.999 || avg > 1.001 {
		t.Fatalf("expected normalized table to average to 1.0, got %v", avg)
	}
}

func TestBusyMeanAtRespectsRange(t *testing.T) {
	minMean, maxMean := math.MaxFloat64, 0.0
	for h := uint64(0); h < blocksPerHour*24; h += blocksPerHour {
		m := busyMeanAt(h, busyParams.txCountMean)
		if m < minMean {
			minMean = m
		}
		if m > maxMean {
			maxMean = m
		}
	}
	// spec: hourly averages ranged 45.4 to 175.5 around a base mean of 91.69
	if minMean > 55 || maxMean < 160 {
		t.Fatalf("expected hourly means spanning roughly 45-175, got min=%v max=%v", minMean, maxMean)
	}
}
