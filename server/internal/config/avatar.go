package config

import "strings"

// AvatarScreeningEnabled is shared by upload activation and purchase discovery.
// An existing paid entitlement remains owned when this capability is unavailable.
func AvatarScreeningEnabled(c ContentScreeningConfig) bool {
	return c.Provider == "openai" && strings.TrimSpace(c.APIKey) != "" && strings.HasPrefix(c.Model, "omni-moderation-") && c.TimeoutS >= 0 && c.TimeoutS <= 30
}
func validateAvatarScreening(c ContentScreeningConfig) []string {
	var errs []string
	if c.Provider != "" && c.Provider != "disabled" && c.Provider != "openai" {
		errs = append(errs, "moderation.avatar_screening.provider must be disabled or openai")
	}
	if c.Provider == "openai" && !strings.HasPrefix(c.Model, "omni-moderation-") {
		errs = append(errs, "moderation.avatar_screening.model requires an image-capable omni-moderation model")
	}
	if c.TimeoutS < 0 || c.TimeoutS > 30 {
		errs = append(errs, "moderation.avatar_screening.timeout_s must be between 0 and 30")
	}
	return errs
}
