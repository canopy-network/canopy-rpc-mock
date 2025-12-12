Entities Dependent ONLY on Height-1 (No Database Entity Dependencies)

These activities only query RPC at height H and H-1, without querying other indexed entities from the database:

1. Block (block.go:33) - Fetches block from RPC, saves to staging
2. Transactions (transactions.go:18) - Fetches transactions from RPC(H)
3. Events (events.go:18) - Fetches events from RPC(H)
4. Params (params.go:22) - Compares RPC(H) vs RPC(H-1), snapshot-on-change
5. Supply (supply.go:27) - Compares RPC(H) vs RPC(H-1), snapshot-on-change
6. Committees (committees.go:21) - Compares RPC(H) vs RPC(H-1), snapshot-on-change
7. DexPrices (dexprices.go:20) - Compares RPC(H) vs RPC(H-1), calculates deltas

Entities Dependent on Events (Must Run After Events)

These activities query events from the staging table, so they depend on Events being indexed first:

1. Accounts (accounts.go:36)
- Queries: EventReward, EventSlash from staging
- Uses events for balance change correlation
2. Validators (validators.go:31)
- Queries: EventReward, EventSlash, EventAutoPause, EventAutoBeginUnstaking, EventAutoFinishUnstaking from staging
- Uses events for validator lifecycle state transitions
3. Orders (orders.go:38)
- Queries: EventOrderBookSwap from staging
- Uses events to determine order completion status
4. Pools (pools.go:22)
- Queries: EventDexLiquidityDeposit, EventDexLiquidityWithdraw, EventDexSwap from staging
- Uses events for pool state change correlation
5. DexBatch (dex_batch.go:47)
- Queries: EventDexSwap, EventDexLiquidityDeposit, EventDexLiquidityWithdraw from staging
- Also queries RPC(H-1) for the events-first pattern
- Most complex dependency: needs both Events and RPC(H-1) data

Must Run Last

- BlockSummary (block_summary.go:14) - Aggregates counts from ALL entity indexing activities, must run after everything else

Independent (Special Case)

- Poll (poll.go:23) - Time-based snapshots (every 20 seconds), not height-based, doesn't depend on any entities

Dependency Graph

Height-1 Only (Parallel):
├─ Block
├─ Transactions
├─ Events  ◄─────────┐
├─ Params            │
├─ Supply            │
├─ Committees        │
└─ DexPrices         │
│
Events-Dependent:    │ (depends on)
├─ Accounts ─────────┘
├─ Validators ───────┘
├─ Orders ───────────┘
├─ Pools ────────────┘
└─ DexBatch ─────────┘ (also uses RPC(H-1))

Final Stage:
└─ BlockSummary (depends on ALL above)

The key insight is that Events is a critical dependency for most complex entity types (Accounts, Validators, Orders, Pools, DexBatch) because they use the "events-first"
architecture to correlate state changes with events.