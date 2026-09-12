package config

import (
	"fmt"
	"regexp"
)

// RewardedConfig contains engineering limits and exact operator-owned AdMob
// callback identities. It never supplies the amount of a match's Noin bonus.
type RewardedConfig struct {
	Enabled               bool                    `yaml:"enabled"`
	MaxQueryBytes         int64                   `yaml:"max_query_bytes"`
	MaxResponseBytes      int64                   `yaml:"max_response_bytes"`
	HTTPTimeoutS          int                     `yaml:"http_timeout_s"`
	KeyCacheS             int                     `yaml:"key_cache_s"`
	KeyRefreshMinS        int                     `yaml:"key_refresh_min_s"`
	MaxConcurrentRequests int                     `yaml:"max_concurrent_requests"`
	ClaimTTLS             int                     `yaml:"claim_ttl_s"`
	AdUnits               map[string]RewardedUnit `yaml:"ad_units"`
}
type RewardedUnit struct {
	RewardItem   string `yaml:"reward_item"`
	RewardAmount int    `yaml:"reward_amount"`
}

func (c RewardedConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.MaxQueryBytes < 1024 || c.MaxQueryBytes > 16384 || c.MaxResponseBytes < 1024 || c.MaxResponseBytes > 1048576 || c.HTTPTimeoutS < 1 || c.HTTPTimeoutS > 30 || c.KeyCacheS < 60 || c.KeyCacheS > 86400 || c.KeyRefreshMinS < 1 || c.KeyRefreshMinS > 300 || c.KeyRefreshMinS > c.KeyCacheS || c.MaxConcurrentRequests < 1 || c.MaxConcurrentRequests > 32 || c.ClaimTTLS < 60 || c.ClaimTTLS > 86400 {
		return fmt.Errorf("rewarded_ads verification limits are invalid")
	}
	if len(c.AdUnits) < 1 || len(c.AdUnits) > 100 {
		return fmt.Errorf("rewarded_ads requires 1..100 exact ad units")
	}
	for id, u := range c.AdUnits {
		if !regexp.MustCompile(`^[0-9]{1,32}$`).MatchString(id) || !regexp.MustCompile(`^[A-Za-z0-9._~-]{1,64}$`).MatchString(u.RewardItem) || u.RewardAmount < 1 || u.RewardAmount > 1000000000 {
			return fmt.Errorf("rewarded_ads ad unit identity is invalid")
		}
	}
	return nil
}
