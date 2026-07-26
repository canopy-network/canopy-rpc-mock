// scale_test.go
package gen

import "testing"

func TestRunModePresetsDefault(t *testing.T) {
	cfg := RunModeConfig(RunModeDefault)
	if cfg.NumAccounts <= 0 || cfg.NumValidators <= 0 {
		t.Fatalf("expected positive default scale, got %+v", cfg)
	}
	if len(cfg.ChainIDs) != 3 || cfg.ChainIDs[0] != 1 || cfg.ChainIDs[1] != 100 || cfg.ChainIDs[2] != 101 {
		t.Fatalf("expected default topology {1,100,101}, got %v", cfg.ChainIDs)
	}
}

func TestRunModePresetsLoadScalesUp(t *testing.T) {
	def := RunModeConfig(RunModeDefault)
	load := RunModeConfig(RunModeLoad)
	if load.NumAccounts <= def.NumAccounts {
		t.Fatalf("expected load profile to scale up accounts, def=%d load=%d", def.NumAccounts, load.NumAccounts)
	}
}

func TestRunModePresetsSparseIsThin(t *testing.T) {
	sparse := RunModeConfig(RunModeSparse)
	if sparse.NumAccounts > 20 {
		t.Fatalf("expected sparse profile to stay thin, got %d accounts", sparse.NumAccounts)
	}
}
