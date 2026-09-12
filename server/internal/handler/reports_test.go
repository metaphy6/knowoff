package handler

import (
	"context"
	"encoding/json"
	"github.com/knowoff/knowoff/server/internal/reports"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPublicReportAndFeedbackRejectUnknownTrailingAndOversize(t *testing.T) {
	if os.Getenv("KNOWOFF_TEST_DB_TOKEN") == "" {
		t.Fatal("disposable runner required")
	}
	_, econ, auth, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	db := econ.DB()
	account, token := seedAccountForEconomy(t, ctx, db, auth)
	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, PublicRouteDeps{Auth: auth, Reports: reports.NewManager(db)})
	valid := map[string]any{"report_type": "conduct", "target_account_id": account, "reason": "test"}
	raw, _ := json.Marshal(valid)
	for _, body := range []string{strings.TrimSuffix(string(raw), "}") + `,"private_role":"donower"}`, string(raw) + ` {}`, `{"report_type":"conduct","target_account_id":"` + account + `","reason":"` + strings.Repeat("x", 20000) + `"}`} {
		r := httptest.NewRequest("POST", "/api/reports", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 400 && w.Code != 413 {
			t.Errorf("invalid body accepted status=%d", w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/feedback", strings.NewReader(`{"type":"bug","message":"test","context_snapshot":{"role":"donower"}}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("private context status=%d", w.Code)
	}
}
