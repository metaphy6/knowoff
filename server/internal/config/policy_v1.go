package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
)

// retiredPolicyV1 retains historical hash inputs outside live configuration.
// None of these values may be used by the text runtime or loaded from YAML.
type retiredPolicyV1 struct {
	revealLockout, revealView, shuffleBonusSeconds, prefetchCountdown int
	specialtyWeights                                                  map[string]float64
	bandHigh, bandLow                                                 float64
	backfillEnabled                                                   bool
	botThinkMinS, botThinkMaxS                                        float64
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
		policy.Timers.RevealLockout = old.revealLockout
		policy.Timers.RevealView = old.revealView
		policy.Timers.ShuffleBonusSeconds = old.shuffleBonusSeconds
		policy.Timers.PrefetchCountdown = old.prefetchCountdown
		policy.Hand.SpecialtyWeights = old.specialtyWeights
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
		revealLockout:       stored.Timers.RevealLockout,
		revealView:          stored.Timers.RevealView,
		shuffleBonusSeconds: stored.Timers.ShuffleBonusSeconds,
		prefetchCountdown:   stored.Timers.PrefetchCountdown,
		specialtyWeights:    maps.Clone(stored.Hand.SpecialtyWeights),
		bandHigh:            stored.Dealing.BandHigh, bandLow: stored.Dealing.BandLow,
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
	copy.specialtyWeights = maps.Clone(copy.specialtyWeights)
	t.retiredPolicy = &copy
}
