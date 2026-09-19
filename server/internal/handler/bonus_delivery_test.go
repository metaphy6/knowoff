package handler

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"gopkg.in/yaml.v3"
)

func bonusHTTPPaid(t *testing.T, db *sql.DB, am *auth.Manager) (*store.TextValueStore, []string, []string) {
	t.Helper()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var tuning config.TuningConfig
	if err = yaml.Unmarshal(raw, &tuning); err != nil {
		t.Fatal(err)
	}
	v := store.NewTextValueStore(db, tuning)
	ids, tokens, admissions := []string{}, []string{}, []string{}
	at := time.Now().UTC().Truncate(time.Microsecond)
	for range 4 {
		id, token := seedAccountForEconomy(t, t.Context(), db, am)
		admission := uuid.NewString()
		if err = v.Reserve(t.Context(), store.TextReservation{ID: admission, AccountID: id, EntryPath: "quick_play", At: at}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		tokens = append(tokens, token)
		admissions = append(admissions, admission)
	}
	hash, err := tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	m := store.TextMatchRecord{Contract: v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: gamecontract.ModeMissedTheBriefing, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: "fixture", PackSHA256: strings.Repeat("a", 64), Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: true, Leaderboard: true}}, Owner: uuid.NewString(), Epoch: 1, AdmissionIDs: admissions}
	if err = v.Prepare(t.Context(), m, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO entitlements(account_id,entitlement_type) VALUES($1,'premium_monthly')`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err = v.Start(t.Context(), m.Contract.MatchID, m.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	o := store.TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(time.Microsecond)}
	for i, id := range ids {
		role := "nower"
		if i == 0 {
			role = "donower"
		}
		o.Players = append(o.Players, store.TextPlayerResult{AccountID: id, Seat: i, Role: role})
	}
	if err = v.Finish(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err = v.SettlePending(t.Context(), m.Contract.MatchID); err != nil {
		t.Fatal(err)
	}
	if _, err = v.ApplyBonus(t.Context(), m.Contract.MatchID, ids[0]); err != nil {
		t.Fatal(err)
	}
	return v, ids, tokens
}

func bonusHTTPCall(t *testing.T, h http.Handler, token, body string) (int, []byte) {
	t.Helper()
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}

func TestBonusDeliveryHTTPRealPaymentPrivateClaimAndACK(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	v, _, tokens := bonusHTTPPaid(t, econ.DB(), am)
	h, err := NewBonusDeliveryHandlers(BonusDeliveryHTTPConfig{Timeout: time.Second, MaxConcurrent: 2}, am, v)
	if err != nil {
		t.Fatal(err)
	}
	code, raw := bonusHTTPCall(t, h.Claim, tokens[0], `{"limit":20,"pending_delivery_ids":[]}`)
	var page store.TextBonusDeliveryPage
	if code != 200 || json.Unmarshal(raw, &page) != nil || len(page.Deliveries) != 1 {
		t.Fatal("real private claim", code)
	}
	d := page.Deliveries[0]
	body, _ := json.Marshal(map[string]string{"delivery_id": d.DeliveryID, "lease": d.Lease})
	if code, _ = bonusHTTPCall(t, h.ACK, tokens[1], string(body)); code != 409 {
		t.Fatal("other account ack", code)
	}
	if code, _ = bonusHTTPCall(t, h.ACK, tokens[0], string(body)); code != 200 {
		t.Fatal("owner ack", code)
	}
	status, _ := json.Marshal(map[string]any{"limit": 20, "pending_delivery_ids": []string{d.DeliveryID}})
	code, raw = bonusHTTPCall(t, h.Claim, tokens[0], string(status))
	if code != 200 || json.Unmarshal(raw, &page) != nil || len(page.AcknowledgedIDs) != 1 || len(page.Deliveries) != 0 {
		t.Fatal("real lost ACK recovery", code)
	}
}

func TestBonusDeliveryHTTPJWTExpiresDuringOutboxWait(t *testing.T) {
	_, econ, _, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	am := auth.NewManager(econ.DB(), []byte("test-key-test-key-test-key-test"), "test", "test", 2*time.Second, time.Hour, auth.OAuthProviders{})
	v, ids, tokens := bonusHTTPPaid(t, econ.DB(), am)
	h, err := NewBonusDeliveryHandlers(BonusDeliveryHTTPConfig{Timeout: 5 * time.Second, MaxConcurrent: 2}, am, v)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := econ.DB().BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM text_bonus_outbox WHERE account_id=$1 FOR UPDATE`, ids[0]); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		code, _ := bonusHTTPCall(t, h.Claim, tokens[0], `{"limit":20,"pending_delivery_ids":[]}`)
		done <- code
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	for {
		var waiting bool
		if err = econ.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT id,payload,payload_sha256 FROM text_bonus_outbox%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case code := <-done:
			t.Fatal("claim did not wait", code)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(strings.Split(tokens[0], ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err = json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatal(err)
	}
	if _, err = econ.DB().ExecContext(ctx, `SELECT pg_sleep(GREATEST(0,$1-extract(epoch FROM clock_timestamp()))+0.05)`, claims.Exp); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 401 {
		t.Fatal("expired authority committed claim", code)
	}
	var attempts int
	if err = econ.DB().QueryRow(`SELECT attempts FROM text_bonus_outbox WHERE account_id=$1`, ids[0]).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("expired claim retained lease", attempts, err)
	}
}

type bonusHTTPStore struct {
	db    *sql.DB
	calls int
}

