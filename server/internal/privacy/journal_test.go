package privacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFixtureJournalConcurrentHandlesAndPrivateSelectors(t *testing.T) {
	cfg, _ := journalFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	other, e := OpenFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	request := journalRequest()
	results := make(chan SuppressionReceipt, 16)
	failures := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			handle := j
			if i%2 == 1 {
				handle = other
			}
			r, e := handle.Publish(t.Context(), request)
			results <- r
			failures <- e
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	var first SuppressionReceipt
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	for r := range results {
		if first.Sequence == 0 {
			first = r
		}
		if r != first || r.Sequence != 1 {
			t.Fatal("duplicate append", r)
		}
	}
	raw, e := os.ReadFile(filepath.Join(cfg.Directory, "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range [][]byte{[]byte(request.AccountID), cfg.SigningKey, cfg.SelectorKey} {
		if bytes.Contains(raw, secret) {
			t.Fatal("private source persisted")
		}
	}
	changed := request
	changed.AccountID = uuid.NewString()
	if _, e := j.Publish(t.Context(), changed); e == nil {
		t.Fatal("changed account accepted")
	}
	changed = request
	changed.VerifiedAt = changed.VerifiedAt.Add(-time.Microsecond)
	if _, e := j.Publish(t.Context(), changed); e == nil {
		t.Fatal("changed verification accepted")
	}
	changed = request
	changed.VerifiedAt = time.Now().Add(time.Hour)
	if _, e := j.Publish(t.Context(), changed); e == nil {
		t.Fatal("future verification accepted")
	}
	if after, e := os.ReadFile(filepath.Join(cfg.Directory, "state.json")); e != nil || !bytes.Equal(raw, after) {
		t.Fatal("refusal mutated journal", e)
	}
}

func TestFixtureJournalUncertainAppendReopensAndAdvancesMinimum(t *testing.T) {
	cfg, minimum := journalFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	request := journalRequest()
	j.afterRename = func() error { return os.ErrPermission }
	if r, e := j.Publish(t.Context(), request); e == nil || r != (SuppressionReceipt{}) {
		t.Fatal("uncertain append acknowledged", r, e)
	}
	j.Close()
	if minimum.r.Sequence != 0 {
		t.Fatal("failed sync advanced minimum")
	}
	j, e = OpenFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	minimum.fail = true
	if r, e := j.Publish(t.Context(), request); e == nil || r != (SuppressionReceipt{}) {
		t.Fatal("minimum failure acknowledged", r, e)
	}
	minimum.fail = false
	r, e := j.Publish(t.Context(), request)
	if e != nil || r.Sequence != 1 || minimum.r != r {
		t.Fatal("uncertain recovery", r, e)
	}
	if _, e := j.Publish(t.Context(), journalRequest()); e != nil {
		t.Fatal(e)
	}
	if again, e := j.Publish(t.Context(), request); e != nil || again != r || minimum.r.Sequence != 2 {
		t.Fatal("old exact replay lowered minimum", again, e)
	}
}

func TestFixtureJournalRejectsCorruptionAndIndependentRollback(t *testing.T) {
	cfg, _ := journalFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	file := filepath.Join(cfg.Directory, "state.json")
	empty, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := j.Publish(t.Context(), journalRequest()); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var state fixtureState
	if json.Unmarshal(raw, &state) != nil {
		t.Fatal("state")
	}
	mutations := map[string][]byte{"rollback": empty, "trailing": append(bytes.Clone(raw), ' '), "unknown": bytes.Replace(raw, []byte(`"version":1`), []byte(`"unknown":0,"version":1`), 1), "duplicate": bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), "oversize": bytes.Repeat([]byte("x"), journalMaxBytes+1)}
	state.Records[0].Body.Selector = "a" + state.Records[0].Body.Selector[1:]
	state.Records[0].Signature[0] ^= 1
	mutations["signature"] = canonical(state)
	for name, b := range mutations {
		t.Run(name, func(t *testing.T) {
			if e := os.WriteFile(file, b, 0600); e != nil {
				t.Fatal(e)
			}
			if opened, e := OpenFixtureJournal(cfg); e == nil {
				opened.Close()
				t.Fatal("corruption accepted")
			}
			if _, e := j.Publish(t.Context(), journalRequest()); e == nil {
				t.Fatal("wrote over corrupted state")
			}
			if e := os.WriteFile(file, raw, 0600); e != nil {
				t.Fatal(e)
			}
		})
	}
	wrong := cfg
	wrong.SelectorKey = bytes.Repeat([]byte{4}, 32)
	if opened, e := OpenFixtureJournal(wrong); e == nil {
		opened.Close()
		t.Fatal("wrong selector key accepted")
	}
}

