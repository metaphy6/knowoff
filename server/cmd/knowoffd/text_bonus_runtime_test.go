package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/transport"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestTextBonusRecoveryBoundedBackoffAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var waits []time.Duration
	calls := 0
	runTextBonusRecovery(ctx, func(context.Context) error { return nil }, func(work context.Context, limit int) (int, error) {
		calls++
		deadline, ok := work.Deadline()
		if limit != 100 || !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("unbounded pass")
		}
		if calls <= 6 {
			return 0, errors.New("retry")
		}
		return 0, nil
	}, func(ctx context.Context, d time.Duration) bool {
		waits = append(waits, d)
		if len(waits) == 8 {
			cancel()
			return false
		}
		return true
	}, nil)
	if !reflect.DeepEqual(waits, []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, time.Second, time.Second}) {
		t.Fatal(waits)
	}
	if calls != 8 {
		t.Fatal(calls)
	}
}

// Existing public value APIs produce the durable completed source before the
// process worker starts; no fixture invents payment, proof or delivery rows.
func bonusRuntimeMatch(t *testing.T, db *sql.DB, cfg *config.Config, premium bool) (string, []auth.TokenPair, *auth.Manager) {
	t.Helper()
	am := auth.NewManager(db, []byte("disposable-bonus-runtime-key"), "bonus-runtime", "bonus-client", time.Hour, 24*time.Hour, auth.OAuthProviders{})
	values := store.NewTextValueStore(db, cfg.Tuning)
	at := time.Now().UTC().Truncate(time.Microsecond)
	pairs := []auth.TokenPair{}
	admissions := []string{}
	for range 4 {
		pair, err := am.AuthenticateDevice(t.Context(), "bonus-runtime-"+uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		admission := uuid.NewString()
		if err = values.Reserve(t.Context(), store.TextReservation{ID: admission, AccountID: pair.AccountID, EntryPath: "quick_play", At: at}); err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, *pair)
		admissions = append(admissions, admission)
	}
	hash, err := cfg.Tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	m := store.TextMatchRecord{Contract: v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: gamecontract.ModeMissedTheBriefing, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: "fixture", PackSHA256: strings.Repeat("a", 64), Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: true, Leaderboard: true}}, Owner: uuid.NewString(), Epoch: 1, AdmissionIDs: admissions}
	if err = values.Prepare(t.Context(), m, at); err != nil {
		t.Fatal(err)
	}
	if premium {
		if _, err = db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, pairs[0].AccountID); err != nil {
			t.Fatal(err)
		}
	}
	if err = values.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	o := store.TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(time.Microsecond)}
	for i, pair := range pairs {
		role := "nower"
		if i == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, store.TextPlayerResult{AccountID: pair.AccountID, Seat: i, Role: role})
	}
	if err = values.Finish(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err = values.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	return m.Contract.MatchID, pairs, am
}

type bonusRuntimeTransport func(*http.Request) (*http.Response, error)

