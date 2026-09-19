package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

// These are diagnostic resource ceilings, not gameplay tuning. A network bot
// obtains all gameplay state and limits from its authenticated recipient stream.
const textNetworkTraceLimit = 32 << 20

var textNetworkObservationOrder atomic.Uint64

var textNetworkHTTP = &http.Client{Transport: &http.Transport{}, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
	return errors.New("network proof does not follow redirects")
}}

// Development credentials are minted only by the server's authenticated
// environment policy. Ordinary device credentials are never used for this path.
func networkDevelopmentAuth(ctx context.Context, endpoint, key string) (string, error) {
	if err := textNetworkEndpoint(endpoint); err != nil {
		return "", err
	}
	if key == "" {
		return "", errors.New("KNOWOFF_DEV_BOT_KEY is required")
	}
	u, _ := url.Parse(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+u.Host+"/api/auth/development", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := textNetworkHTTP.Do(req)
	if err != nil {
		return "", errors.New("development authentication request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("development authentication status %d", response.StatusCode)
	}
	var pair struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 65537)).Decode(&pair); err != nil || pair.AccessToken == "" || len(pair.AccessToken) > 16384 {
		return "", errors.New("invalid development authentication response")
	}
	return pair.AccessToken, nil
}

func runTextNetwork(ctx context.Context, endpoint string, mode gamecontract.ModeID, size int, seed int64, output, key string) (runErr error) {
	if err := textNetworkEndpoint(endpoint); err != nil {
		return err
	}
	if output == "" || (size != 4 && size != 6) || key == "" {
		return errors.New("text network proof requires -text-out, -text-size 4/6 and KNOWOFF_DEV_BOT_KEY")
	}
	if _, err := os.Lstat(output); err == nil {
		return errors.New("private network output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	// Availability is authenticated. The dedicated development endpoint already
	// enforces server-side prototype policy; reuse this first identity for seat 0.
	firstToken, err := networkDevelopmentAuth(ctx, endpoint, key)
	if err != nil {
		return err
	}
	u, _ := url.Parse(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+u.Host+"/api/text/availability", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+firstToken)
	response, err := textNetworkHTTP.Do(req)
	if err != nil {
		return errors.New("prototype availability request failed")
	}
	defer response.Body.Close()
	var availability lobby.TextAvailability
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&availability) != nil || !availability.Prototype {
		return errors.New("server has not authorized prototype availability")
	}
	settings := v2.LobbySettings{ModeID: mode, Size: size}
	for _, candidate := range availability.Modes {
		if candidate.ModeID == mode && candidate.Available && len(candidate.Languages) > 0 {
			language := candidate.Languages[0]
			settings.ContentLanguage = language.ContentLanguage
			settings.PackReleaseID = language.PackReleaseID
			settings.RulesVersion = language.RulesVersion
			break
		}
	}
	if settings.ContentLanguage == "" {
		return errors.New("requested prototype mode unavailable")
	}
	peers := make([]*textNetworkBot, 0, size)
	defer func() {
		for _, peer := range peers {
			peer.close()
		}
		if runErr != nil && len(peers) > 0 {
			runErr = errors.Join(runErr, writeTextNetworkTrace(output, peers))
		}
	}()
	for i := 0; i < size; i++ {
		token := firstToken
		if i > 0 {
			token, err = networkDevelopmentAuth(ctx, endpoint, key)
			if err != nil {
				return err
			}
		}
		peer, err := connectTextNetwork(ctx, endpoint, token, seed+int64(i))
		if err != nil {
			return err
		}
		peers = append(peers, peer)
	}
	if err := peers[0].control(ctx, "room_create", settings); err != nil {
		return err
	}
	code := peers[0].lobby.Code
	for _, peer := range peers[1:] {
		if err := peer.control(ctx, "room_join", map[string]string{"code": code}); err != nil {
			return err
		}
	}
	for _, peer := range peers {
		if err := peer.control(ctx, "resync", struct{}{}); err != nil {
			return err
		}
		state := peer.lobby.Lobby
		if err := peer.control(ctx, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: state.SettingsRevision, MembershipRevision: state.MembershipRevision}); err != nil {
			return err
		}
	}
	if err := peers[0].control(ctx, "room_start", struct{}{}); err != nil {
		return err
	}
	reconnected, actions := false, 0
	for step := 0; step < 2000; step++ {
		for _, peer := range peers {
			if err := peer.control(ctx, "resync", struct{}{}); err != nil {
				return err
			}
		}
		if peers[0].snapshot.Phase == v2.PhaseVerdict {
			complete := true
			for _, peer := range peers {
				complete = complete && peer.snapshot.Phase == v2.PhaseVerdict && len(peer.deliveries) == 1
			}
			if complete {
				return writeTextNetworkTrace(output, peers)
			}
		} else {
			if !reconnected && actions >= 3 && peers[0].snapshot.PendingOffer == nil {
				if err := peers[0].reconnect(ctx, code); err != nil {
					return err
				}
				reconnected = true
				continue
			}
			acted := false
			for _, peer := range peers {
				if action := textPolicy(peer.snapshot, peer.rng); action != nil {
					if err := peer.act(ctx, *action); err != nil {
						return err
					}
					actions++
					acted = true
					break
				}
			}
			if acted {
				continue
			}
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("prototype match did not finish within bounded network steps")
}

type textNetworkFrame struct {
	Order     uint64             `json:"order"`
	Direction string             `json:"direction"`
	AtMS      int64              `json:"observed_at_ms"`
	Frame     lobby.TextEnvelope `json:"frame"`
}

type textNetworkBot struct {
	endpoint, token string
	conn            *websocket.Conn
	rng             *rand.Rand
	seed            int64
	serial          int
	account         string
	availability    lobby.TextAvailability
	limits          v2.Limits
	lobby           lobby.TextLobbyView
	snapshot        v2.Snapshot
	pending         *v2.Snapshot
	pages           []v2.HistoryPage
	pageCount       int
	deliveries      map[int64]store.TextPrivateSettlement
	trace           []textNetworkFrame
	traceBytes      int
	retiredEpochs   map[string]bool
}

func textNetworkEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || u.Scheme != "ws" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/ws/v2" || u.RawPath != "" {
		return errors.New("text network proof requires an explicit loopback ws:// endpoint at /ws/v2")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("text network proof requires a literal loopback address")
	}
	return nil
}

