package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
)

func TestConnectionRepliesShareTextSocketWriter(t *testing.T) {
	srv, _, manager := textHTTPFixture(t)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	done := make(chan error, 2)
	go func() {
		for i := 0; i < 10; i++ {
			if err := c.WriteJSON(map[string]any{"v": 2, "type": "availability", "request_id": fmt.Sprintf("lookup-%d", i), "payload": struct{}{}}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	go func() {
		for i := 0; i < 10; i++ {
			if err := manager.NotifyNotices(context.Background()); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	notices, acks, availability := 0, 0, 0
	for i := 0; i < 30; i++ {
		frame := textRead(t, c)
		switch frame.Type {
		case "system_notice":
			notices++
			if string(frame.Payload) != "{\"refresh\":true}" {
				t.Fatal("notice leaked obsolete payload")
			}
		case "control_ack":
			acks++
		case "availability":
			availability++
		default:
			t.Fatal("unexpected concurrent output", frame.Type)
		}
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if notices != 10 || acks != 10 || availability != 10 {
		t.Fatal("writer lost/corrupted messages", notices, acks, availability)
	}
}

func TestRealtimeHandlerKeepsIdleConnectionAliveWithPing(t *testing.T) {
	srv, _, _ := textHTTPFixture(t)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	var pings atomic.Int32
	c.SetPingHandler(func(data string) error {
		pings.Add(1)
		return c.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
	})
	finished := make(chan error, 1)
	go func() {
		c.SetReadDeadline(time.Now().Add(6 * time.Second))
		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				finished <- err
				return
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for pings.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if pings.Load() < 2 {
		t.Error("idle socket received fewer than two configured pings")
	}
	c.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("reader did not close")
	}
}

func TestRealtimeHandlerRejectsOverCapacityAndReleasesSlot(t *testing.T) {
	var cfg *config.Config
	_, _, manager := textHTTPCustomFixture(t, textAuthStub{}, func(c *config.Config, _ *lobby.TextDeps) { cfg = c })
	srv := httptest.NewServer(TextRealtimeHandler(TextHandlerDeps{Config: cfg, Lobby: manager, Auth: textAuthStub{}, ConnLimiter: ratelimit.NewConnLimiter(2)}))
	defer srv.Close()
	address := "ws" + strings.TrimPrefix(srv.URL, "http")
	first, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	rejected, response, err := websocket.DefaultDialer.Dial(address, nil)
	if rejected != nil {
		rejected.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("capacity did not refuse upgrade", err)
	}
	response.Body.Close()
	first.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		next, response, e := websocket.DefaultDialer.Dial(address, nil)
		if e == nil {
			next.Close()
			break
		}
		if response != nil {
			response.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatal("closed socket slot not released", e)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRetiredRoomHTTPAndQRRoutesNeverClaimSeats(t *testing.T) {
	auth := &textCountingAuth{}
	srv, values, _ := textHTTPAuthFixture(t, auth)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, path := range []string{"/rooms/create", "/rooms/create?size=6&dev_role=donower", "/join/ABCDEF", "/join/ABCDEF?format=qr", "/ws"} {
			request, err := http.NewRequest(method, srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusUpgradeRequired || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("retired route not fenced", method, path, response.StatusCode)
			}
			var body map[string]string
			if err = json.Unmarshal(raw, &body); err != nil || body["code"] != "protocol.upgrade_required" {
				t.Fatal("unclear obsolete client refusal", string(raw), err)
			}
		}
	}
	if values.reserved.Load() != 0 || auth.calls.Load() != 0 {
		t.Fatal("obsolete room/QR request reached auth/admission")
	}
}

func TestRetiredDevControlsCannotBindOrAlterSeats(t *testing.T) {
	srv, values, _ := textHTTPFixture(t)
	c := textDial(t, srv)
	textHello(t, c, uuid.NewString())
	for i, kind := range []string{"dev_force_role", "dev_grant_specialty", "dev_restart", "join", "queue", "prefetch_ack", "asset_request"} {
		id := fmt.Sprintf("retired-%d", i)
		if err := c.WriteJSON(map[string]any{"v": 2, "type": kind, "request_id": id, "payload": map[string]any{"role": "donower", "specialty": "one_more"}}); err != nil {
			t.Fatal(err)
		}
		frame := textRead(t, c)
		if frame.Type != "error" || frame.RequestID != id {
			t.Fatal("retired control accepted or lost correlation", frame)
		}
	}
	if values.reserved.Load() != 0 {
		t.Fatal("retired dev control reserved a seat")
	}
}
