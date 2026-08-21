package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/skip2/go-qrcode"
)

// RoomCreateHandler creates a new Local Room and returns its join code so the
// host can share it as a 6-character code or QR (via RoomJoinHandler).
func RoomCreateHandler(mgr *lobby.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"code":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Size int `json:"size"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		if body.Size == 0 {
			body.Size = 4
		}
		room, err := mgr.CreateRoom(body.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"code":"create_failed","error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"room_id":%q,"code":%q,"size":%d}`, room.ID, room.Code, room.Size)
	}
}

// RoomJoinHandler returns the deep-link information for a room code. Clients
// can use the returned URL to render a QR code or attempt a native deep link.
// If ?format=qr is present, it returns a PNG QR code image.
func RoomJoinHandler(mgr *lobby.Manager, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/join/")
		if code == "" {
			http.Error(w, `{"code":"missing"}`, http.StatusBadRequest)
			return
		}
		room := mgr.RoomByCode(code)
		if room == nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"code":%q,"error":"room_not_found"}`, code)
			return
		}
		joinURL := fmt.Sprintf("%s/join/%s", strings.TrimSuffix(baseURL, "/"), code)
		deepLink := fmt.Sprintf("knowoff://join?code=%s", code)

		if r.URL.Query().Get("format") == "qr" {
			png, err := qrcode.Encode(joinURL, qrcode.Medium, 256)
			if err != nil {
				http.Error(w, `{"code":"qr_failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":%q,"join_url":%q,"deep_link":%q}`, code, joinURL, deepLink)
	}
}
