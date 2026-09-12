package avatar

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func avatarAuth(t *testing.T, db *sql.DB) (*auth.Manager, *auth.TokenPair) {
	t.Helper()
	am := auth.NewManager(db, []byte("fixture-avatar-signing-key-32-bytes"), "avatar-test", "avatar-test", time.Hour, time.Hour, auth.OAuthProviders{})
	pair, err := am.AuthenticateDevice(t.Context(), "avatar-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return am, pair
}
func avatarRequest(t *testing.T, token string, image []byte) *http.Request {
	t.Helper()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	part, err := writer.CreateFormFile("avatar", "photo.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(image); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/avatar", &b)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}
func TestAvatarReadRequiresCurrentApprovedRevisionAndPrivateVisibility(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	am, owner := avatarAuth(t, db)
	_, viewer := avatarAuth(t, db)
	m := entitledManager(t, db, owner.AccountID)
	if err := m.ProcessUpload(t.Context(), owner.AccountID, bytes.NewReader(avatarPNG(t)), ""); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/avatar/{account_id}", m.ImageHandler(am))
	read := func(target, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/avatar/"+target, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, token := range []string{owner.AccessToken, viewer.AccessToken} {
		w := read(owner.AccountID, token)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/webp" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Body.Len() == 0 {
			t.Fatalf("approved read status=%d", w.Code)
		}
	}
	missing := read(uuid.NewString(), viewer.AccessToken)
	if missing.Code != 404 {
		t.Fatal("missing image status")
	}
	for _, query := range []string{
		`UPDATE custom_avatars SET moderated=false WHERE account_id=$1`,
		`UPDATE custom_avatars SET moderated=true,revision=NULL WHERE account_id=$1`,
		`UPDATE accounts SET avatar='party' WHERE id=$1`,
		`UPDATE accounts SET avatar='custom',deleted_at=clock_timestamp() WHERE id=$1`,
	} {
		if _, err := db.Exec(query, owner.AccountID); err != nil {
			t.Fatal(err)
		}
		w := read(owner.AccountID, viewer.AccessToken)
		if w.Code != missing.Code || w.Body.String() != missing.Body.String() {
			t.Fatal("unavailable image enumerated")
		}
	}
	for _, query := range []string{`UPDATE accounts SET deleted_at=NULL WHERE id=$1`, `UPDATE custom_avatars SET revision=1 WHERE account_id=$1`} {
		if _, err := db.Exec(query, owner.AccountID); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range [][2]string{{owner.AccountID, viewer.AccountID}, {viewer.AccountID, owner.AccountID}} {
		if _, err := db.Exec(`INSERT INTO player_blocks(actor_id,target_id,created_at) VALUES($1,$2,clock_timestamp())`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
		w := read(owner.AccountID, viewer.AccessToken)
		if w.Code != 404 || w.Body.String() != missing.Body.String() {
			t.Fatal("blocked image visible")
		}
		if _, err := db.Exec(`DELETE FROM player_blocks WHERE actor_id=$1 AND target_id=$2`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	if w := read(owner.AccountID, "invalid"); w.Code != 401 {
		t.Fatal("unauthenticated image visible")
	}
}
func TestAvatarUploadRevalidatesActualJWTAfterProviderWait(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	am, pair := avatarAuth(t, db)
	m := entitledManager(t, db, pair.AccountID)
	entered, release := make(chan struct{}), make(chan struct{})
	m.screen = func(context.Context, []byte) error { close(entered); <-release; return nil }
	r := avatarRequest(t, pair.AccessToken, avatarPNG(t))
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { m.Handler(am).ServeHTTP(w, r); close(done) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no provider call")
	}
	if err := am.RevokeSessions(t.Context(), pair.AccountID); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	<-done
	if w.Code != 409 && w.Code != 401 {
		t.Fatalf("revoked token status %d", w.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM custom_avatars WHERE account_id=$1`, pair.AccountID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("revoked upload wrote blob")
	}
}
func TestAvatarUploadHTTPBoundsAndAuthentication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	am, pair := avatarAuth(t, db)
	m := entitledManager(t, db, pair.AccountID)
	calls := 0
	m.screen = func(context.Context, []byte) error { calls++; return nil }
	for _, tc := range []struct {
		name, token string
		body        []byte
		status      int
	}{{"bad-auth", "invalid", avatarPNG(t), 401}, {"large", pair.AccessToken, bytes.Repeat([]byte{'x'}, maxAvatarBytes+1), 400}, {"format", pair.AccessToken, []byte("<svg/>"), 400}} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			m.Handler(am).ServeHTTP(w, avatarRequest(t, tc.token, tc.body))
			if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
	if calls != 0 {
		t.Fatal("invalid uploads screened")
	}
	w := httptest.NewRecorder()
	m.Handler(am).ServeHTTP(w, avatarRequest(t, pair.AccessToken, avatarPNG(t)))
	if w.Code != 204 || calls != 1 {
		t.Fatalf("valid upload %d calls%d", w.Code, calls)
	}
}

type countingAvatarBody struct{ reads int }

func (b *countingAvatarBody) Read(p []byte) (int, error) { b.reads++; return 0, io.EOF }
func (b *countingAvatarBody) Close() error               { return nil }
func TestAvatarAdmissionRejectsExcessBeforeReadingBodyOrScreening(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	am, pair := avatarAuth(t, db)
	m := entitledManager(t, db, pair.AccountID)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	m.screen = func(ctx context.Context, _ []byte) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		r := avatarRequest(t, pair.AccessToken, avatarPNG(t))
		go func() { m.Handler(am).ServeHTTP(httptest.NewRecorder(), r); done <- struct{}{} }()
	}
	defer func() { close(release); <-done; <-done }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("provider not reached")
		}
	}
	body := &countingAvatarBody{}
	r := httptest.NewRequest(http.MethodPost, "/api/avatar", body)
	r.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.Header.Set("Content-Type", "multipart/form-data; boundary=fixture")
	w := httptest.NewRecorder()
	m.Handler(am).ServeHTTP(w, r)
	if w.Code != 429 || body.reads != 0 {
		t.Fatalf("over-cap upload status=%d reads=%d", w.Code, body.reads)
	}
	if err := m.ProcessUpload(t.Context(), pair.AccountID, body, ""); err == nil || body.reads != 0 {
		t.Fatal("trusted upload bypassed shared bound")
	}
}

func TestAvatarCancelledScreenReleasesConfiguredSingleSlot(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	entitledManager(t, db, account)
	cfg := testConfig()
	cfg.Moderation.AvatarUploadSlots = 1
	m := NewManager(db, cfg, nil)
	entered := make(chan struct{})
	m.screen = func(ctx context.Context, _ []byte) error { close(entered); <-ctx.Done(); return ctx.Err() }
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	source := avatarPNG(t)
	go func() { result <- m.ProcessUpload(ctx, account, bytes.NewReader(source), "") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("screen missing")
	}
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); !errors.Is(err, ErrBusy) {
		t.Fatal("configured bound bypassed", err)
	}
	cancel()
	if err := <-result; err == nil {
		t.Fatal("cancel accepted")
	}
	m.screen = func(context.Context, []byte) error { return nil }
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); err != nil {
		t.Fatal("canceled slot leaked", err)
	}
}
func TestAvatarCaptureAndFinalAccountLocksAreBounded(t *testing.T) {
	for _, final := range []bool{false, true} {
		t.Run(map[bool]string{false: "capture", true: "activation"}[final], func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			account := newAccount(t, db)
			m := entitledManager(t, db, account)
			m.sqlTimeout = 80 * time.Millisecond
			barrier, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback()
			entered, release := make(chan struct{}), make(chan struct{})
			if final {
				m.screen = func(context.Context, []byte) error { close(entered); <-release; return nil }
			} else {
				if _, err = barrier.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
					t.Fatal(err)
				}
			}
			result := make(chan error, 1)
			source := avatarPNG(t)
			go func() { result <- m.ProcessUpload(t.Context(), account, bytes.NewReader(source), "") }()
			if final {
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("screen missing")
				}
				if _, err = barrier.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
					t.Fatal(err)
				}
				close(release)
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("blocked write activated")
				}
			case <-time.After(time.Second):
				t.Fatal("SQL lock has no bounded deadline")
			}
			if err := barrier.Rollback(); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM custom_avatars WHERE account_id=$1`, account).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("timeout activated image")
			}
			if len(m.slots) != 0 {
				t.Fatal("SQL timeout leaked slot")
			}
		})
	}
}
