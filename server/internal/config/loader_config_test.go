package config

import (
	"fmt"
	"testing"
)

func TestLoadLocalCap(t *testing.T) {
	cfg, err := Load("../../../configs/base.yaml", "../../../configs/local.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fmt.Printf("free_daily_quickplay_matches = %d\n", cfg.Tuning.Economy.FreeDailyQuickplayMatches)
	if cfg.Tuning.Economy.FreeDailyQuickplayMatches != 10000 {
		t.Errorf("expected 10000, got %d", cfg.Tuning.Economy.FreeDailyQuickplayMatches)
	}
}