func connectTextNetwork(ctx context.Context, endpoint, token string, seed int64) (*textNetworkBot, error) {
	if err := textNetworkEndpoint(endpoint); err != nil {
		return nil, err
	}
	b := &textNetworkBot{endpoint: endpoint, token: token, seed: seed, rng: rand.New(rand.NewSource(seed)), deliveries: map[int64]store.TextPrivateSettlement{}}
	if err := b.connect(ctx); err != nil {
		b.close()
		return nil, err
	}
	return b, nil
}

func textNetworkDeadline(ctx context.Context) time.Time {
	at := time.Now().Add(10 * time.Second)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(at) {
		at = deadline
	}
	return at
}

func (b *textNetworkBot) connect(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, response, err := dialer.DialContext(ctx, b.endpoint, nil)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return err
	}
	b.conn = conn
	b.conn.SetReadLimit(1 << 20)
	// Authentication is deliberately excluded from privileged gameplay artifacts.
	if err := b.write(ctx, "hello", "", map[string]any{"client_generation": 2, "access_token": b.token}, false); err != nil {
		return err
	}
	hello, err := b.read(ctx)
	if err != nil {
		return err
	}
	var h struct {
		Prototype  bool             `json:"prototype"`
		Generation int              `json:"client_generation"`
		Account    string           `json:"account_id"`
		Limits     lobby.TextLimits `json:"limits"`
	}
	if hello.Type != "hello" || networkDecode(hello.Payload, &h) != nil || !h.Prototype || h.Generation != 2 || h.Account == "" {
		return errors.New("server has not authorized a prototype connection")
	}
	if b.account != "" && h.Account != b.account {
		return errors.New("reconnect account changed")
	}
	b.account = h.Account
	b.limits = v2.Limits{MaxFrameBytes: h.Limits.MaxFrameBytes, MaxHistoryEvents: h.Limits.MaxHistoryEvents, MaxHistoryPageEvents: h.Limits.MaxHistoryPageEvents, MaxTextBytes: h.Limits.MaxTextBytes, MaxRequestsPerSeat: h.Limits.MaxRequestsPerSeat}
	if err := b.limits.Validate(); err != nil || b.limits.MaxFrameBytes > 1<<20 {
		return errors.New("invalid server network limits")
	}
	b.conn.SetReadLimit(int64(b.limits.MaxFrameBytes))
	availability, err := b.read(ctx)
	if err != nil {
		return err
	}
	if availability.Type != "availability" || networkDecode(availability.Payload, &b.availability) != nil || !b.availability.Prototype || b.availability.ProtocolVersion != 2 || b.availability.ClientGeneration != 2 || b.availability.Limits != h.Limits {
		return errors.New("server has not authorized prototype availability")
	}
	return nil
}

