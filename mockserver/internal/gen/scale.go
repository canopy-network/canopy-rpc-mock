// scale.go
package gen

type RunMode string

const (
	RunModeDefault RunMode = "default"
	RunModeSparse  RunMode = "sparse"
	RunModeLoad    RunMode = "load"
)

// RunModeParams' fields are exported (ChainIDs/NumAccounts/NumValidators)
// because internal/chain's repro_test.go (Task 12, moved under this task)
// iterates cfg.ChainIDs directly across the package boundary.
type RunModeParams struct {
	ChainIDs      []uint64
	NumAccounts   int
	NumValidators int
}

// RunModeConfig implements spec Section 6 (default scale stays small — dozens
// of validators, low hundreds of accounts) and Section 7 (named presets).
func RunModeConfig(mode RunMode) RunModeParams {
	switch mode {
	case RunModeSparse:
		return RunModeParams{ChainIDs: []uint64{1, 100, 101}, NumAccounts: 10, NumValidators: 10}
	case RunModeLoad:
		return RunModeParams{ChainIDs: []uint64{1, 100, 101}, NumAccounts: 100_000, NumValidators: 5_000}
	default:
		return RunModeParams{ChainIDs: []uint64{1, 100, 101}, NumAccounts: 200, NumValidators: 30}
	}
}
