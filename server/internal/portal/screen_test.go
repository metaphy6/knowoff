package portal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

func TestScreeningFailsClosedWithoutProvider(t *testing.T) {
	for _, cfg := range []config.ContentScreeningConfig{{}, {Provider: "disabled"}, {Provider: "openai", Model: "omni-moderation-latest"}} {
		m := NewManager(Deps{Screener: NewTextScreener(cfg)})
		if !errors.Is(m.screenText(t.Context(), "A safe text"), ErrScreeningUnavailable) {
			t.Fatal("missing configured service allowed approval")
		}
	}
}

func TestOpenAITextScreening(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"accepted", `{"results":[{"flagged":false}]}`, 200, nil},
		{"flagged", `{"results":[{"flagged":true}]}`, 200, ErrContentFlagged},
		{"partly flagged", `{"results":[{"flagged":false},{"flagged":true}]}`, 200, ErrContentFlagged},
		{"empty", `{"results":[]}`, 200, ErrScreeningUnavailable},
		{"missing decision", `{"results":[{}]}`, 200, ErrScreeningUnavailable},
		{"invalid", `not json`, 200, ErrScreeningUnavailable},
		{"trailing", `{"results":[{"flagged":false}]} {}`, 200, ErrScreeningUnavailable},
		{"rate limit", `provider response with sensitive details`, 429, ErrScreeningUnavailable},
		{"redirect", "", 302, ErrScreeningUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-only-key" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("incorrect moderation request")
				}
				var payload struct{ Model, Input string }
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Model != "omni-moderation-latest" || payload.Input != "A safe text" {
					t.Errorf("unexpected body %+v", payload)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			s := NewTextScreener(config.ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "test-only-key", TimeoutS: 1}).(*openAITextScreener)
			s.endpoint = srv.URL
			err := s.ScreenText(t.Context(), "A safe text")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if err != nil && strings.Contains(err.Error(), "sensitive") {
				t.Fatal("upstream body leaked")
			}
		})
	}
}

func TestTextScreeningBoundsAndCancellation(t *testing.T) {
	calls := 0
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; <-release }))
	defer func() { close(release); srv.Close() }()
	s := NewTextScreener(config.ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "test-only-key", TimeoutS: 1}).(*openAITextScreener)
	s.endpoint = srv.URL
	for _, input := range []string{"", strings.Repeat("x", maxScreenTextBytes+1)} {
		if err := s.ScreenText(t.Context(), input); !errors.Is(err, ErrScreeningUnavailable) {
			t.Fatal("invalid input allowed")
		}
	}
	if calls != 0 {
		t.Fatal("invalid input reached provider")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := s.ScreenText(ctx, "Text"); !errors.Is(err, ErrScreeningUnavailable) {
		t.Fatalf("canceled request=%v", err)
	}
}

type screenTextFunc func(context.Context, string) error

func (f screenTextFunc) ScreenText(ctx context.Context, text string) error { return f(ctx, text) }

func legacyTopicForScreenTest(t *testing.T, m *Manager) (string, string) {
	t.Helper()
	account := newAccount(t, m.db)
	var id string
	err := m.db.QueryRow(`INSERT INTO portal_submissions(account_id,media_type,content,status,terms_version,terms_accepted_at) VALUES($1,'text','A legacy approved topic','approved','v1',now()) RETURNING id`, account).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id, account
}

func TestChallengeTopicScreeningFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		screener TextScreener
		want     error
	}{
		{"missing", nil, ErrScreeningUnavailable}, {"flagged", screenTextFunc(func(context.Context, string) error { return ErrContentFlagged }), ErrContentFlagged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m := newTestManager(t, db)
			admin := newAdmin(t, db)
			source, owner := legacyTopicForScreenTest(t, m)
			m.screener = tc.screener
			if topic, err := m.CreateChallengeTopic(t.Context(), admin, weekMonday(time.Now().UTC()), source); !errors.Is(err, tc.want) || topic != nil {
				t.Fatalf("screened topic=%v error=%v", topic, err)
			}
			var topics, audits, rewards int
			if err := db.QueryRow(`SELECT count(*) FROM challenge_topics`).Scan(&topics); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='challenge_topic_create' AND after_state->>'nown_media_id'=$1`, source).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, owner).Scan(&rewards); err != nil {
				t.Fatal(err)
			}
			if topics != 0 || audits != 0 || rewards != 0 {
				t.Fatalf("failed screen mutated state: topics=%d audits=%d rewards=%d", topics, audits, rewards)
			}
			if snapshot, err := m.ChallengeSnapshot(t.Context(), owner); err != nil || snapshot != nil {
				t.Fatalf("blocked topic became public: %v %v", snapshot, err)
			}
		})
	}
}

func TestChallengeEntryScreeningFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		screener TextScreener
		want     error
	}{
		{"missing", nil, ErrScreeningUnavailable}, {"flagged", screenTextFunc(func(context.Context, string) error { return ErrContentFlagged }), ErrContentFlagged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			m := newTestManager(t, db)
			admin := newAdmin(t, db)
			source, _ := legacyTopicForScreenTest(t, m)
			topic, err := m.CreateChallengeTopic(t.Context(), admin, weekMonday(time.Now().UTC()), source)
			if err != nil {
				t.Fatal(err)
			}
			owner := newAccount(t, db)
			viewer := newAccount(t, db)
			entry, err := m.SubmitChallengeEntry(t.Context(), owner, topic.ID, MediaText, "Pending private entry", ContributionConsent{Version: "v1", Accepted: true})
			if err != nil {
				t.Fatal(err)
			}
			m.screener = tc.screener
			if err = m.ApproveChallengeEntry(t.Context(), admin, entry.ID); !errors.Is(err, tc.want) {
				t.Fatalf("approval error=%v", err)
			}
			var status string
			var decision sql.NullTime
			var audits int
			if err = db.QueryRow(`SELECT status,screen_decided_at FROM challenge_entries WHERE id=$1`, entry.ID).Scan(&status, &decision); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='challenge_entry_approved' AND target_id=$1`, entry.ID).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if status != "submitted" || decision.Valid || audits != 0 {
				t.Fatalf("failed approval mutated entry: %s %v audits%d", status, decision, audits)
			}
			snapshot, err := m.ChallengeSnapshot(t.Context(), viewer)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot["entries"].([]map[string]any)) != 0 {
				t.Fatal("failed screening leaked public entry")
			}
			winner, err := m.CloseChallengeWeek(t.Context(), admin, topic.ID)
			if err != nil || winner != nil {
				t.Fatalf("failed screening won: %v %v", winner, err)
			}
			balance, err := m.economy.Wallet.Balance(t.Context(), owner)
			if err != nil || balance != 0 {
				t.Fatalf("failed screening paid: %d %v", balance, err)
			}
			var rewards int
			if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, owner).Scan(&rewards); err != nil {
				t.Fatal(err)
			}
			if rewards != 0 {
				t.Fatalf("unapproved entry created %d rewards", rewards)
			}
		})
	}
}

func TestChallengeScreeningChecksExactPublishedText(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	// Deliberately simulate privileged database corruption in this disposable
	// fixture. Ordinary submitted/reviewed writes are separately rejected by SQL;
	// the screening path must still detect a changed source on its own.
	if _, err := db.Exec(`ALTER TABLE portal_submissions DISABLE TRIGGER text_review_identity; ALTER TABLE challenge_entries DISABLE TRIGGER text_review_identity`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`ALTER TABLE portal_submissions ENABLE TRIGGER text_review_identity; ALTER TABLE challenge_entries ENABLE TRIGGER text_review_identity`); err != nil {t.Error(err)}
	}()
	m := newTestManager(t, db)
	admin := newAdmin(t, db)
	source, _ := legacyTopicForScreenTest(t, m)
	var seen []string
	m.screener = screenTextFunc(func(ctx context.Context, text string) error {
		seen = append(seen, text)
		_, err := db.ExecContext(ctx, `UPDATE portal_submissions SET content='Changed after screening' WHERE id=$1`, source)
		return err
	})
	if topic, err := m.CreateChallengeTopic(t.Context(), admin, weekMonday(time.Now().UTC()), source); err == nil || topic != nil {
		t.Fatal("changed source inherited earlier approval")
	}
	m.screener = screenTextFunc(func(ctx context.Context, text string) error { seen = append(seen, text); return nil })
	topic, err := m.CreateChallengeTopic(t.Context(), admin, weekMonday(time.Now().UTC()), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "A legacy approved topic" || seen[1] != "Changed after screening" {
		t.Fatalf("legacy topic not rescreened: %v", seen)
	}
	owner := newAccount(t, db)
	entry, err := m.SubmitChallengeEntry(t.Context(), owner, topic.ID, MediaText, "Original entry", ContributionConsent{Version: "v1", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	m.screener = screenTextFunc(func(ctx context.Context, text string) error {
		if text != "Original entry" {
			t.Error("screened wrong entry")
		}
		_, err := db.ExecContext(ctx, `UPDATE challenge_entries SET content='Replaced entry' WHERE id=$1`, entry.ID)
		return err
	})
	if err := m.ApproveChallengeEntry(t.Context(), admin, entry.ID); err == nil {
		t.Fatal("changed entry inherited earlier approval")
	}
	got, err := m.GetChallengeEntry(t.Context(), entry.ID)
	if err != nil || got.Status != EntrySubmitted {
		t.Fatalf("changed entry approved: %v %v", got, err)
	}
}
