package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestTextContentRejectsSignedURLsAndAssetFieldsWithoutFetching(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	for _, field := range []string{"url", "signed_url", "asset_ref", "embedding"} {
		for _, item := range []string{"nowns", "cards"} {
			t.Run(field+"/"+item, func(t *testing.T) {
				raw, err := json.Marshal(textFixtureBundle(t, "en"))
				if err != nil {
					t.Fatal(err)
				}
				var object map[string]any
				if err := json.Unmarshal(raw, &object); err != nil {
					t.Fatal(err)
				}
				rows := object[item].([]any)
				rows[0].(map[string]any)[field] = server.URL + "/private?round=old&token=synthetic-expired-token"
				raw, err = json.Marshal(object)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := DecodeTextBundle(raw, textFixtureLimits()); err == nil {
					t.Fatal("retired asset field accepted")
				}
				if requests.Load() != 0 {
					t.Fatal("text validation fetched a signed asset")
				}
			})
		}
	}
}
