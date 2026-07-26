// scale.go
package main

type runMode string

const (
	runModeDefault runMode = "default"
	runModeSparse  runMode = "sparse"
	runModeLoad    runMode = "load"
)

type runModeParams struct {
	chainIDs      []uint64
	numAccounts   int
	numValidators int
}

// runModeConfig implements spec Section 6 (default scale stays small — dozens
// of validators, low hundreds of accounts) and Section 7 (named presets).
func runModeConfig(mode runMode) runModeParams {
	switch mode {
	case runModeSparse:
		return runModeParams{chainIDs: []uint64{1, 100, 101}, numAccounts: 10, numValidators: 10}
	case runModeLoad:
		return runModeParams{chainIDs: []uint64{1, 100, 101}, numAccounts: 100_000, numValidators: 5_000}
	default:
		return runModeParams{chainIDs: []uint64{1, 100, 101}, numAccounts: 200, numValidators: 30}
	}
}
