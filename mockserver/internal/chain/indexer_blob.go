package chain

import (
	"errors"
	"fmt"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"

	"github.com/canopy-network/canopy-rpc-mock/mockserver/internal/gen"
)

// ErrUnknownHeight is the sentinel wrapped by errUnknownHeight — callers
// (e.g. the HTTP handler in internal/rpc/server_routes.go, or a
// canopy-indexer test asserting on a not-yet-generated height) should check
// errors.Is(err, ErrUnknownHeight) rather than string-matching Error()'s
// output.
var ErrUnknownHeight = errors.New("unknown height")

// BuildIndexerBlob assembles an fsm.IndexerBlob at the given committed block
// height, mirroring the real node's fsm.StateMachine.IndexerBlob() field
// layout (see github.com/canopy-network/canopy fsm/indexer.go) — every field
// here is the marshaled-bytes form of the corresponding closed-form generator
// output, not a live struct, matching the real wire contract exactly.
func (mc *mockChain) BuildIndexerBlob(blockHeight uint64) (*fsm.IndexerBlob, error) {
	block, ok := mc.blocks[blockHeight]
	if !ok {
		return nil, errUnknownHeight(blockHeight)
	}
	blockBz, err := lib.Marshal(block)
	if err != nil {
		return nil, err
	}

	state := mc.states[blockHeight]
	if state == nil {
		return nil, errUnknownHeight(blockHeight)
	}

	accountsBz, err := marshalEach(state.Accounts)
	if err != nil {
		return nil, err
	}
	poolsBz, err := marshalEach(state.Pools)
	if err != nil {
		return nil, err
	}
	validatorsBz, err := marshalEach(state.Validators)
	if err != nil {
		return nil, err
	}
	paramsBz, err := lib.Marshal(state.Params)
	if err != nil {
		return nil, err
	}
	supplyBz, err := lib.Marshal(state.Supply)
	if err != nil {
		return nil, err
	}
	ordersBz, err := lib.Marshal(state.OrderBooks)
	if err != nil {
		return nil, err
	}
	var committeesBz []byte
	if state.Committees != nil {
		committeesBz, err = lib.Marshal(state.Committees)
		if err != nil {
			return nil, err
		}
	}

	totalActive, totalPaused, totalUnstaking := 0, 0, 0
	for _, v := range state.Validators {
		switch gen.ValidatorStatusAt(v.Address, 0, blockHeight+1) {
		case gen.ValidatorActive:
			totalActive++
		case gen.ValidatorUnstaked:
			totalUnstaking++
		}
	}

	return &fsm.IndexerBlob{
		Block:                    blockBz,
		Accounts:                 accountsBz,
		Pools:                    poolsBz,
		Validators:               validatorsBz,
		Params:                   paramsBz,
		Supply:                   supplyBz,
		Orders:                   ordersBz,
		CommitteesData:           committeesBz,
		SubsidizedCommittees:     mc.subsidizedCommittees,
		RetiredCommittees:        mc.retiredCommittees,
		TotalValidatorsActive:    uint32(totalActive),
		TotalValidatorsPaused:    uint32(totalPaused),
		TotalValidatorsUnstaking: uint32(totalUnstaking),
	}, nil
}

// BuildIndexerBlobs implements the same height/previous-height pairing as the
// real node's StateMachine.IndexerBlobs(): query height H pairs state@H with
// committed block H-1, and Previous only exists for query height > 2. This is
// the only method internal/rpc's HTTP handler calls.
func (mc *mockChain) BuildIndexerBlobs(queryHeight uint64) (*fsm.IndexerBlobs, error) {
	out := &fsm.IndexerBlobs{}
	if queryHeight <= 1 {
		return nil, errUnknownHeight(queryHeight)
	}
	current, err := mc.BuildIndexerBlob(queryHeight - 1)
	if err != nil {
		return nil, err
	}
	out.Current = current
	if queryHeight > 2 {
		previous, err := mc.BuildIndexerBlob(queryHeight - 2)
		if err != nil {
			return nil, err
		}
		out.Previous = previous
	}
	return out, nil
}

func marshalEach[T any](items []T) ([][]byte, lib.ErrorI) {
	out := make([][]byte, 0, len(items))
	for _, item := range items {
		bz, err := lib.Marshal(item)
		if err != nil {
			return nil, err
		}
		out = append(out, bz)
	}
	return out, nil
}

// errUnknownHeight wraps the ErrUnknownHeight sentinel with the actual height
// (decimal, not truncated) so both `errors.Is` checks and human-readable
// messages work correctly.
func errUnknownHeight(h uint64) error {
	return fmt.Errorf("%w: %d", ErrUnknownHeight, h)
}
