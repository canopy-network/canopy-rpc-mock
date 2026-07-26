// distributions.go
package gen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
)

// HeightSeed derives a deterministic uint64 seed from a base seed and a height,
// reusing the existing HashBytes() helper for consistency with
// blockHash()/HashBytes()-based determinism elsewhere in this package.
func HeightSeed(baseSeed, height uint64) uint64 {
	sum := HashBytes(baseSeed, height)
	return binary.BigEndian.Uint64(sum[:8])
}

// AddrHeightSeed derives a deterministic seed from an address and a height.
func AddrHeightSeed(addr []byte, height uint64) uint64 {
	sum := HashBytes(addr, height)
	return binary.BigEndian.Uint64(sum[:8])
}

func RngForHeight(baseSeed, height uint64) *rand.Rand {
	return rand.New(rand.NewSource(int64(HeightSeed(baseSeed, height))))
}

func RngForAddrHeight(addr []byte, height uint64) *rand.Rand {
	return rand.New(rand.NewSource(int64(AddrHeightSeed(addr, height))))
}

// StandardNormal draws one N(0,1) sample via Box-Muller.
func StandardNormal(rng *rand.Rand) float64 {
	u1, u2 := rng.Float64(), rng.Float64()
	if u1 < 1e-12 {
		u1 = 1e-12
	}
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// sampleGamma draws Gamma(shape, scale) via Marsaglia-Tsang; boosts shape<1 per
// the standard shape+1 / U^(1/shape) correction.
func sampleGamma(rng *rand.Rand, shape, scale float64) float64 {
	if shape < 1 {
		u := rng.Float64()
		return sampleGamma(rng, shape+1, scale) * math.Pow(u, 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		var x, v float64
		for {
			x = StandardNormal(rng)
			v = 1 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v * scale
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v * scale
		}
	}
}

// samplePoisson draws Poisson(lambda) via Knuth's algorithm. Adequate for the
// lambda ranges this package needs (single/low-hundreds per block).
func samplePoisson(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}
	l := math.Exp(-lambda)
	k := 0
	p := 1.0
	for {
		k++
		p *= rng.Float64()
		if p <= l {
			return k - 1
		}
	}
}

// sampleNegBinomial draws a negative-binomial count with the given dispersion
// r and target mean, via the standard Gamma-Poisson mixture:
// lambda ~ Gamma(r, (1-p)/p), X ~ Poisson(lambda), where p = r/(r+mean).
func sampleNegBinomial(rng *rand.Rand, r, mean float64) int {
	p := r / (r + mean)
	lambda := sampleGamma(rng, r, (1-p)/p)
	return samplePoisson(rng, lambda)
}

// standard normal quantiles at the 25th/75th percentile, used by fitLogNormal.
const (
	z25 = -0.6744897501960817
	z75 = 0.6744897501960817
)

// fitLogNormal solves (mu, sigma) for a log-normal distribution from its
// measured 25th/50th/75th percentiles: mu = ln(median), sigma from the IQR
// (more robust to tail noise than solving from a single percentile pair).
func fitLogNormal(p25, median, p75 float64) (mu, sigma float64) {
	mu = math.Log(median)
	sigma = (math.Log(p75) - math.Log(p25)) / (z75 - z25)
	return mu, sigma
}

func sampleLogNormal(rng *rand.Rand, mu, sigma float64) float64 {
	return math.Exp(mu + sigma*StandardNormal(rng))
}

type weightedOption struct {
	label  string
	weight float64
}

// sampleCategorical draws one label with probability proportional to its weight.
func sampleCategorical(rng *rand.Rand, options []weightedOption) string {
	if len(options) == 0 {
		return ""
	}
	total := 0.0
	for _, o := range options {
		total += o.weight
	}
	r := rng.Float64() * total
	cum := 0.0
	for _, o := range options {
		cum += o.weight
		if r <= cum {
			return o.label
		}
	}
	return options[len(options)-1].label
}

// HashBytes was originally defined in mock_data.go (package main); it moved
// here because HeightSeed/AddrHeightSeed depend on it and both live in gen
// now. It has no mockChain dependency — it's a pure hash helper.
func HashBytes(parts ...any) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(fmt.Sprint(p)))
	}
	return h.Sum(nil)
}

// PickAccountIndex was originally defined in mock_data.go (package main);
// like HashBytes it moved here — it's a pure hash-index helper with no
// mockChain dependency, and the rename table pairs it with the other gen
// exports. It maps a hash seed onto [0, n) without ever going through a
// signed int cast of the raw uint64 first. AddrHeightSeed's return value
// routinely exceeds math.MaxInt64; casting that straight to int wraps to a
// negative number, and Go's % keeps the sign of the dividend — so
// int(seed) % n can be negative and panic on the subsequent slice index.
// Reducing mod n while still in uint64 space avoids that entirely.
func PickAccountIndex(seed uint64, n int) int {
	return int(seed % uint64(n))
}