func (b *textNetworkBot) close() {
	if b.conn != nil {
		b.conn.Close()
	}
}

func (b *textNetworkBot) reconnect(ctx context.Context, code string) error {
	b.close()
	b.pending, b.pages = nil, nil
	if err := b.connect(ctx); err != nil {
		return err
	}
	return b.control(ctx, "room_join", map[string]any{"code": code})
}

func networkDecode(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("extra network JSON")
	}
	return nil
}

func (b *textNetworkBot) record(direction string, frame lobby.TextEnvelope) error {
	if b.token != "" && (textJSONContainsCredential(frame.Payload, b.token) || strings.Contains(frame.RequestID, b.token) || strings.Contains(frame.Type, b.token)) {
		return errors.New("network frame unexpectedly contained an authentication credential")
	}
	n := len(frame.Payload) + len(frame.Type) + len(frame.RequestID) + 100
	if b.traceBytes+n > textNetworkTraceLimit {
		return errors.New("private network trace limit reached")
	}
	b.traceBytes += n
	b.trace = append(b.trace, textNetworkFrame{Order: textNetworkObservationOrder.Add(1), Direction: direction, AtMS: time.Now().UnixMilli(), Frame: frame})
	return nil
}

func textJSONContainsCredential(raw []byte, token string) bool {
	if token == "" {
		return false
	}
	if bytes.Contains(raw, []byte(token)) {
		return true
	}
	if bytes.IndexByte(raw, '\\') >= 0 {
		// Inspect decoded strings too: a reflected token may use JSON escapes.
		// Token scanning retains duplicate keys, unlike decoding into a map.
		decoder := json.NewDecoder(bytes.NewReader(raw))
		for {
			value, err := decoder.Token()
			if err != nil {
				break
			}
			if value, ok := value.(string); ok && strings.Contains(value, token) {
				return true
			}
		}
	}
	return false
}

func (b *textNetworkBot) write(ctx context.Context, kind, id string, payload any, record bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame := lobby.TextEnvelope{Version: 2, Type: kind, RequestID: id, Payload: raw}
	if record {
		if err := b.record("client", frame); err != nil {
			return err
		}
	}
	if err := b.conn.SetWriteDeadline(textNetworkDeadline(ctx)); err != nil {
		return err
	}
	return b.conn.WriteJSON(frame)
}

func (b *textNetworkBot) read(ctx context.Context) (lobby.TextEnvelope, error) {
	var frame lobby.TextEnvelope
	if err := ctx.Err(); err != nil {
		return frame, err
	}
	if err := b.conn.SetReadDeadline(textNetworkDeadline(ctx)); err != nil {
		return frame, err
	}
	kind, raw, err := b.conn.ReadMessage()
	if err != nil {
		return frame, err
	}
	if kind != websocket.TextMessage || networkDecode(raw, &frame) != nil || frame.Version != 2 {
		return frame, errors.New("invalid network envelope")
	}
	return frame, b.record("server", frame)
}

