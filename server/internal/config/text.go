package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"gopkg.in/yaml.v3"
)

// TuningSnapshotVersion identifies the typed JSON hash format. Match contracts
// pin the complete policy, not the smaller dealing-only certification hash.
const TuningSnapshotVersion = "text-tuning-v1"

func (t TuningConfig) Clone() TuningConfig {
	t.cloneRetiredPolicy()
	t.Game.RoomSizes = slices.Clone(t.Game.RoomSizes)
	t.Game.DonowersBySize = maps.Clone(t.Game.DonowersBySize)
	t.Game.VotesBySize = maps.Clone(t.Game.VotesBySize)
	t.Game.AbandonCooldownsS = slices.Clone(t.Game.AbandonCooldownsS)
	t.Economy.PlayPassPrices = maps.Clone(t.Economy.PlayPassPrices)
	t.Economy.UnlockPrices = maps.Clone(t.Economy.UnlockPrices)
	t.Economy.NoinBundles = slices.Clone(t.Economy.NoinBundles)
	t.Progression.LevelThresholds = slices.Clone(t.Progression.LevelThresholds)
	return t
}

func (t TuningConfig) SHA256() (string, error) {
	// encoding/json sorts map keys. Field order is versioned by the constant.
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// TextConfig selects intended runtime cells. A cell is available only when its
// immutable certified release also resolves through the server release store.
type TextConfig struct {
	Version       int                                    `yaml:"version"`
	RulesVersion  string                                 `yaml:"rules_version"`
	DefaultMode   gamecontract.ModeID                    `yaml:"default_mode"`
	Compatibility TextCompatibility                      `yaml:"compatibility"`
	Modes         map[gamecontract.ModeID]TextModeConfig `yaml:"modes"`
}

type TextCompatibility struct {
	ProtocolVersion     int `yaml:"protocol_version"`
	MinClientGeneration int `yaml:"min_client_generation"`
}

type TextModeConfig struct {
	Enabled          bool     `yaml:"enabled"`
	ContentLanguages []string `yaml:"content_languages"`
}

// ContractTuning bounds v2 input, deduplication and paged public evidence. The
// numeric defaults live only in gameplay/tuning.yaml; runtime wiring is Phase 4.
type ContractTuning struct {
	MaxHistoryEvents     int `yaml:"max_history_events"`
	MaxHistoryPageEvents int `yaml:"max_history_page_events"`
	MaxTextBytes         int `yaml:"max_text_bytes"`
	MaxRequestsPerSeat   int `yaml:"max_requests_per_seat"`
}

// TextCatalogTuning bounds private bundle loading and complete-schedule search.
type TextCatalogTuning struct {
	MaxRecords     int   `yaml:"max_records"`
	MaxFileBytes   int64 `yaml:"max_file_bytes"`
	MaxBundleBytes int64 `yaml:"max_bundle_bytes"`
	MaxSearchNodes int   `yaml:"max_search_nodes"`
}

func validateTextConfig(cfg *Config) []string {
	if cfg.Text == nil { // Small validation fixtures may omit the runtime policy.
		return nil
	}
	var errs []string
	text := cfg.Text
	if text.Version != 2 {
		errs = append(errs, "text.version must be 2")
	}
	if !gamecontract.ValidIdentifier(text.RulesVersion) {
		errs = append(errs, "text.rules_version must be a 1–128 byte ASCII identifier using letters, digits, hyphen, underscore, period or colon")
	}
	if !text.DefaultMode.Valid() {
		errs = append(errs, "text.default_mode must name an intended text mode")
	}
	if text.Compatibility.ProtocolVersion != 2 {
		errs = append(errs, "text.compatibility.protocol_version must be 2")
	}
	if text.Compatibility.MinClientGeneration < 2 {
		errs = append(errs, "text.compatibility.min_client_generation must be at least 2")
	}
	for _, mode := range gamecontract.AllModes() {
		if _, ok := text.Modes[mode]; !ok {
			errs = append(errs, fmt.Sprintf("text.modes.%s is required", mode))
		}
	}
	for mode, availability := range text.Modes {
		if !mode.Valid() {
			errs = append(errs, "text.modes contains an unknown mode")
		}
		if availability.Enabled && len(availability.ContentLanguages) == 0 {
			errs = append(errs, fmt.Sprintf("text.modes.%s.content_languages must name an enabled cell", mode))
		}
		seen := make(map[string]bool)
		for _, language := range availability.ContentLanguages {
			if !gamecontract.ValidContentLanguage(language) || seen[language] {
				errs = append(errs, fmt.Sprintf("text.modes.%s.content_languages must contain unique canonical language tags", mode))
			}
			seen[language] = true
		}
	}
	if cfg.Tuning.Timers.TradeResponseS <= 0 || cfg.Tuning.Timers.TradeResponseS > 60 {
		errs = append(errs, "tuning.timers.trade_response_s must be between 1 and 60")
	}
	if cfg.Tuning.Timers.VoteResultFalling <= 0 || cfg.Tuning.Timers.VoteResultFalling >= cfg.Tuning.Timers.VoteResultWindow {
		errs = append(errs, "tuning.timers.vote_result_falling must be positive and precede vote_result_window")
	}
	if cfg.Tuning.Timers.RoundStartCountdown < 0 || cfg.Tuning.Timers.RoundStartCountdown > 60 {
		errs = append(errs, "tuning.timers.round_start_countdown must be between 0 and 60")
	}
	limits := cfg.Tuning.Contract
	catalog := cfg.Tuning.TextCatalog
	if catalog.MaxRecords <= 0 || catalog.MaxFileBytes <= 0 || catalog.MaxBundleBytes < catalog.MaxFileBytes || catalog.MaxSearchNodes <= 0 {
		errs = append(errs, "tuning.text_catalog budgets must be positive and file bytes cannot exceed bundle bytes")
	}
	frame := cfg.WebSocket.MaxMessageBytes
	if cfg.RateLimit.MaxBytesPerFrame > 0 && cfg.RateLimit.MaxBytesPerFrame < frame {
		frame = cfg.RateLimit.MaxBytesPerFrame
	}
	if limits.MaxHistoryEvents <= 0 || limits.MaxHistoryPageEvents <= 0 || limits.MaxHistoryPageEvents > limits.MaxHistoryEvents || limits.MaxTextBytes <= 0 || limits.MaxTextBytes > frame || limits.MaxRequestsPerSeat <= 0 {
		errs = append(errs, "tuning.contract budgets must be positive; pages cannot exceed history and text cannot exceed the frame budget")
	}
	sort.Strings(errs)
	return errs
}

// ValidateTextCutover is a read-only key-presence preflight over a fully merged
// YAML document, called by Load before secret interpolation. Passing it alone
// neither validates a complete config nor authorizes removing historical data.
func ValidateTextCutover(mergedYAML []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(mergedYAML, &document); err != nil {
		return fmt.Errorf("config_migration_invalid_yaml")
	}
	obsolete := []string{
		"storage",
		"media",
		"bots",
		"security.ssv_callback_key",
		"security.ssv_allowed_senders",
		"tuning.hand.specialty_weights",
		"tuning.dealing.band_high",
		"tuning.dealing.band_low",
		"tuning.timers.prefetch_countdown",
		"tuning.timers.reveal_lockout",
		"tuning.timers.reveal_view",
		"tuning.timers.shuffle_bonus_seconds",
		"tuning.liquidity.backfill_enabled",
		"tuning.liquidity.bot_think_min_s",
		"tuning.liquidity.bot_think_max_s",
		"media.signed_url_ttl_s",
		"media.url_signing_key",
		"media.workbench_ingest_path",
	}
	var found []string
	if protocol, present := document["protocol"]; present {
		mapping, ok := protocol.(map[string]any)
		if !ok || mapping["version"] != 2 {
			found = append(found, "protocol.version (must be 2)")
		}
	}
	for _, path := range obsolete {
		var current any = document
		present := true
		for _, key := range strings.Split(path, ".") {
			mapping, ok := current.(map[string]any)
			if !ok {
				present = false
				break
			}
			current, present = mapping[key]
			if !present {
				break
			}
		}
		if present {
			found = append(found, path)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		return fmt.Errorf("config_migration_required: remove obsolete keys after consumer retirement: %s", strings.Join(found, ", "))
	}
	return nil
}
