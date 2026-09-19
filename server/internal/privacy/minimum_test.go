package privacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func minimumFixture(t *testing.T) (FixtureJournalConfig, FixtureMinimumConfig, *FixtureMinimum) {
	t.Helper()
	cfg, _ := journalFixture(t)
	mc := FixtureMinimumConfig{Directory: filepath.Join(filepath.Dir(cfg.Directory), "minimum"), JournalDirectory: cfg.Directory, RestorableRoots: cfg.RestorableRoots, InstallationID: cfg.InstallationID, KeyID: cfg.KeyID, SigningKey: cfg.SigningKey}
	m, e := CreateFixtureMinimum(mc)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close() })
	cfg.Minimum = m
	return cfg, mc, m
}

func TestFixtureMinimumUncertainAdvanceNeverAcknowledgesEarly(t *testing.T) {
	cfg, mc, m := minimumFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	request := journalRequest()
	m.afterRename = func() error { return os.ErrPermission }
	if r, e := j.Publish(t.Context(), request); e == nil || r != (SuppressionReceipt{}) {
		t.Fatal("uncertain minimum acknowledged", r, e)
	}
	j.Close()
	m.Close()
	reopened, e := OpenFixtureMinimum(mc)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	cfg.Minimum = reopened
	j, e = OpenFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	r, e := j.Publish(t.Context(), request)
	if e != nil || r.Sequence != 1 {
		t.Fatal("retry lost identity", r, e)
	}
	stored, e := reopened.Load(t.Context())
	if e != nil || stored != r {
		t.Fatal("minimum receipt mismatch", stored, e)
	}
}

func TestFixtureMinimumRejectsWrongAuthorityAndMalformedState(t *testing.T) {
	_, mc, m := minimumFixture(t)
	for _, directory := range []string{mc.JournalDirectory, filepath.Join(mc.JournalDirectory, "nested"), filepath.Dir(mc.JournalDirectory), mc.RestorableRoots[0]} {
		bad := mc
		bad.Directory = directory
		if value, e := CreateFixtureMinimum(bad); e == nil {
			value.Close()
			t.Fatal("overlapping authority created")
		}
	}
	_, other, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*FixtureMinimumConfig){func(c *FixtureMinimumConfig) { c.InstallationID = uuid.NewString() }, func(c *FixtureMinimumConfig) { c.KeyID = "different" }, func(c *FixtureMinimumConfig) { c.SigningKey = other }} {
		bad := mc
		change(&bad)
		if value, e := OpenFixtureMinimum(bad); e == nil {
			value.Close()
			t.Fatal("wrong authority accepted")
		}
	}
	file := filepath.Join(mc.Directory, "state.json")
	original, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	for _, raw := range [][]byte{append(bytes.Clone(original), ' '), bytes.Repeat([]byte("x"), 8193), []byte(`{}`), bytes.Replace(original, []byte(`"body":`), []byte(`"extra":0,"body":`), 1)} {
		if e := os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := m.Load(t.Context()); e == nil {
			t.Fatal("malformed minimum accepted")
		}
		if e := os.WriteFile(file, original, 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, invalid := range []SuppressionReceipt{{Sequence: -1}, {Sequence: 1}, {Digest: [32]byte{1}}, {Sequence: journalMaxRecords + 1, Digest: [32]byte{1}}} {
		if e := m.Advance(t.Context(), invalid); e == nil {
			t.Fatal("invalid receipt accepted")
		}
	}
}

func TestFixtureMinimumCanceledLockCannotAdvance(t *testing.T) {
	_, _, m := minimumFixture(t)
	f, e := m.root.OpenFile("lock", os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e := lockJournal(t.Context(), f); e != nil {
		t.Fatal(e)
	}
	defer unlockJournal(f)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if e := m.Advance(ctx, SuppressionReceipt{Sequence: 1, Digest: [32]byte{1}}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("minimum lock ignored cancellation", e)
	}
}

func TestFixtureMinimumSurvivesReopenAndRefusesJournalRollback(t *testing.T) {
	cfg, _ := journalFixture(t)
	mc := FixtureMinimumConfig{Directory: filepath.Join(filepath.Dir(cfg.Directory), "minimum"), JournalDirectory: cfg.Directory, RestorableRoots: cfg.RestorableRoots, InstallationID: cfg.InstallationID, KeyID: cfg.KeyID, SigningKey: cfg.SigningKey}
	m, e := CreateFixtureMinimum(mc)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Minimum = m
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(filepath.Join(cfg.Directory, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := j.Publish(t.Context(), journalRequest())
	if e != nil {
		t.Fatal(e)
	}
	j.Close()
	m.Close()
	m, e = OpenFixtureMinimum(mc)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if got, e := m.Load(t.Context()); e != nil || got != r {
		t.Fatal("durable minimum lost", got, e)
	}
	if e := m.Advance(t.Context(), SuppressionReceipt{}); e == nil {
		t.Fatal("lowered minimum")
	}
	changed := r
	changed.Digest[0] ^= 1
	if e := m.Advance(t.Context(), changed); e == nil {
		t.Fatal("same sequence altered")
	}
	if e := os.WriteFile(filepath.Join(cfg.Directory, "state.json"), before, 0600); e != nil {
		t.Fatal(e)
	}
	cfg.Minimum = m
	if j, e := OpenFixtureJournal(cfg); e == nil {
		j.Close()
		t.Fatal("restored old journal bypassed independent file minimum")
	}
}
