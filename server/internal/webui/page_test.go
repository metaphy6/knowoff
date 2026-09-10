package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderEscapesContentAndMarksCurrentNavigation(t *testing.T) {
	w := httptest.NewRecorder()
	Render(w, 200, Page{Title: "Contribute", Area: "portal", Path: "/portal/submissions", CSRF: "safe-token", Data: `<script>alert(1)</script>`}, `<div class="preview">{{.Data}}</div>`)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{`&lt;script&gt;alert(1)&lt;/script&gt;`, `href="/portal/submissions" aria-current="page"`, `name="csrf_token" value="safe-token"`, `name="viewport"`, `Skip to content`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(w.Body.String(), `<script>`) {
		t.Fatal("raw content reached HTML")
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
}