func (s *bonusHTTPStore) ClaimBonusDeliveries(ctx context.Context, account string, limit int, pending []string, guard func(context.Context, *sql.Tx) error) (store.TextBonusDeliveryPage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.TextBonusDeliveryPage{}, err
	}
	defer tx.Rollback()
	if err = guard(ctx, tx); err != nil {
		return store.TextBonusDeliveryPage{}, err
	}
	s.calls++
	return store.TextBonusDeliveryPage{Version: 1, Deliveries: []store.TextBonusDelivery{}, AcknowledgedIDs: []string{}}, nil
}
func (s *bonusHTTPStore) AcknowledgeBonusDelivery(ctx context.Context, account, id, lease string, guard func(context.Context, *sql.Tx) error) error {
	_, err := s.ClaimBonusDeliveries(ctx, account, 1, []string{}, guard)
	return err
}

func TestBonusDeliveryHTTPStrictBodyAndCurrentBearer(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	s := &bonusHTTPStore{db: econ.DB()}
	h, err := NewBonusDeliveryHandlers(BonusDeliveryHTTPConfig{Timeout: time.Second, MaxConcurrent: 2}, am, s)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Claim)
	defer srv.Close()
	request := func(body, credential string) int {
		req, err := http.NewRequest("POST", srv.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res.StatusCode
	}
	if n := request(`{"limit":20,"pending_delivery_ids":[]}`, ""); n != 401 {
		t.Fatal("missing bearer", n)
	}
	for _, body := range []string{`{"limit":20,"pending_delivery_ids":[],"amount":1}`, `{"limit":1,"limit":2,"pending_delivery_ids":[]}`, `{"limit":1}`, `{"limit":1.5,"pending_delivery_ids":[]}`, `{"limit":1,"pending_delivery_ids":[]} {}`, `{"limit":0,"pending_delivery_ids":[]}`, `{"limit":21,"pending_delivery_ids":[]}`, `{"limit":1,"pending_delivery_ids":null}`, `{"limit":1,"pending_delivery_ids":["bad"]}`, strings.Repeat("x", 4097)} {
		if n := request(body, token); n != 400 {
			t.Fatal("invalid body", n, body)
		}
	}
	if s.calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	if n := request(`{"limit":20,"pending_delivery_ids":[]}`, token); n != 200 || s.calls != 1 {
		t.Fatal("valid claim", n, s.calls)
	}
	if err := am.RevokeSessions(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	if n := request(`{"limit":20,"pending_delivery_ids":[]}`, token); n != 401 || s.calls != 1 {
		t.Fatal("revoked bearer reached storage", n, s.calls)
	}
}

func TestBonusDeliveryHTTPSlowBodiesAndUnsupportedDeadline(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	_, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	s := &bonusHTTPStore{db: econ.DB()}
	h, err := NewBonusDeliveryHandlers(BonusDeliveryHTTPConfig{Timeout: 100 * time.Millisecond, MaxConcurrent: 2}, am, s)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h.Claim)
	defer srv.Close()
	for _, tc := range []struct {
		method, bearer string
		want           int
	}{{"POST", token, 400}, {"POST", "", 401}, {"PUT", token, 405}} {
		conn, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err = conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err = fmt.Fprintf(conn, "%s / HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{", tc.method, tc.bearer); err != nil {
			t.Fatal(err)
		}
		res, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			conn.Close()
			t.Fatal(err)
		}
		res.Body.Close()
		conn.Close()
		if res.StatusCode != tc.want || time.Since(started) > 750*time.Millisecond || s.calls != 0 {
			t.Fatal("unbounded/accepted slow body", res.StatusCode, tc.want)
		}
	}
	w := httptest.NewRecorder()
	h.Claim.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("{")))
	if w.Code != 503 || w.Header().Get("Connection") != "close" || s.calls != 0 {
		t.Fatal("unsupported deadline accepted", w.Code)
	}
}

func TestBonusDeliveryHTTPRevocationDuringBodyAndACKShape(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	s := &bonusHTTPStore{db: econ.DB()}
	h, err := NewBonusDeliveryHandlers(BonusDeliveryHTTPConfig{Timeout: time.Second, MaxConcurrent: 2}, am, s)
	if err != nil {
		t.Fatal(err)
	}
	rd, wr := io.Pipe()
	defer rd.Close()
	defer wr.Close()
	r := httptest.NewRequest("POST", "/", rd)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() { h.Claim.ServeHTTP(w, r); close(done) }()
	if _, err = wr.Write([]byte("{")); err != nil {
		t.Fatal(err)
	}
	if err = am.RevokeSessions(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	if _, err = wr.Write([]byte(`"limit":1,"pending_delivery_ids":[]}`)); err != nil {
		t.Fatal(err)
	}
	wr.Close()
	<-done
	if w.Code != 401 || s.calls != 0 {
		t.Fatal("stale body authority", w.Code, s.calls)
	}
	_, token = seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	for _, body := range []string{`{"delivery_id":"` + uuid.NewString() + `","lease":"bad"}`, `{"delivery_id":"` + uuid.NewString() + `","lease":"` + base64.RawURLEncoding.EncodeToString(make([]byte, 32)) + `","amount":1}`} {
		w = &rewardDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
		r = httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		h.ACK.ServeHTTP(w, r)
		if w.Code != 400 || s.calls != 0 {
			t.Fatal("invalid ack accepted", w.Code)
		}
	}
}
