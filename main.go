package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	var (
		blockCount   = flag.Int("blocks", 25, "number of blocks to prebuild per chain")
		numChains    = flag.Int("chains", 2, "number of chains to serve")
		startPort    = flag.Int("start-port", 60000, "starting port for first chain")
		startChainID = flag.Uint64("start-chain-id", 5, "starting chain ID")
	)
	flag.Parse()

	chains := make([]*mockChain, 0, *numChains)
	for i := 0; i < *numChains; i++ {
		chainID := *startChainID + uint64(i)
		chain := newMockChain(*blockCount, chainID)
		chains = append(chains, chain)

		port := *startPort + i
		addr := ":" + strconv.Itoa(port)
		mux := http.NewServeMux()
		registerRoutes(mux, chain)

		log.Printf("mock RPC server ready on %s (chainID=%d)", addr, chainID)
		go func(address string, mux *http.ServeMux) {
			log.Fatal(http.ListenAndServe(address, mux))
		}(addr, mux)
	}

	select {} // block forever
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
