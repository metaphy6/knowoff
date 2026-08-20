package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/skip2/go-qrcode"
)

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
