package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func withoutEnvironment(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		t.Setenv(name, "") // Cleanup restores the original environment after Unsetenv.
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeConfigRequiresOnlyCoreSecrets(t *testing.T) {
	for _, name := range []string{"KNOWOFF_DB_PASSWORD", "KNOWOFF_REDIS_PASSWORD", "KNOWOFF_JWT_KEY"} {
		t.Setenv(name, "fixture-core-secret")
	}
	withoutEnvironment(t, "KNOWOFF_STORAGE_ACCESS_KEY", "KNOWOFF_STORAGE_SECRET_KEY", "KNOWOFF_MEDIA_URL_KEY", "KNOWOFF_SSV_CALLBACK_KEY", "KNOWOFF_OAUTH_GOOGLE_CLIENT_ID", "KNOWOFF_OAUTH_GOOGLE_CLIENT_SECRET", "KNOWOFF_OAUTH_FACEBOOK_CLIENT_ID", "KNOWOFF_OAUTH_FACEBOOK_CLIENT_SECRET", "KNOWOFF_DEV_BOT_KEY")
	for _, overlay := range []string{"", "../../../configs/local.yaml", "../../../configs/staging.yaml", "../../../configs/prod.yaml"} {
		t.Run(overlay, func(t *testing.T) {
			cfg, err := Load("../../../configs/base.yaml", overlay)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Protocol.Version != 2 || cfg.Text == nil || cfg.Text.Version != 2 {
				t.Fatal("text runtime must advertise protocol2")
			}
			if cfg.Security.OAuth.Google.ClientID != "" || cfg.Security.OAuth.Facebook.ClientID != "" {
				t.Fatal("obsolete runtime dependency or enabled provider in default configuration")
			}
		})
	}
	if !reflect.DeepEqual((&Config{}).RequiredSecrets(), []string{"KNOWOFF_DB_PASSWORD", "KNOWOFF_REDIS_PASSWORD", "KNOWOFF_JWT_KEY"}) {
		t.Fatal("core runtime secret inventory drifted")
	}
}

func TestRuntimeConfigRejectsRetiredInputBeforeInterpolation(t *testing.T) {
	textConfigSecrets(t)
	for _, tc := range []struct{ body, path string }{
		{"storage: {}\n", "storage"},
		{"storage: null\n", "storage"},
		{"media:\n  active_pack_tag: old\n", "media"},
		{"media:\n  url_signing_key: ${MUST_NOT_BE_READ}\n", "media"},
		{"bots: {enabled: false}\n", "bots"},
		{"security:\n  ssv_callback_key: PRIVATE_SENTINEL\n", "security.ssv_callback_key"},
		{"security:\n  ssv_allowed_senders: ''\n", "security.ssv_allowed_senders"},
		{"protocol: {version: 1}\n", "protocol.version"},
		{"protocol: {version: 0}\n", "protocol.version"},
		{"protocol: {version: 3}\n", "protocol.version"},
		{"tuning:\n  liquidity:\n    backfill_enabled: false\n", "tuning.liquidity.backfill_enabled"},
		{"tuning:\n  dealing:\n    band_high: ${MUST_NOT_BE_READ}\n", "tuning.dealing.band_high"},
		{"tuning:\n  dealing:\n    band_low: null\n", "tuning.dealing.band_low"},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			_, err := Load("../../../configs/base.yaml", writeTemp(t, "retired.yaml", tc.body))
			if err == nil || !strings.Contains(err.Error(), "config_migration_required") || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("retired runtime input did not fail explicitly: %v", err)
			}
			if strings.Contains(err.Error(), "PRIVATE_SENTINEL") || strings.Contains(err.Error(), "MUST_NOT_BE_READ") || strings.Contains(err.Error(), "missing secret") {
				t.Fatal("retired values were interpolated or exposed")
			}
		})
	}
}

func TestLiveConfigTypesExcludeRetiredConsumers(t *testing.T) {
	for _, tc := range []struct {
		value any
		names []string
	}{
		{Config{}, []string{"Storage", "Media", "Bots"}},
		{SecurityConfig{}, []string{"SSVCallbackKey", "SSVAllowedSenders"}},
		{TimersTuning{}, []string{"PrefetchCountdown"}},
		{DealingTuning{}, []string{"BandHigh", "BandLow"}},
		{LiquidityTuning{}, []string{"BackfillEnabled", "BotThinkMinS", "BotThinkMaxS"}},
	} {
		typ := reflect.TypeOf(tc.value)
		for _, name := range tc.names {
			if _, found := typ.FieldByName(name); found {
				t.Errorf("retired live field %s.%s remains", typ.Name(), name)
			}
		}
	}
}

func TestRestoredSpecialtyConfiguration(t *testing.T) {
	textConfigSecrets(t)
	cfg, err := Load("../../../configs/base.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	hand := reflect.ValueOf(cfg.Tuning.Hand).FieldByName("SpecialtyWeights")
	if !hand.IsValid() {
		t.Fatal("specialty weights were removed from live configuration")
	}
	weights := hand.Interface().(map[string]float64)
	for _, name := range []string{"pass", "reveal", "one_more_free_card", "shuffle", "revote"} {
		if weights[name] <= 0 {
			t.Fatalf("specialty %s not dealt", name)
		}
	}
	for _, name := range []string{"RevealLockout", "RevealView", "ShuffleBonusSeconds"} {
		value := reflect.ValueOf(cfg.Tuning.Timers).FieldByName(name)
		if !value.IsValid() || value.Int() <= 0 {
			t.Fatalf("specialty timer %s absent", name)
		}
	}
}

func TestSpecialtyConfigurationRejectsInvalidTuning(t *testing.T) {
	textConfigSecrets(t)
	for _, body := range []string{
		"tuning:\n  hand:\n    specialty_weights: {pass: -0.1}\n",
		"tuning:\n  hand:\n    specialty_weights: {unknown: 0.1}\n",
		"tuning:\n  timers:\n    reveal_view: 0\n",
		"tuning:\n  timers:\n    reveal_lockout: 99\n",
		"tuning:\n  timers:\n    shuffle_bonus_seconds: 61\n",
	} {
		if _, err := Load("../../../configs/base.yaml", writeTemp(t, "specialties.yaml", body)); err == nil {
			t.Fatalf("accepted invalid specialty tuning: %s", body)
		}
	}
}
