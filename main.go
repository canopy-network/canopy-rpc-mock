package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/canopy-network/canopy-rpc-mock/mockserver"
)

func main() {
	var (
		blockCount   = flag.Int("blocks", 50, "number of blocks to prebuild per chain")
		numChains    = flag.Int("chains", 3, "number of chains to serve")
		startPort    = flag.Int("start-port", 60000, "starting port for first chain")
		startChainID = flag.Uint64("start-chain-id", 1, "starting chain ID")
	)
	flag.Parse()

	servers := make([]*mockserver.Server, 0, *numChains)
	for i := 0; i < *numChains; i++ {
		chainID := *startChainID + uint64(i)
		addr := fmt.Sprintf(":%d", *startPort+i)
		srv := mockserver.New(chainID, mockserver.WithBlocks(*blockCount), mockserver.WithAddr(addr))
		servers = append(servers, srv)
		log.Printf("mock RPC server ready on %s (chainID=%d)", srv.URL, chainID)
	}

	select {} // block forever
}