func (b *textNetworkBot) apply(snapshot v2.Snapshot) error {
	if snapshot.Contract.Eligibility.Rewards || snapshot.Contract.Eligibility.Leaderboard {
		return errors.New("prototype unexpectedly became value eligible")
	}
	old := b.snapshot
	if b.retiredEpochs == nil {
		b.retiredEpochs = map[string]bool{}
	}
	if b.retiredEpochs[snapshot.Cursor.StreamEpoch] {
		return errors.New("retired recipient stream replayed")
	}
	if old.Version == 0 && snapshot.Cursor.RecipientSeq != 1 {
		return errors.New("initial recipient stream did not begin at one")
	}
	if old.Version != 0 {
		if snapshot.Contract != old.Contract || snapshot.Private.Seat != old.Private.Seat || snapshot.Private.Role != old.Private.Role {
			return errors.New("recipient or immutable match contract changed")
		}
		if snapshot.Cursor.StreamEpoch == old.Cursor.StreamEpoch {
			if snapshot.Cursor.RecipientSeq != old.Cursor.RecipientSeq+1 {
				return errors.New("recipient sequence gap")
			}
		} else if snapshot.Cursor.RecipientSeq != 1 {
			return errors.New("resync did not begin a new recipient stream")
		}
		if len(snapshot.History) < len(old.History) || snapshot.Board.Revision < old.Board.Revision {
			return errors.New("public evidence regressed")
		}
		for i := range old.History {
			a, _ := json.Marshal(old.History[i])
			c, _ := json.Marshal(snapshot.History[i])
			if !bytes.Equal(a, c) {
				return errors.New("previous public evidence changed")
			}
		}
	}
	if old.Version != 0 && snapshot.Cursor.StreamEpoch != old.Cursor.StreamEpoch {
		b.retiredEpochs[old.Cursor.StreamEpoch] = true
	}
	b.snapshot = snapshot
	return nil
}

func (b *textNetworkBot) receive(frame lobby.TextEnvelope) error {
	switch frame.Type {
	case "lobby":
		var next lobby.TextLobbyView
		if err := networkDecode(frame.Payload, &next); err != nil {
			return err
		}
		if b.snapshot.Phase == v2.PhaseVerdict && next.Lobby.SettingsRevision > b.lobby.Lobby.SettingsRevision && next.Lobby.MembershipRevision > b.lobby.Lobby.MembershipRevision {
			b.snapshot = v2.Snapshot{}
			b.pending, b.pages, b.retiredEpochs = nil, nil, nil
		}
		b.lobby = next
		return nil
	case "snapshot":
		var s v2.Snapshot
		if err := v2.Decode(frame.Payload, &s, b.limits); err != nil {
			return err
		}
		if b.pending != nil {
			return errors.New("snapshot interrupted incomplete history")
		}
		if s.HistoryPages != nil {
			b.pending, b.pages = &s, nil
			return nil
		}
		return b.apply(s)
	case "history_page":
		var p v2.HistoryPage
		if err := v2.Decode(frame.Payload, &p, b.limits); err != nil {
			return err
		}
		if b.pending == nil {
			return errors.New("history page without pending snapshot")
		}
		b.pages = append(b.pages, p)
		b.pageCount++
		if len(b.pages) == b.pending.HistoryPages.PageCount {
			s, err := v2.ResolveHistory(*b.pending, b.pages, b.limits)
			if err != nil {
				return err
			}
			b.pending, b.pages = nil, nil
			return b.apply(s)
		}
		return nil
	case "settlement":
		var delivery store.TextDelivery
		if err := networkDecode(frame.Payload, &delivery); err != nil {
			return err
		}
		var value store.TextPrivateSettlement
		if err := networkDecode(delivery.Payload, &value); err != nil {
			return err
		}
		if b.snapshot.Phase != v2.PhaseVerdict || delivery.ID <= 0 || value.MatchID != b.snapshot.Contract.MatchID || delivery.MatchID != value.MatchID || value.Points != 0 || value.XP != 0 || value.LeaderboardCounted {
			return errors.New("nonterminal, foreign or value-bearing prototype settlement")
		}
		for _, award := range value.Awards {
			if award.Credited != 0 {
				return errors.New("prototype award credited Noin")
			}
		}
		if old, exists := b.deliveries[delivery.ID]; exists {
			a, _ := json.Marshal(old)
			c, _ := json.Marshal(value)
			if !bytes.Equal(a, c) {
				return errors.New("private delivery replay changed")
			}
		}
		b.deliveries[delivery.ID] = value
		return nil
	case "error":
		var e struct {
			Code      string     `json:"code"`
			RequestID string     `json:"request_id"`
			Cursor    *v2.Cursor `json:"cursor"`
			Version   int        `json:"v"`
			Revision  *uint64    `json:"current_board_revision"`
		}
		if err := networkDecode(frame.Payload, &e); err != nil {
			return err
		}
		if e.Cursor != nil {
			var event v2.ErrorEvent
			if err := v2.Decode(frame.Payload, &event, b.limits); err != nil {
				return err
			}
			old := b.snapshot.Cursor
			if event.Cursor.StreamEpoch != old.StreamEpoch || event.Cursor.RecipientSeq != old.RecipientSeq+1 || event.Cursor.EvidenceSeq != old.EvidenceSeq || event.CurrentBoardRevision == nil || *event.CurrentBoardRevision != b.snapshot.Board.Revision {
				return errors.New("invalid action error cursor")
			}
			b.snapshot.Cursor = event.Cursor
		}
		return fmt.Errorf("server rejected network request: %s", e.Code)
	default:
		return fmt.Errorf("unexpected network frame type %q", frame.Type)
	}
}

