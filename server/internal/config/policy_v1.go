package config

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// retiredPolicyV1 retains historical hash inputs outside live configuration.
// Only image/backfill-era values remain here; specialties are live tuning again.
type retiredPolicyV1 struct {
	prefetchCountdown          int
	bandHigh, bandLow          float64
	backfillEnabled            bool
	botThinkMinS, botThinkMaxS float64
}

// MarshalJSON preserves the complete historical identity independently of the
// shape of live configuration. New policies serialize retired values as zero.
func (t TuningConfig) MarshalJSON() ([]byte, error) {
	type livePolicy TuningConfig
	raw, err := json.Marshal(livePolicy(t))
	if err != nil {
		return nil, err
	}
	var policy policyV1TuningConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return nil, err
	}
	if old := t.retiredPolicy; old != nil {
		policy.Timers.PrefetchCountdown = old.prefetchCountdown
		policy.Dealing.BandHigh = old.bandHigh
		policy.Dealing.BandLow = old.bandLow
		policy.Liquidity.BackfillEnabled = old.backfillEnabled
		policy.Liquidity.BotThinkMinS = old.botThinkMinS
		policy.Liquidity.BotThinkMaxS = old.botThinkMaxS
	}
	return json.Marshal(policy)
}

// UnmarshalJSON reads a version-1 durable policy without silently dropping
// unknown future fields. A failed decode leaves the existing receiver intact.
func (t *TuningConfig) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("policy must be an object")
	}
	var stored policyV1TuningConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return err
	}
	type livePolicy TuningConfig
	var current livePolicy
	if err := json.Unmarshal(raw, &current); err != nil {
		return err
	}
	current.retiredPolicy = &retiredPolicyV1{
		prefetchCountdown: stored.Timers.PrefetchCountdown,
		bandHigh:          stored.Dealing.BandHigh, bandLow: stored.Dealing.BandLow,
		backfillEnabled: stored.Liquidity.BackfillEnabled,
		botThinkMinS:    stored.Liquidity.BotThinkMinS, botThinkMaxS: stored.Liquidity.BotThinkMaxS,
	}
	*t = TuningConfig(current)
	return nil
}

func (t *TuningConfig) cloneRetiredPolicy() {
	if t.retiredPolicy == nil {
		return
	}
	copy := *t.retiredPolicy
	t.retiredPolicy = &copy
}
