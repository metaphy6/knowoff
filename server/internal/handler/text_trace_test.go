package handler

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

type textTraceFrame struct {
	Direction string          `json:"direction"`
	Frame     json.RawMessage `json:"frame"`
}
type textTrace struct {
	mu     sync.Mutex
	frames []textTraceFrame
}
type textSessionTrace struct {
	Mode   string           `json:"mode"`
	Size   int              `json:"size"`
	Frames []textTraceFrame `json:"frames"`
}

var textTraces sync.Map

func textCapture(c *websocket.Conn, direction string, v any) {
	item, ok := textTraces.Load(c)
	if !ok {
		return
	}
	trace := item.(*textTrace)
	raw, e := json.Marshal(v)
	if e != nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	trace.frames = append(trace.frames, textTraceFrame{direction, raw})
}
func textWriteJSON(c *websocket.Conn, v any) error {
	e := c.WriteJSON(v)
	if e == nil {
		textCapture(c, "client", v)
	}
	return e
}
func textCaptured(c *websocket.Conn) []textTraceFrame {
	item, ok := textTraces.Load(c)
	if !ok {
		return nil
	}
	trace := item.(*textTrace)
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]textTraceFrame(nil), trace.frames...)
}
func writeTextSessionTraces(t *testing.T, sessions []textSessionTrace) {
	t.Helper()
	path := os.Getenv("KNOWOFF_TEXT_TRACE_OUT")
	if path == "" {
		return
	}
	raw, e := json.MarshalIndent(struct {
		Version   int                `json:"version"`
		Synthetic bool               `json:"synthetic"`
		Sessions  []textSessionTrace `json:"sessions"`
	}{1, true, sessions}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	// Only synthetic UUID account/token placeholders from textAuthStub are saved.
	// This shared protocol fixture is intentionally readable by both test runtimes.
	if e = os.WriteFile(path, append(raw, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
}
