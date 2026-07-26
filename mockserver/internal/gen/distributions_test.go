// distributions_test.go
package gen

import (
	"math"
	"math/rand"
	"testing"
)

func TestRngForHeightIsDeterministic(t *testing.T) {
	r1 := RngForHeight(42, 100)
	r2 := RngForHeight(42, 100)
	v1, v2 := r1.Float64(), r2.Float64()
	if v1 != v2 {
		t.Fatalf("expected identical draws for same (seed,height), got %v vs %v", v1, v2)
	}
}

func TestRngForHeightVariesByHeight(t *testing.T) {
	r1 := RngForHeight(42, 100)
	r2 := RngForHeight(42, 101)
	if r1.Float64() == r2.Float64() {
		t.Fatalf("expected different draws for different heights")
	}
}

func TestSampleNegBinomialMeanConverges(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const wantMean = 91.69
	const r = 8.0
	sum := 0
	const n = 20000
	for i := 0; i < n; i++ {
		sum += sampleNegBinomial(rng, r, wantMean)
	}
	got := float64(sum) / float64(n)
	if got < wantMean*0.9 || got > wantMean*1.1 {
		t.Fatalf("mean drifted: want ~%v, got %v", wantMean, got)
	}
}

func TestFitLogNormalRecoversMedian(t *testing.T) {
	mu, sigma := fitLogNormal(20_000_000, 87_900_000, 398_000_000)
	median := math.Exp(mu)
	if math.Abs(median-87_900_000) > 1 {
		t.Fatalf("expected median 87900000, got %v", median)
	}
	if sigma <= 0 {
		t.Fatalf("expected positive sigma, got %v", sigma)
	}
}

func TestSampleCategoricalRespectsWeights(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	opts := []weightedOption{{"a", 90}, {"b", 10}}
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		counts[sampleCategorical(rng, opts)]++
	}
	if counts["a"] < 8500 || counts["a"] > 9500 {
		t.Fatalf("expected ~9000 'a' draws, got %d", counts["a"])
	}
}

func TestSampleCategoricalEmptyOptionsDoesNotPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	result := sampleCategorical(rng, nil)
	if result != "" {
		t.Fatalf("expected empty string for nil options, got %q", result)
	}
	result = sampleCategorical(rng, []weightedOption{})
	if result != "" {
		t.Fatalf("expected empty string for empty options, got %q", result)
	}
}
