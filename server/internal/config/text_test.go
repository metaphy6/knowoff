package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func textConfigSecrets(t *testing.T) {
	t.Helper()
	setRequiredSecrets(t)
	for _, name := range []string{"KNOWOFF_OAUTH_FACEBOOK_CLIENT_ID", "KNOWOFF_OAUTH_FACEBOOK_CLIENT_SECRET", "KNOWOFF_OAUTH_GOOGLE_CLIENT_ID", "KNOWOFF_OAUTH_GOOGLE_CLIENT_SECRET", "KNOWOFF_SSV_CALLBACK_KEY"} {
		t.Setenv(name, "test-only")
	}
}

func TestTextConfigDefaultsRemainUnavailable(t *testing.T) {
	textConfigSecrets(t)
	cfg, err := Load("../../../configs/base.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Protocol.Version != 1 || cfg.Text == nil || cfg.Text.Version != 2 || cfg.Text.DefaultMode != gamecontract.ModeMissedTheBriefing {
		t.Fatal("text contract must be present without switching live protocol")
	}
	if cfg.Text.Compatibility.ProtocolVersion != 2 || cfg.Text.Compatibility.MinClientGeneration != 2 || cfg.Text.RulesVersion == "" {
		t.Fatal("missing pinned compatibility contract")
	}
	if len(cfg.Text.Modes) != len(gamecontract.AllModes()) {
		t.Fatal("all five intended modes must be represented")
	}
	for _, mode := range gamecontract.AllModes() {
		availability, ok := cfg.Text.Modes[mode]
		if !ok || availability.Enabled || len(availability.ContentLanguages) != 0 {
			t.Fatalf("uncertified mode %s must stay unavailable", mode)
		}
	}
	if cfg.Tuning.Timers.TradeResponseS != 10 || cfg.Tuning.Timers.RoundStartCountdown != 5 || cfg.Tuning.Timers.PlayTurn != 20 || cfg.Tuning.Timers.KnowoffBallot != 20 || cfg.Tuning.Hand.Size != 5 || cfg.Tuning.Hand.DrawPile != 3 {
		t.Fatal("text trade timer must retain the existing clock and hand defaults")
	}
	if cfg.Tuning.Points.DrawPenalty != 5 || cfg.Tuning.Noin.DailyEarnCap != 300 || cfg.Tuning.Economy.FreeDailyQuickplayMatches != 3 {
		t.Fatal("contract setup must preserve economy defaults")
	}
	if cfg.Tuning.Contract.MaxHistoryEvents != 8192 || cfg.Tuning.Contract.MaxHistoryPageEvents != 8 || cfg.Tuning.Contract.MaxTextBytes != 512 || cfg.Tuning.Contract.MaxRequestsPerSeat != 512 {
		t.Fatal("missing explicit bounded contract budgets")
	}
}