func (f bonusRuntimeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func bonusRuntimeRequest(t *testing.T, server *httptest.Server, token, path, body string) (int, []byte) {
	t.Helper()
	method := http.MethodPost
	if strings.HasPrefix(path, "/v2/rewards/ssv?") {
		method = http.MethodGet
	}
	r, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func waitBonusPayment(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM text_bonus_payments`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("payment not recovered")
		case <-tick.C:
		}
	}
}

func TestTextBonusRuntimeJoinedPremiumAndVerifiedSSVRestart(t *testing.T) {
	for _, premium := range []bool{true, false} {
		t.Run(fmt.Sprint("premium=", premium), func(t *testing.T) {
			db, cfg := textRuntimeProofDB(t)
			match, pairs, am := bonusRuntimeMatch(t, db, cfg, premium)
			rt, err := newTextRuntime(t.Context(), db, cfg, "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { rt.Close(context.Background()) })
			adcfg := config.RewardedConfig{}
			var key *ecdsa.PrivateKey
			var keys http.RoundTripper
			if !premium {
				adcfg = config.RewardedConfig{Enabled: true, MaxQueryBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 2, KeyCacheS: 60, KeyRefreshMinS: 1, MaxConcurrentRequests: 4, ClaimTTLS: 60, MaxClaimsPerMatchWindow: 2, AdUnits: map[string]config.RewardedUnit{"123": {RewardItem: "match_bonus", RewardAmount: 1}}}
				key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(map[string]any{"keys": []map[string]any{{"keyId": 7, "base64": base64.StdEncoding.EncodeToString(pub)}}})
				keys = bonusRuntimeTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				})
			}
			h, err := newTextRewardHTTP(adcfg, db, am, rt.Values, keys)
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			h.register(mux)
			server := httptest.NewServer(mux)
			defer server.Close()
			query := ""
			if premium {
				if h.Rewarded != nil {
					t.Fatal("ads constructed without configuration")
				}
				if status, _ := bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/claim", `{}`); status != 404 {
					t.Fatal(status)
				}
			} else {
				status, raw := bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/claim", fmt.Sprintf(`{"match_id":%q,"ad_unit":"123"}`, match))
				var claim store.TextRewardClaim
				if status != 200 || json.Unmarshal(raw, &claim) != nil || claim.AdUnit != "123" || !claim.ExpiresAt.After(time.Now()) {
					t.Fatal("claim authority", status, string(raw))
				}
				body := fmt.Sprintf("ad_network=5450213213286189855&ad_unit=123&custom_data=%s&reward_amount=1&reward_item=match_bonus&timestamp=%d&transaction_id=abcdef", claim.Claim, time.Now().UnixMilli())
				digest := sha256.Sum256([]byte(body))
				sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
				if err != nil {
					t.Fatal(err)
				}
				query = body + "&signature=" + base64.RawURLEncoding.EncodeToString(sig) + "&key_id=7"
				if status, _ = bonusRuntimeRequest(t, server, "", "/v2/rewards/ssv?"+strings.Replace(query, "reward_amount=1", "reward_amount=999", 1), ""); status != 400 {
					t.Fatal("client amount granted", status)
				}
				for range 2 {
					if status, _ = bonusRuntimeRequest(t, server, "", "/v2/rewards/ssv?"+query, ""); status != 200 {
						t.Fatal("signed receipt", status)
					}
				}
				var n int
				if err = db.QueryRow(`SELECT count(*) FROM text_bonus_payments`).Scan(&n); err != nil || n != 0 {
					t.Fatal("callback directly credited", n, err)
				}
				// Stop after durable verification, before any worker paid it.
				if err = rt.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
				rt, err = newTextRuntime(t.Context(), db, cfg, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			gate := transport.NewWorkGate()
			defer gate.Close()
			if !rt.startBonusRecovery(t.Context(), gate, nil) || rt.startBonusRecovery(t.Context(), gate, nil) {
				t.Fatal("worker launch identity")
			}
			waitBonusPayment(t, db)
			status, raw := bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/bonuses/claim", `{"limit":20,"pending_delivery_ids":[]}`)
			var page store.TextBonusDeliveryPage
			if status != 200 || json.Unmarshal(raw, &page) != nil || len(page.Deliveries) != 1 {
				t.Fatal("private payment", status, string(raw))
			}
			d := page.Deliveries[0]
			if d.Payload.Credited <= 0 || d.Payload.MatchID != match {
				t.Fatal("payment not genuine", d.Payload)
			}
			if status, _ = bonusRuntimeRequest(t, server, pairs[1].AccessToken, "/v2/rewards/bonuses/ack", fmt.Sprintf(`{"delivery_id":%q,"lease":%q}`, d.DeliveryID, d.Lease)); status != 409 {
				t.Fatal("foreign ack", status)
			}
			if status, _ = bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/bonuses/ack", fmt.Sprintf(`{"delivery_id":%q,"lease":%q}`, d.DeliveryID, d.Lease)); status != 200 {
				t.Fatal("ack", status)
			}
			status, raw = bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/bonuses/claim", fmt.Sprintf(`{"limit":20,"pending_delivery_ids":[%q]}`, d.DeliveryID))
			if status != 200 || json.Unmarshal(raw, &page) != nil || len(page.AcknowledgedIDs) != 1 || page.AcknowledgedIDs[0] != d.DeliveryID {
				t.Fatal("lost ack recovery", status)
			}
			if err = am.RevokeSessions(t.Context(), pairs[0].AccountID); err != nil {
				t.Fatal(err)
			}
			if status, _ = bonusRuntimeRequest(t, server, pairs[0].AccessToken, "/v2/rewards/bonuses/claim", `{"limit":20,"pending_delivery_ids":[]}`); status != 401 {
				t.Fatal("revoked bearer", status)
			}
			if err = rt.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
			gate.Close()
			if err = gate.Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			var payments, outbox int
			if err = db.QueryRow(`SELECT (SELECT count(*) FROM text_bonus_payments),(SELECT count(*) FROM text_bonus_outbox)`).Scan(&payments, &outbox); err != nil || payments != 1 || outbox != 1 {
				t.Fatal("duplicate payment", payments, outbox, err)
			}
		})
	}
}

func TestTextBonusRuntimeDefaultAssembly(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, activation := range []string{"newTextRewardHTTP(", "rewardHTTP.register(publicMux)", "startBonusRecovery("} {
		if !strings.Contains(source, activation) {
			t.Fatal("default reward assembly missing", activation)
		}
	}
	if strings.Index(source, "startBonusRecovery(") > strings.Index(source, "server.ListenAndServe()") {
		t.Fatal("reward worker must start before listeners")
	}
}

func TestTextBonusRuntimeOwnerLossCancelsBlockedSQLAndRestarts(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	match, _, _ := bonusRuntimeMatch(t, db, cfg, true)
	rt, err := newTextRuntime(t.Context(), db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rt.Close(context.Background()) })
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM text_matches WHERE id=$1 FOR UPDATE`, match); err != nil {
		t.Fatal(err)
	}
	gate := transport.NewWorkGate()
	defer gate.Close()
	if !rt.startBonusRecovery(t.Context(), gate, nil) {
		t.Fatal("worker refused")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		var blocked bool
		err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM text_matches%FOR UPDATE%')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if !waitTextBonus(ctx, time.Millisecond) {
			t.Fatal("worker never reached SQL wait")
		}
	}
	if _, err = db.Exec(`SELECT pg_terminate_backend(backend_pid) FROM text_process_owners WHERE incarnation_id=$1`, rt.Owner.Token().IncarnationID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-rt.Owner.Done():
	case <-ctx.Done():
		t.Fatal("physical owner loss not observed")
	}
	if err = rt.bonus.stop(ctx); err != nil {
		t.Fatal("owner loss failed to cancel SQL", err)
	}
	gate.Close()
	if err = gate.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM text_bonus_payments`).Scan(&n); err != nil || n != 0 {
		t.Fatal("blocked canceled pass paid", n, err)
	}
	if err = rt.Close(t.Context()); err != nil && !errors.Is(err, lobby.ErrTextUnavailable) {
		t.Fatal(err)
	}
	rt, err = newTextRuntime(t.Context(), db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	next := transport.NewWorkGate()
	defer next.Close()
	if !rt.startBonusRecovery(t.Context(), next, nil) {
		t.Fatal("successor refused")
	}
	waitBonusPayment(t, db)
	if err = rt.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	next.Close()
	if err = next.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM text_bonus_payments`).Scan(&n); err != nil || n != 1 {
		t.Fatal("restart duplicate", n, err)
	}
}

