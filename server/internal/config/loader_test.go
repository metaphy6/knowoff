package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp %s: %v", name, err)
	}
	return p
}

func setRequiredSecrets(t *testing.T) {
	t.Helper()
	for _, k := range (&Config{}).RequiredSecrets() {
		t.Setenv(k, "secret-"+strings.ToLower(k))
	}
}

func TestLoad_DefaultBaseWithLocalOverlay(t *testing.T) {
	setRequiredSecrets(t)
	base := writeTemp(t, "base.yaml", `
app:
  name: knowoffd
  version: 0.1.0
  env: local
server:
  port: 8080
  admin_port: 9090
  metrics_port: 9091
  shutdown_grace_s: 15
database:
  host: postgres
  port: 5432
  name: knowoff
  user: knowoff
  password: ${KNOWOFF_DB_PASSWORD}
redis:
  addr: redis:6379
  password: ${KNOWOFF_REDIS_PASSWORD}
storage:
  access_key_id: ${KNOWOFF_STORAGE_ACCESS_KEY}
  secret_access_key: ${KNOWOFF_STORAGE_SECRET_KEY}
security:
  jwt_signing_key: ${KNOWOFF_JWT_KEY}
localization:
  default_locale: en
  supported_locales: [en]
`)
	overlay := writeTemp(t, "local.yaml", `
app:
  env: local
log:
  level: debug
`)

	cfg, err := Load(base, overlay)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.App.Env != "local" {
		t.Errorf("expected env local, got %q", cfg.App.Env)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug from overlay, got %q", cfg.Log.Level)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingSecretsReportedTogether(t *testing.T) {
	base := writeTemp(t, "base.yaml", `
app:
  name: knowoffd
server:
  port: 8080
  admin_port: 9090
  metrics_port: 9091
database:
  host: postgres
  password: ${KNOWOFF_DB_PASSWORD}
redis:
  password: ${KNOWOFF_REDIS_PASSWORD}
storage:
  access_key_id: ${KNOWOFF_STORAGE_ACCESS_KEY}
  secret_access_key: ${KNOWOFF_STORAGE_SECRET_KEY}
security:
  jwt_signing_key: ${KNOWOFF_JWT_KEY}
media:
  url_signing_key: ${KNOWOFF_MEDIA_URL_KEY}
`)
	// Clear all required secrets to ensure a single error lists every missing one.
	for _, k := range (&Config{}).RequiredSecrets() {
		os.Unsetenv(k)
	}

	_, err := Load(base, "")
	if err == nil {
		t.Fatal("expected error for missing secrets")
	}
	msg := err.Error()
	for _, k := range (&Config{}).RequiredSecrets() {
		if !strings.Contains(msg, k) {
			t.Errorf("error message should mention %s, got: %s", k, msg)
		}
	}
}

func TestLoad_UnknownKeyRejected(t *testing.T) {
	setRequiredSecrets(t)
	base := writeTemp(t, "base.yaml", `
app:
  name: knowoffd
server:
  port: 8080
  admin_port: 9090
  metrics_port: 9091
database:
  host: postgres
  password: ${KNOWOFF_DB_PASSWORD}
redis:
  password: ${KNOWOFF_REDIS_PASSWORD}
storage:
  access_key_id: ${KNOWOFF_STORAGE_ACCESS_KEY}
  secret_access_key: ${KNOWOFF_STORAGE_SECRET_KEY}
security:
  jwt_signing_key: ${KNOWOFF_JWT_KEY}
`)
	overlay := writeTemp(t, "bad.yaml", `
typo_section:
  value: 1
`)

	_, err := Load(base, overlay)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected unknown-key error, got: %v", err)
	}
}

func TestLoad_InvalidValueReported(t *testing.T) {
	setRequiredSecrets(t)
	base := writeTemp(t, "base.yaml", `
app:
  name: knowoffd
server:
  port: 0
  admin_port: 9090
  metrics_port: 9091
database:
  host: postgres
  password: ${KNOWOFF_DB_PASSWORD}
redis:
  password: ${KNOWOFF_REDIS_PASSWORD}
storage:
  access_key_id: ${KNOWOFF_STORAGE_ACCESS_KEY}
  secret_access_key: ${KNOWOFF_STORAGE_SECRET_KEY}
security:
  jwt_signing_key: ${KNOWOFF_JWT_KEY}
`)

	_, err := Load(base, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "server.port") {
		t.Errorf("expected server.port validation error, got: %v", err)
	}
}

func TestInterpolateEnvVars(t *testing.T) {
	t.Setenv("TEST_VAR_A", "alpha")
	t.Setenv("TEST_VAR_B", "beta")
	out, missing := interpolateEnvVars("${TEST_VAR_A} and ${TEST_VAR_B} and ${TEST_VAR_MISSING}")
	if out != "alpha and beta and ${TEST_VAR_MISSING}" {
		t.Errorf("unexpected interpolation result: %q", out)
	}
	if len(missing) != 1 || missing[0] != "TEST_VAR_MISSING" {
		t.Errorf("expected missing [TEST_VAR_MISSING], got %v", missing)
	}
}

func TestContentScreeningConfigLoadsOptionalServerKey(t *testing.T) {
	setRequiredSecrets(t)
	for _, name := range []string{"KNOWOFF_OAUTH_FACEBOOK_CLIENT_ID", "KNOWOFF_OAUTH_FACEBOOK_CLIENT_SECRET", "KNOWOFF_OAUTH_GOOGLE_CLIENT_ID", "KNOWOFF_OAUTH_GOOGLE_CLIENT_SECRET", "KNOWOFF_SSV_CALLBACK_KEY"} {
		t.Setenv(name, "test-only")
	}
	cfg, err := Load("../../../configs/base.yaml", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Moderation.ContentScreening.Provider != "disabled" || cfg.Moderation.ContentScreening.APIKey != "" {
		t.Fatal("screening must be disabled without credentials")
	}
	t.Setenv("KNOWOFF_TEST_SCREENING_KEY", "test-only-key")
	overlay := writeTemp(t, "screening.yaml", `moderation:
  content_screening:
    provider: openai
    api_key: ${KNOWOFF_TEST_SCREENING_KEY}
`)
	cfg, err = Load("../../../configs/base.yaml", overlay)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Moderation.ContentScreening.APIKey != "test-only-key" || cfg.Moderation.ContentScreening.Model != "omni-moderation-latest" || cfg.Moderation.ContentScreening.TimeoutS != 10 {
		t.Fatal("screening config did not merge correctly")
	}
}

func TestContentScreeningConfigRejectsInvalidSettings(t *testing.T) {
	for _, cfg := range []ContentScreeningConfig{{Provider: "unknown"}, {Provider: "openai"}, {TimeoutS: 31}, {TimeoutS: -1}} {
		c := &Config{}
		c.Moderation.ContentScreening = cfg
		if !strings.Contains(strings.Join(validate(c), ";"), "moderation.content_screening") {
			t.Errorf("invalid screening config accepted: %+v", cfg)
		}
	}
}
