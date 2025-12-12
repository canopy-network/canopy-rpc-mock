# canopy-rpc-mock

This mock RPC server prebuilds deterministic blockchain data and serves it on the same routes as the real Canopy RPC. It is intended for local integration testing where you want predictable but realistic responses across multiple chains.

## Quick start

```bash
go run . \
  -chains 2 \            # how many chains to serve
  -blocks 25 \           # blocks to prebuild per chain
  -start-port 60000 \    # first RPC port; increments per chain
  -start-chain-id 5      # chain ID for the first chain; increments per chain
```

Each chain runs on its own port (e.g., 60000, 60001, …) with its own `chainID`. Routes match the Canopy RPC surface (e.g., `/v1/query/block-by-height`, `/v1/query/state`, `/v1/query/dex-batch`, etc.).

## Data model highlights

- **Blocks/txs/events**: 25 (configurable) blocks are prebuilt with deterministic transactions covering send, stake/unstake, param changes, DAO transfer, orders, and dex operations. Timestamps are Unix microseconds starting 2024-01-01 UTC, 6s per block.
- **Dex batches**: `nextDexBatch` accrues dex ops every height; the locked `dexBatch` rotates on heights where `height % 4 == 2`, and its effects/events are applied 4 heights later.
- **Per-chain variation**: All non-address fields (stakes, balances, pool sizes, params) are randomized with a per-chain seed. Addresses and keys stay deterministic.
- **State snapshots**: `/v1/query/state` returns per-height genesis snapshots; `/v1/query/accounts`, `/pools`, `/validators`, `/orders`, `/supply`, `/non-signers`, `/double-signers` all read from those snapshots.
- **Certificates**: `cert-by-height` and certificate-result txs use populated QCs (phase `PRECOMMIT_VOTE`) with matching block hashes, reward recipients, checkpoint, and dex batch info.

## Flags

- `-chains` (int, default 1): number of chains to launch.
- `-start-port` (int, default 60000): port for the first chain; increments per chain.
- `-blocks` (int, default 25): number of blocks to prebuild per chain.
- `-start-chain-id` (uint64, default 5): chain ID for the first chain; increments per chain.

## Notes

- The server runs until killed; each chain is an independent HTTP server.
- Events and dex batch updates appear only after the scheduled batch processing height (+4 from inclusion).
- Randomness is deterministic per chain (seeded by `chainID`) to keep responses stable across runs.***