func TestFixtureJournalFreshHeadCannotBeReplayedOrForged(t *testing.T) {
	cfg, min := journalFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	if _, e := j.Publish(t.Context(), journalRequest()); e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	j.now = func() time.Time { return at }
	challenge := bytes.Repeat([]byte{7}, 32)
	snapshot, e := j.Snapshot(t.Context(), challenge)
	if e != nil {
		t.Fatal(e)
	}
	pub := cfg.SigningKey.Public().(ed25519.PublicKey)
	for _, clock := range []time.Time{at, at.Add(time.Minute)} {
		if e := VerifyFixtureSnapshot(snapshot, pub, cfg.InstallationID, cfg.KeyID, challenge, min.r, clock); e != nil {
			t.Fatal(e)
		}
	}
	for _, clock := range []time.Time{at.Add(-time.Microsecond), at.Add(time.Minute + time.Microsecond)} {
		if e := VerifyFixtureSnapshot(snapshot, pub, cfg.InstallationID, cfg.KeyID, challenge, min.r, clock); e == nil {
			t.Fatal("stale or future head accepted")
		}
	}
	if e := VerifyFixtureSnapshot(snapshot, pub, cfg.InstallationID, cfg.KeyID, bytes.Repeat([]byte{8}, 32), min.r, at); e == nil {
		t.Fatal("old challenge accepted")
	}
	if e := VerifyFixtureSnapshot(snapshot, pub, uuid.NewString(), cfg.KeyID, challenge, min.r, at); e == nil {
		t.Fatal("wrong installation")
	}
	wrongMin := min.r
	wrongMin.Digest[0] ^= 1
	if e := VerifyFixtureSnapshot(snapshot, pub, cfg.InstallationID, cfg.KeyID, challenge, wrongMin, at); e == nil {
		t.Fatal("wrong trusted minimum")
	}
	snapshot.Head.Sequence++
	if e := VerifyFixtureSnapshot(snapshot, pub, cfg.InstallationID, cfg.KeyID, challenge, min.r, at); e == nil {
		t.Fatal("forged head")
	}
}

func TestFixtureJournalCancellationAndFilesystemBoundaries(t *testing.T) {
	cfg, _ := journalFixture(t)
	for _, backup := range []string{cfg.Directory, filepath.Dir(cfg.Directory), filepath.Join(cfg.Directory, "nested")} {
		bad := cfg
		bad.RestorableRoots = []string{backup}
		if j, e := CreateFixtureJournal(bad); e == nil {
			j.Close()
			t.Fatal("restorable overlap accepted")
		}
	}
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	lock, e := j.root.OpenFile("lock", os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if e := lockJournal(t.Context(), lock); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, e := j.Publish(ctx, journalRequest()); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("lock wait ignored cancellation", e)
	}
	unlockJournal(lock)
	file := filepath.Join(cfg.Directory, "state.json")
	if e := os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := j.Publish(t.Context(), journalRequest()); e == nil {
		t.Fatal("public state accepted")
	}
	if e := os.Chmod(file, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(file, file+".saved"); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("state.json.saved", file); e != nil {
		t.Fatal(e)
	}
	if _, e := j.Publish(t.Context(), journalRequest()); e == nil {
		t.Fatal("symlink state accepted")
	}
}

// This test double supplies a separate trusted minimum. It is not production
// durable storage; filesystem/restart deployment proof remains a separate gate.
type testMinimum struct {
	mu   sync.Mutex
	r    SuppressionReceipt
	fail bool
}

func (s *testMinimum) Load(context.Context) (SuppressionReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r, nil
}
func (s *testMinimum) Advance(_ context.Context, r SuppressionReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return os.ErrPermission
	}
	if r.Sequence < s.r.Sequence || r.Sequence == s.r.Sequence && r.Digest != s.r.Digest {
		return os.ErrInvalid
	}
	s.r = r
	return nil
}

func journalFixture(t *testing.T) (FixtureJournalConfig, *testMinimum) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	minimum := &testMinimum{}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	selector := make([]byte, 32)
	if _, err := rand.Read(selector); err != nil {
		t.Fatal(err)
	}
	return FixtureJournalConfig{Directory: filepath.Join(base, "journal"), RestorableRoots: []string{filepath.Join(base, "backup")}, InstallationID: uuid.NewString(), KeyID: "fixture-key-1", SigningKey: key, SelectorKey: selector, Minimum: minimum}, minimum
}
func journalRequest() SuppressionRequest {
	return SuppressionRequest{RequestID: uuid.NewString(), AccountID: uuid.NewString(), VerifiedAt: time.Now().UTC().Truncate(time.Microsecond), PolicyVersion: "deletion-2026-09-19"}
}

func TestFixtureJournalPublishReplayAndFreshSnapshot(t *testing.T) {
	cfg, minimum := journalFixture(t)
	j, err := CreateFixtureJournal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	request := journalRequest()
	r, err := j.Publish(t.Context(), request)
	if err != nil || r.Sequence != 1 {
		t.Fatal(r, err)
	}
	again, err := j.Publish(t.Context(), request)
	if err != nil || again != r {
		t.Fatal("retry changed receipt", again, err)
	}
	if minimum.r != r {
		t.Fatal("acknowledged before minimum advanced")
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		t.Fatal(err)
	}
	snapshot, err := j.Snapshot(t.Context(), challenge)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFixtureSnapshot(snapshot, cfg.SigningKey.Public().(ed25519.PublicKey), cfg.InstallationID, cfg.KeyID, challenge, r, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestFixtureJournalRefusesSubMicrosecondIdentity(t *testing.T) {
	cfg, _ := journalFixture(t)
	j, e := CreateFixtureJournal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer j.Close()
	request := journalRequest()
	request.VerifiedAt = request.VerifiedAt.Add(-time.Second + time.Nanosecond)
	if _, e := j.Publish(t.Context(), request); e == nil {
		t.Fatal("lossy timestamp identity accepted")
	}
}
