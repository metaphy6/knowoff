package config

import (
	"errors"
	"fmt"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads base.yaml, gameplay/tuning.yaml, overlays envFile, interpolates
// ${VAR} references, rejects unknown keys, and validates the merged result.
// It returns an error that aggregates every problem found, so callers can
// surface all missing/invalid keys in one pass.
func Load(basePath, envFile string) (*Config, error) {
	baseRaw, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("read base config: %w", err)
	}

	merged := string(baseRaw)

	// gameplay/tuning.yaml lives next to base.yaml (same configs directory).
	// Its top-level keys belong under the "tuning" key in the merged config.
	tuningPath := filepath.Join(filepath.Dir(basePath), "gameplay", "tuning.yaml")
	if tuningRaw, err := os.ReadFile(tuningPath); err == nil {
		wrapped := fmt.Sprintf("tuning:\n%s", indentLines(string(tuningRaw)))
		merged, err = mergeYAML(merged, wrapped)
		if err != nil {
			return nil, fmt.Errorf("merge gameplay tuning: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read gameplay tuning: %w", err)
	}

	if envFile != "" {
		overlayRaw, err := os.ReadFile(envFile)
		if err != nil {
			return nil, fmt.Errorf("read overlay config %q: %w", envFile, err)
		}
		merged, err = mergeYAML(merged, string(overlayRaw))
		if err != nil {
			return nil, fmt.Errorf("merge config overlay: %w", err)
		}
	}

	var problems []string

	if err := ValidateTextCutover([]byte(merged)); err != nil {
		return nil, err
	}

	interpolated, missing := interpolateEnvVars(merged)
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("missing secret environment variables: %s", strings.Join(missing, ", ")))
	}

	cfg, unknown := unmarshalStrict([]byte(interpolated))
	if len(unknown) > 0 {
		problems = append(problems, fmt.Sprintf("unknown config keys: %s", strings.Join(unknown, "; ")))
	}

	if cfg != nil {
		if key, ok := os.LookupEnv("KNOWOFF_DEV_BOT_KEY"); ok {
			cfg.Security.DevBotKey = key
		}
		if valErrs := validate(cfg); len(valErrs) > 0 {
			problems = append(problems, valErrs...)
		}
	}

	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}

	return cfg, nil
}

// mergeYAML deep-merges two YAML documents. Scalar and sequence values from
// the overlay replace those in the base; mappings are merged recursively.
func mergeYAML(base, overlay string) (string, error) {
	var baseMap, overlayMap map[string]any
	if err := yaml.Unmarshal([]byte(base), &baseMap); err != nil {
		return "", fmt.Errorf("parse base yaml: %w", err)
	}
	if err := yaml.Unmarshal([]byte(overlay), &overlayMap); err != nil {
		return "", fmt.Errorf("parse overlay yaml: %w", err)
	}

	merged := deepMergeMaps(baseMap, overlayMap)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("marshal merged yaml: %w", err)
	}
	return string(out), nil
}

// indentLines prefixes every non-empty line of s with two spaces so it can be
// nested under a YAML key.
func indentLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

func deepMergeMaps(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if baseVal, ok := out[k]; ok {
			if baseMap, ok1 := baseVal.(map[string]any); ok1 {
				if overlayMap, ok2 := v.(map[string]any); ok2 {
					out[k] = deepMergeMaps(baseMap, overlayMap)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

// interpolateEnvVars replaces ${VAR} with the value of VAR. It returns the
// interpolated string and a deduplicated list of missing variable names.
func interpolateEnvVars(input string) (string, []string) {
	seen := map[string]bool{}
	var missing []string
	out := envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		name := match[2 : len(match)-1]
		val, ok := os.LookupEnv(name)
		if !ok {
			if !seen[name] {
				seen[name] = true
				missing = append(missing, name)
			}
			return match
		}
		return val
	})
	return out, missing
}

// unmarshalStrict decodes YAML into Config while collecting unknown-key errors.
func unmarshalStrict(data []byte) (*Config, []string) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		// KnownFields produces errors like "line N: field foo not found in type ...".
		// For the first Phase 1 gate we surface the raw message.
		return nil, []string{err.Error()}
	}
	return &cfg, nil
}