func TestTextConfigRejectsInvalidOverlay(t *testing.T) {
	textConfigSecrets(t)
	tests := []struct{ name, overlay, want string }{
		{"version", "text:\n  version: 3\n", "text.version"},
		{"rules", "text:\n  rules_version: ''\n", "text.rules_version"},
		{"rules whitespace", "text:\n  rules_version: 'text v1'\n", "text.rules_version"},
		{"rules character", "text:\n  rules_version: 'text/v1'\n", "text.rules_version"},
		{"rules unicode", "text:\n  rules_version: 'text-ş'\n", "text.rules_version"},
		{"rules length", "text:\n  rules_version: '" + strings.Repeat("a", 129) + "'\n", "text.rules_version"},
		{"default", "text:\n  default_mode: image_game\n", "text.default_mode"},
		{"protocol", "text:\n  compatibility:\n    protocol_version: 1\n", "text.compatibility.protocol_version"},
		{"generation", "text:\n  compatibility:\n    min_client_generation: 0\n", "text.compatibility.min_client_generation"},
		{"missing modes", "text:\n  modes: null\n", "text.modes"},
		{"unknown mode", "text:\n  modes:\n    image_game: {enabled: false, content_languages: []}\n", "text.modes"},
		{"premature activation", "text:\n  modes:\n    secret_scale: {enabled: true, content_languages: [en]}\n", "not implemented"},
		{"invalid language", "text:\n  modes:\n    secret_scale: {content_languages: [en_US]}\n", "content_languages"},
		{"noncanonical language", "text:\n  modes:\n    secret_scale: {content_languages: [EN]}\n", "content_languages"},
		{"duplicate language", "text:\n  modes:\n    secret_scale: {content_languages: [tr, tr]}\n", "content_languages"},
		{"zero trade", "tuning:\n  timers:\n    trade_response_s: 0\n", "trade_response_s"},
		{"zero falling", "tuning:\n  timers:\n    vote_result_falling: 0\n", "vote_result_falling"},
		{"late falling", "tuning:\n  timers:\n    vote_result_falling: 8\n", "vote_result_falling"},
		{"negative falling", "tuning:\n  timers:\n    vote_result_falling: -1\n", "vote_result_falling"},
		{"negative trade", "tuning:\n  timers:\n    trade_response_s: -1\n", "trade_response_s"},
		{"unbounded trade", "tuning:\n  timers:\n    trade_response_s: 61\n", "trade_response_s"},
		{"unknown key", "text:\n  secret_catalog_url: bad\n", "unknown config keys"},
		{"history budget", "tuning:\n  contract:\n    max_history_events: 0\n", "tuning.contract"},
		{"page budget", "tuning:\n  contract:\n    max_history_page_events: 8193\n", "tuning.contract"},
		{"text budget", "tuning:\n  contract:\n    max_text_bytes: 65537\n", "tuning.contract"},
		{"request budget", "tuning:\n  contract:\n    max_requests_per_seat: 0\n", "tuning.contract"},
		{"round countdown", "tuning:\n  timers:\n    round_start_countdown: -1\n", "round_start_countdown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			overlay := writeTemp(t, "overlay.yaml", tc.overlay)
			_, err := Load("../../../configs/base.yaml", overlay)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestTextConfigLanguageIsIndependentOfUILocale(t *testing.T) {
	textConfigSecrets(t)
	overlay := writeTemp(t, "overlay.yaml", "text:\n  modes:\n    secret_scale: {content_languages: [tr, ar, en-GB]}\n")
	cfg, err := Load("../../../configs/base.yaml", overlay)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Localization.DefaultLocale != "en" || !reflect.DeepEqual(cfg.Text.Modes[gamecontract.ModeSecretScale].ContentLanguages, []string{"tr", "ar", "en-GB"}) {
		t.Fatal("content languages must not be inferred from interface locale")
	}
}

func TestTextCatalogBudgets(t *testing.T) {
	textConfigSecrets(t)
	cfg, err := Load("../../../configs/base.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tuning.TextCatalog.MaxRecords != 20000 || cfg.Tuning.TextCatalog.MaxFileBytes != 16777216 || cfg.Tuning.TextCatalog.MaxBundleBytes != 67108864 || cfg.Tuning.TextCatalog.MaxSearchNodes != 100000 {
		t.Fatal("missing explicit text catalog bounds")
	}
	for _, overlay := range []string{
		"max_records: 0", "max_file_bytes: 0", "max_bundle_bytes: 0", "max_search_nodes: 0", "max_file_bytes: 67108865",
	} {
		_, err := Load("../../../configs/base.yaml", writeTemp(t, "overlay.yaml", "tuning:\n  text_catalog:\n    "+overlay+"\n"))
		if err == nil || !strings.Contains(err.Error(), "text_catalog") {
			t.Fatalf("accepted invalid budget %s: %v", overlay, err)
		}
	}
}

func TestTextCutoverRejectsObsoleteKeysByPresence(t *testing.T) {
	for _, tc := range []struct{ yaml, path string }{
		{"tuning:\n  hand:\n    specialty_weights: {}\n", "tuning.hand.specialty_weights"},
		{"tuning:\n  timers:\n    prefetch_countdown: 0\n", "tuning.timers.prefetch_countdown"},
		{"tuning:\n  timers:\n    reveal_lockout: null\n", "tuning.timers.reveal_lockout"},
		{"tuning:\n  timers:\n    reveal_view: 0\n", "tuning.timers.reveal_view"},
		{"tuning:\n  timers:\n    shuffle_bonus_seconds: 0\n", "tuning.timers.shuffle_bonus_seconds"},
		{"tuning:\n  liquidity:\n    backfill_enabled: false\n", "tuning.liquidity.backfill_enabled"},
		{"tuning:\n  liquidity:\n    bot_think_min_s: 0\n", "tuning.liquidity.bot_think_min_s"},
		{"tuning:\n  liquidity:\n    bot_think_max_s: 0\n", "tuning.liquidity.bot_think_max_s"},
		{"media:\n  signed_url_ttl_s: 0\n", "media.signed_url_ttl_s"},
		{"media:\n  url_signing_key: ${PRIVATE_VALUE}\n", "media.url_signing_key"},
		{"media:\n  workbench_ingest_path: ''\n", "media.workbench_ingest_path"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			err := ValidateTextCutover([]byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), "config_migration_required") || !strings.Contains(err.Error(), tc.path) || strings.Contains(err.Error(), "PRIVATE_VALUE") {
				t.Fatalf("expected value-free obsolete-key error for %s, got %v", tc.path, err)
			}
		})
	}
	if err := ValidateTextCutover([]byte("tuning:\n  timers:\n    trade_response_s: 10\n    round_start_countdown: 5\n")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTextCutover([]byte("tuning: [malformed\n")); err == nil {
		t.Fatal("malformed preflight must fail")
	}
}
