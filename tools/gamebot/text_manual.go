package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

var manualRoomCode = regexp.MustCompile(`^[A-Z0-9]{6}$`)

// Each companion owns a fresh development identity. It never takes over a
// human seat, creates a room, starts a match, or initiates a rematch.
func joinManualBots(ctx context.Context, endpoint, code string, count int, seed int64, key string) ([]*textNetworkBot, error) {
	if !manualRoomCode.MatchString(code) || count < 1 || count > 5 {
		return nil, errors.New("manual bots require -room CODE and -count 1..5")
	}
	if err := textNetworkEndpoint(endpoint); err != nil {
		return nil, err
	}
	peers := make([]*textNetworkBot, 0, count)
	for i := 0; i < count; i++ {
		token, err := networkDevelopmentAuth(ctx, endpoint, key)
		if err != nil {
			closeManualBots(peers)
			return nil, err
		}
		p, err := connectTextNetwork(ctx, endpoint, token, seed+int64(i))
		if err != nil {
			closeManualBots(peers)
			return nil, err
		}
		// Refresh retries return fresh authorized state while retaining one receipt.
		p.resyncRequestID = p.nextID()
		peers = append(peers, p)
		if err = p.control(ctx, "room_join", map[string]string{"code": code}); err != nil {
			closeManualBots(peers)
			return nil, err
		}
	}
	return peers, nil
}

func closeManualBots(peers []*textNetworkBot) {
	for _, p := range peers {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		// Best effort release; socket closure also triggers authoritative disconnect.
		_ = p.control(ctx, "room_leave", struct{}{})
		cancel()
		p.close()
	}
}

func manualBotStep(ctx context.Context, p *textNetworkBot) (bool, error) {
	// Interactive sessions are not privileged replay artifacts. Keep only the
	// current exchange in memory rather than retaining every human interaction.
	p.trace = nil
	p.traceBytes = 0
	if err := p.control(ctx, "resync", struct{}{}); err != nil {
		return false, err
	}
	if p.snapshot.Version == 0 {
		l := p.lobby.Lobby
		for _, seat := range l.Seats {
			if seat.Seat == p.lobby.Seat && seat.Ready != nil {
				return false, nil
			}
		}
		err := p.control(ctx, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: l.SettingsRevision, MembershipRevision: l.MembershipRevision})
		return false, manualRaceError(err)
	}
	if p.snapshot.Phase == v2.PhaseVerdict {
		for id := range p.deliveries {
			if err := p.control(ctx, "settlement_ack", map[string]int64{"id": id}); err != nil {
				return false, err
			}
			delete(p.deliveries, id)
		}
		return false, nil
	}
	action := textPolicy(p.snapshot, p.rng)
	if action == nil {
		return false, nil
	}
	err := p.act(ctx, *action)
	return err == nil, manualRaceError(err)
}

// Human input and deadline ticks can win between a resync and an intent. The
// server rejects the stale intent; the next bounded poll fetches fresh state.
func manualRaceError(err error) error {
	if err == nil {
		return nil
	}
	for _, code := range []string{"lobby.stale_revision", string(v2.ErrStalePhase), string(v2.ErrStaleRevision), string(v2.ErrDeadlineExpired)} {
		if strings.HasSuffix(err.Error(), "server rejected network request: "+code) {
			return nil
		}
	}
	return err
}

func runManualBots(ctx context.Context, endpoint, code string, count int, seed int64, key string) error {
	peers, err := joinManualBots(ctx, endpoint, code, count, seed, key)
	if err != nil {
		return err
	}
	defer closeManualBots(peers)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for i, p := range peers {
			if _, err := manualBotStep(ctx, p); err != nil {
				return fmt.Errorf("bot %d: %w", i+1, err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