// validate checks that every required value is present and well-formed.
func validate(cfg *Config) []string {
	var errs []string
	if cfg.App.Name == "" {
		errs = append(errs, "app.name is required")
	}
	if cfg.Server.Port <= 0 {
		errs = append(errs, fmt.Sprintf("server.port must be > 0, got %d", cfg.Server.Port))
	}
	if cfg.Server.AdminPort <= 0 {
		errs = append(errs, fmt.Sprintf("server.admin_port must be > 0, got %d", cfg.Server.AdminPort))
	}
	if cfg.Server.MetricsPort <= 0 {
		errs = append(errs, fmt.Sprintf("server.metrics_port must be > 0, got %d", cfg.Server.MetricsPort))
	}
	if cfg.Server.MaxConnections < 0 {
		errs = append(errs, fmt.Sprintf("server.max_connections must be >= 0, got %d", cfg.Server.MaxConnections))
	}
	if cfg.Database.Host == "" {
		errs = append(errs, "database.host is required")
	}
	if cfg.Database.Password == "" {
		errs = append(errs, "database.password is required")
	}
	if cfg.Redis.Password == "" {
		errs = append(errs, "redis.password is required")
	}
	if cfg.Security.JWTSigningKey == "" {
		errs = append(errs, "security.jwt_signing_key is required")
	}
	if key := cfg.Security.DevBotKey; key != "" && (len(key) < 32 || len(key) > 512 || cfg.App.Env == "prod") {
		errs = append(errs, "security.dev_bot_key requires 32–512 bytes and a non-production environment")
	}
	for _, lvl := range cfg.Localization.SupportedLocales {
		if lvl == "" {
			errs = append(errs, "localization.supported_locales must not contain empty entries")
			break
		}
	}
	screening := cfg.Moderation.ContentScreening
	if screening.Provider != "" && screening.Provider != "disabled" && screening.Provider != "openai" {
		errs = append(errs, "moderation.content_screening.provider must be disabled or openai")
	}
	if screening.Provider == "openai" && strings.TrimSpace(screening.Model) == "" {
		errs = append(errs, "moderation.content_screening.model is required for openai")
	}
	if screening.TimeoutS < 0 || screening.TimeoutS > 30 {
		errs = append(errs, "moderation.content_screening.timeout_s must be between 0 and 30")
	}
	errs = append(errs, validateAvatarScreening(cfg.Moderation.AvatarScreening)...)
	if cfg.Moderation.AvatarUploadSlots < 0 || cfg.Moderation.AvatarUploadSlots > 16 {
		errs = append(errs, "moderation.avatar_upload_slots must be between 0 and 16 (0 uses 2)")
	}
	errs = append(errs, validateTextConfig(cfg)...)
	errs = append(errs, validateTrustConfig(cfg.Trust)...)
	errs = append(errs, validateOAuthConfig(cfg.Security.OAuth)...)
	errs = append(errs, validateBilling(cfg.Billing, cfg.Tuning.Economy)...)
	if err := cfg.Rewarded.Validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if (cfg.App.Env == "prod" || cfg.App.Env == "production") && (cfg.Billing.Google.AllowTestPurchases || cfg.Billing.Apple.Environment == "Sandbox") {
		errs = append(errs, "billing sandbox verification is forbidden in production")
	}
	return errs
}

func validateTrustConfig(c TrustConfig) []string {
	var errs []string
	if c.UserTermsVersion != "" && !gamecontract.ValidIdentifier(c.UserTermsVersion) {
		errs = append(errs, "trust.user_terms_version must be a stable identifier")
	}
	for key, value := range map[string]string{"support_url": c.SupportURL, "privacy_url": c.PrivacyURL} {
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(value) > 2048 {
			errs = append(errs, "trust."+key+" must be an absolute HTTPS URL without credentials")
		}
	}
	return errs
}
