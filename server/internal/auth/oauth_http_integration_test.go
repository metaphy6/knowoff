package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/handler"
)

func TestOAuthRealHTTPLinkAndSecondDeviceRestoration(t *testing.T) {
	manager, providerToken := auth.GoogleOAuthHTTPFixtureForTest(t)
	mux := http.NewServeMux()
	handler.RegisterAuthRoutes(mux, handler.AuthDeps{Auth: manager})
	server := httptest.NewServer(mux)
	defer server.Close()
	request := func(method, path, credential string, body any) (int, http.Header, []byte) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = strings.NewReader(string(data))
		}
		r, err := http.NewRequest(method, server.URL+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		r.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, response.Header, data
	}
	installation := auth.HashDevice(uuid.NewString())
	status, _, data := request("POST", "/api/auth/device", "", map[string]string{"device_hash": installation})
	var original auth.TokenPair
	if err := json.Unmarshal(data, &original); err != nil || status != 200 {
		t.Fatal("device session", status, err)
	}
	subject := "http-proof-" + uuid.NewString()
	for _, intent := range []string{"link", "restore"} {
		credential := ""
		if intent == "restore" {
			installation = auth.HashDevice(uuid.NewString())
		}
		if intent == "link" {
			credential = original.AccessToken
		}
		status, headers, data := request("POST", "/api/auth/oauth/start", credential, map[string]string{"provider": "google", "intent": intent, "device_hash": installation})
		var flow auth.OAuthStart
		if err := json.Unmarshal(data, &flow); err != nil || status != 200 || headers.Get("Cache-Control") != "no-store" {
			t.Fatal("OAuth start", status, err)
		}
		target, err := url.Parse(flow.URL)
		if err != nil {
			t.Fatal(err)
		}
		providerToken(intent+"-code", subject, target.Query().Get("nonce"))
		callback := "/api/auth/oauth/callback?" + url.Values{"provider": {"google"}, "state": {target.Query().Get("state")}, "code": {intent + "-code"}}.Encode()
		status, headers, data = request("GET", callback, "", nil)
		if status != 200 || headers.Get("Referrer-Policy") != "no-referrer" || !strings.HasPrefix(headers.Get("Content-Type"), "text/html") {
			t.Fatal("callback", status)
		}
		for _, secret := range []string{flow.CompletionSecret, original.AccessToken, original.RefreshToken, subject, target.Query().Get("state"), intent + "-code"} {
			if strings.Contains(string(data), secret) {
				t.Fatal("browser callback disclosed identity proof")
			}
		}
		status, _, data = request("POST", "/api/auth/oauth/result", "", map[string]string{"flow_id": flow.FlowID, "completion_secret": flow.CompletionSecret})
		var restored auth.TokenPair
		if err = json.Unmarshal(data, &restored); err != nil || status != 200 || restored.AccountID != original.AccountID {
			t.Fatal("second-device identity changed", status, err)
		}
		if account, err := manager.ValidateAccessToken(t.Context(), restored.AccessToken); err != nil || account != original.AccountID {
			t.Fatal("restored credential unusable", err)
		}
		status, _, _ = request("GET", callback, "", nil)
		if status != 400 {
			t.Fatal("HTTP callback replay", status)
		}
	}
}
