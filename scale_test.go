// scale_test.go
package main

import "testing"

func TestRunModePresetsDefault(t *testing.T) {
	cfg := runModeConfig(runModeDefault)
	if cfg.numAccounts <= 0 || cfg.numValidators <= 0 {
		t.Fatalf("expected positive default scale, got %+v", cfg)
	}
	if len(cfg.chainIDs) != 3 || cfg.chainIDs[0] != 1 || cfg.chainIDs[1] != 100 || cfg.chainIDs[2] != 101 {
		t.Fatalf("expected default topology {1,100,101}, got %v", cfg.chainIDs)
	}
}

func TestRunModePresetsLoadScalesUp(t *testing.T) {
	def := runModeConfig(runModeDefault)
	load := runModeConfig(runModeLoad)
	if load.numAccounts <= def.numAccounts {
		t.Fatalf("expected load profile to scale up accounts, def=%d load=%d", def.numAccounts, load.numAccounts)
	}
}

func TestRunModePresetsSparseIsThin(t *testing.T) {
	sparse := runModeConfig(runModeSparse)
	if sparse.numAccounts > 20 {
		t.Fatalf("expected sparse profile to stay thin, got %d accounts", sparse.numAccounts)
	}
}
