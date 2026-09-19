package config

import "testing"

func TestRewardedConfigurationClosedAndBounded(t *testing.T) {
	if err := (RewardedConfig{}).Validate(); err != nil {
		t.Fatal(err)
	}
	base := RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, KeyCacheS: 3600, KeyRefreshMinS: 60, MaxConcurrentRequests: 4, ClaimTTLS: 1800, MaxClaimsPerMatchWindow: 4, AdUnits: map[string]RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []func(*RewardedConfig){
		func(c *RewardedConfig) { c.MaxQueryBytes = 0 }, func(c *RewardedConfig) { c.MaxResponseBytes = 1048577 }, func(c *RewardedConfig) { c.HTTPTimeoutS = 0 }, func(c *RewardedConfig) { c.KeyCacheS = 86401 }, func(c *RewardedConfig) { c.KeyRefreshMinS = 0 }, func(c *RewardedConfig) { c.MaxConcurrentRequests = 33 }, func(c *RewardedConfig) { c.ClaimTTLS = 0 }, func(c *RewardedConfig) { c.MaxClaimsPerMatchWindow = 0 }, func(c *RewardedConfig) { c.MaxClaimsPerMatchWindow = 33 }, func(c *RewardedConfig) { c.AdUnits = nil }, func(c *RewardedConfig) {
			c.AdUnits = map[string]RewardedUnit{"123": {RewardItem: "has spaces", RewardAmount: 1}}
		}, func(c *RewardedConfig) {
			c.AdUnits = map[string]RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 0}}
		},
	} {
		c := base
		bad(&c)
		if err := c.Validate(); err == nil {
			t.Fatal("invalid rewarded configuration accepted")
		}
	}
}