func (b *textNetworkBot) await(ctx context.Context, kind, id string) error {
	for {
		frame, err := b.read(ctx)
		if err != nil {
			return err
		}
		if frame.Type == kind {
			var ack struct {
				RequestID string `json:"request_id"`
				Duplicate bool   `json:"duplicate"`
			}
			if networkDecode(frame.Payload, &ack) != nil || ack.RequestID != id || frame.RequestID != id || b.pending != nil {
				return errors.New("invalid request acknowledgement")
			}
			return nil
		}
		if err := b.receive(frame); err != nil {
			return err
		}
	}
}

func (b *textNetworkBot) nextID() string {
	b.serial++
	return fmt.Sprintf("network-%d-%d", b.seed, b.serial)
}

func (b *textNetworkBot) control(ctx context.Context, kind string, payload any) (err error) {
	if !b.availability.Prototype {
		return errors.New("prototype admission not authorized")
	}
	id := b.nextID()
	defer func() {
		if err != nil {
			err = fmt.Errorf("control %s request %s: %w", kind, id, err)
		}
	}()
	if err := b.write(ctx, kind, id, payload, true); err != nil {
		return err
	}
	return b.await(ctx, "control_ack", id)
}

func (b *textNetworkBot) act(ctx context.Context, action v2.Action) error {
	s := b.snapshot
	if b.pending != nil || s.Version == 0 {
		return errors.New("action requires complete recipient snapshot")
	}
	req := v2.ActionRequest{Version: 2, RequestID: b.nextID(), MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: action}
	if err := req.Validate(b.limits); err != nil {
		return err
	}
	if err := b.write(ctx, "action", req.RequestID, req, true); err != nil {
		return err
	}
	return b.await(ctx, "action_ack", req.RequestID)
}

func writeTextNetworkTrace(path string, peers []*textNetworkBot) error {
	type recipient struct {
		PolicySeed int64              `json:"policy_seed"`
		Frames     []textNetworkFrame `json:"frames"`
	}
	trace := struct {
		Version            int         `json:"version"`
		Kind               string      `json:"kind"`
		ProductionEligible bool        `json:"production_eligible"`
		Recipients         []recipient `json:"recipients"`
	}{Version: 1, Kind: "privileged_authenticated_prototype_network_observations"}
	for _, peer := range peers {
		trace.Recipients = append(trace.Recipients, recipient{peer.seed, peer.trace})
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	// A recipient knows only its own credential while recording. Before any
	// file is created, inspect the aggregate against every known credential.
	for _, peer := range peers {
		if textJSONContainsCredential(encoded, peer.token) {
			return errors.New("network trace unexpectedly contained an authentication credential")
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(encoded, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		os.Remove(path)
		return err
	}
	if closeErr != nil {
		os.Remove(path)
	}
	return closeErr
}