func TestTextBonusRuntimeShutdownJoinsBeforeOwnerRelease(t *testing.T) {
	db, cfg := textRuntimeProofDB(t)
	rt, err := newTextRuntime(t.Context(), db, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := newRuntimeLifecycle()
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	if !rt.bonus.start(t.Context(), lifecycle.Workers, rt.Owner.Done(), func(ctx context.Context) { close(entered); <-ctx.Done(); close(cancelled); <-release }) {
		t.Fatal("worker refused")
	}
	waitRuntimeSignal(t, entered)
	done := make(chan error, 1)
	go func() { done <- lifecycle.Shutdown(time.Second, rt.Close) }()
	waitRuntimeSignal(t, cancelled)
	if err = rt.Owner.Check(t.Context()); err != nil {
		close(release)
		t.Fatal("owner released before bonus worker joined", err)
	}
	select {
	case err = <-done:
		close(release)
		t.Fatal("shutdown completed before worker return", err)
	default:
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-rt.Owner.Done():
	default:
		t.Fatal("owner retained after joined shutdown")
	}
	if lifecycle.Workers.Active() != 0 {
		t.Fatal("worker retained after shutdown")
	}
}

func TestTextBonusWorkerOneLaunchOwnerCancelAndJoin(t *testing.T) {
	var worker textBonusWorker
	gate := transport.NewWorkGate()
	ownerDone := make(chan struct{})
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	if !worker.start(t.Context(), gate, ownerDone, func(ctx context.Context) { calls.Add(1); close(entered); <-ctx.Done(); close(cancelled); <-release }) {
		t.Fatal("start refused")
	}
	waitRuntimeSignal(t, entered)
	if worker.start(t.Context(), gate, ownerDone, func(context.Context) { calls.Add(1) }) {
		t.Fatal("duplicate worker launched")
	}
	close(ownerDone)
	waitRuntimeSignal(t, cancelled)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := worker.stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unjoined worker reported stopped", err)
	}
	close(release)
	if err := worker.stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	gate.Close()
	if err := gate.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
}
