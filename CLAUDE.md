# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a mock RPC server for the Canopy blockchain. It prebuilds deterministic blockchain data and serves it on the same routes as the real Canopy RPC. The server is intended for local integration testing where predictable but realistic responses are needed across multiple chains.

## Build and Run Commands

```bash
# Run the server (from canopy-rpc-mock directory)
go run .

# Run with custom configuration
go run . -chains 2 -blocks 25 -start-port 60000 -start-chain-id 5

# Run tests
go test ./...

# Run a single test
go test -run TestMockChainBuildsBlocks
```

## Architecture

The codebase is organized into four main files:

- **main.go**: Entry point. Parses flags and launches one HTTP server per chain on incrementing ports.
- **mock_data.go**: Contains `mockChain` struct and all data generation logic. Prebuilds blocks, transactions, events, validators, accounts, pools, order books, dex batches, and certificates at initialization time.
- **mock_state.go**: Contains `mockState` struct that tracks mutable state (accounts, validators, pools) and applies transactions. Handles block lifecycle (`beginBlock`/`endBlock`) and generates per-height state snapshots.
- **server_routes.go**: Registers HTTP handlers that map to Canopy RPC routes (`/v1/query/*`). All routes read from prebuilt data structures.

## Key Concepts

**Deterministic Randomness**: Each chain seeds its RNG with `chainID`, making all non-address data (balances, stakes, pool sizes) stable across runs while varying per chain.

**Dex Batch Processing**: The mock simulates Canopy's dex batch lifecycle:
- `nextDexBatch` accumulates dex operations each height
- Batches lock on heights where `height % 4 == 2`
- Locked batch effects/events apply 4 heights later

**State Snapshots**: `mockState.snapshot()` creates a full `GenesisState` at each height. The `/v1/query/state` endpoint and height-aware queries (`accounts`, `validators`, etc.) read from these snapshots.

**Transaction Cycling**: `txBuilders()` cycles through 15 different transaction types deterministically based on `(height-1) % 15`, covering sends, stake operations, param changes, DAO transfers, orders, and dex operations.

## RPC Routes

All routes match the Canopy RPC surface and are defined in `server_routes.go:16-42`. Key endpoints:
- `/v1/query/height` - Latest height
- `/v1/query/block-by-height` - Block data
- `/v1/query/state` - Full genesis state snapshot
- `/v1/query/txs-by-height`, `/v1/query/events-by-height` - Paginated results
- `/v1/query/dex-batch`, `/v1/query/next-dex-batch` - Dex batch state

## Testing Notes

The test file validates:
1. All blocks/certs/states are populated for requested heights
2. Account addresses match validator public keys
3. Dex events appear at expected heights after batch processing (height 22 for 25 blocks)
