package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/lobby"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestManualCLIAuthenticatesInsteadOfRefusingRoom(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	t.Setenv("KNOWOFF_DEV_BOT_KEY", "fixture-key")
	if got := runGamebot([]string{"-room", "ABCDEF", "-count", "3", "-server", "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/v2"}); got != 1 || calls != 1 {
		t.Fatalf("manual command never reached authenticated admission: exit=%d calls=%d", got, calls)
	}
}

func TestManualBotsHumanRoomAndRematch(t *testing.T) {
	f := newNetworkFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	endpoint := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/ws/v2"
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				response, err := http.Post(f.server.URL+"/api/auth/device", "application/json", strings.NewReader(fmt.Sprintf(`{"device_hash":"manual-human-%s-%d"}`, mode, size)))
				if err != nil {
					t.Fatal(err)
				}
				var pair struct {
					AccessToken string `json:"access_token"`
				}
				err = json.NewDecoder(response.Body).Decode(&pair)
				response.Body.Close()
				if err != nil || pair.AccessToken == "" {
					t.Fatal("human authentication", err)
				}
				host, err := connectTextNetwork(ctx, endpoint, pair.AccessToken, 42)
				if err != nil {
					t.Fatal(err)
				}
				defer host.close()
				settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
				if err = host.control(ctx, "room_create", settings); err != nil {
					t.Fatal(err)
				}
				peers, err := joinManualBots(ctx, endpoint, host.lobby.Code, size-1, 100, "disposable-network-fixture-key-for-local-tests")
				if err != nil {
					t.Fatal(err)
				}
				defer closeManualBots(peers)
				for match := 0; match < 2; match++ {
					for _, p := range peers {
						if _, err := manualBotStep(ctx, p); err != nil {
							t.Fatal(err)
						}
					}
					if err = host.control(ctx, "resync", struct{}{}); err != nil {
						t.Fatal(err)
					}
					for _, seat := range host.lobby.Lobby.Seats {
						if seat.Seat != host.lobby.Seat && seat.Ready == nil {
							t.Fatal("bot did not ready", seat.Seat)
						}
					}
					l := host.lobby.Lobby
					if err = host.control(ctx, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: l.SettingsRevision, MembershipRevision: l.MembershipRevision}); err != nil {
						t.Fatal(err)
					}
					if err = host.control(ctx, "room_start", struct{}{}); err != nil {
						t.Fatal(err)
					}
					finished := false
					for step := 0; step < 500; step++ {
						if err = host.control(ctx, "resync", struct{}{}); err != nil {
							t.Fatal(err)
						}
						if host.snapshot.Phase == v2.PhaseVerdict {
							finished = true
							break
						}
						acted := false
						if action := textPolicy(host.snapshot, host.rng); action != nil {
							if err = host.act(ctx, *action); err != nil {
								t.Fatal(err)
							}
							acted = true
						}
						for _, p := range peers {
							a, e := manualBotStep(ctx, p)
							if e != nil {
								t.Fatal(e)
							}
							acted = acted || a
						}
						if !acted {
							if err = host.control(ctx, "resync", struct{}{}); err != nil {
								t.Fatal(err)
							}
							if host.snapshot.Phase != v2.PhaseVerdict {
								if err = f.advance(ctx, host.snapshot.DeadlineMS); err != nil {
									t.Fatal(err)
								}
							}
						}
					}
					if !finished {
						t.Fatal("mixed human/bot match did not finish")
					}
					for _, p := range peers {
						if _, err = manualBotStep(ctx, p); err != nil {
							t.Fatal(err)
						}
						if p.snapshot.Phase != v2.PhaseVerdict {
							t.Fatal("bot missed verdict")
						}
					}
					if match == 0 {
						if err = host.control(ctx, "rematch", struct{}{}); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err = host.control(ctx, "room_leave", struct{}{}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	networkZero(t, f.db, `SELECT count(*) FROM noin_ledger`)
	networkZero(t, f.db, `SELECT count(*) FROM text_matches WHERE contract#>>'{Contract,eligibility,rewards}' <> 'false' OR contract#>>'{Contract,eligibility,leaderboard}' <> 'false'`)
}

func TestManualRematchClearsOnlyTerminalSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		phase                v2.Phase
		settings, membership uint64
		cleared              bool
	}{
		{"terminal rematch", v2.PhaseVerdict, 2, 3, true},
		{"active match", v2.PhasePlay, 2, 3, false},
		{"unchanged revisions", v2.PhaseVerdict, 1, 2, false},
		{"only settings changed", v2.PhaseVerdict, 2, 2, false},
		{"only membership changed", v2.PhaseVerdict, 1, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := &textNetworkBot{lobby: lobby.TextLobbyView{Lobby: v2.LobbyState{SettingsRevision: 1, MembershipRevision: 2}}, snapshot: v2.Snapshot{Version: 2, Phase: tc.phase}}
			raw, _ := json.Marshal(lobby.TextLobbyView{Lobby: v2.LobbyState{SettingsRevision: tc.settings, MembershipRevision: tc.membership}})
			if err := peer.receive(lobby.TextEnvelope{Type: "lobby", Payload: raw}); err != nil {
				t.Fatal(err)
			}
			if (peer.snapshot.Version == 0) != tc.cleared {
				t.Fatalf("snapshot cleared=%t, want %t", peer.snapshot.Version == 0, tc.cleared)
			}
			if !tc.cleared && peer.snapshot.Phase != tc.phase {
				t.Fatal("retained snapshot changed phase")
			}
		})
	}
}

func TestManualCommandInvalidOptionsNeverAuthenticate(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/v2"
	for _, args := range [][]string{{"-room", "ABCDEF", "-count", "0"}, {"-room", "ABCDEF", "-count", "6"}, {"-room", "bad"}, {"-room", "ABCDEF", "-text-network", "top_that"}, {"-room", "ABCDEF", "-text-pack", "x"}, {"-room", "ABCDEF", "-text-size", "4"}, {"-count", "3"}} {
		if got := runGamebot(append(args, "-server", endpoint)); got != 2 {
			t.Fatalf("invalid args %v exit=%d", args, got)
		}
	}
	if calls != 0 {
		t.Fatal("invalid manual options caused authentication")
	}
}

func TestManualRaceErrorsOnlyRetryStaleIntents(t *testing.T) {
	for _, code := range []string{"lobby.stale_revision", string(v2.ErrStalePhase), string(v2.ErrStaleRevision), string(v2.ErrDeadlineExpired)} {
		if err := manualRaceError(fmt.Errorf("server rejected network request: %s", code)); err != nil {
			t.Fatal(err)
		}
	}
	for _, code := range []string{"action.unauthorized", "protocol.malformed", "request.limit", "mode.unavailable"} {
		if manualRaceError(fmt.Errorf("server rejected network request: %s", code)) == nil {
			t.Fatal("swallowed real failure", code)
		}
	}
}

func TestManualRunnerWaitsForHumanStartAndLeavesOnCancel(t *testing.T) {
	f := newNetworkFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	endpoint := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/ws/v2"
	token, err := networkDevelopmentAuth(ctx, endpoint, "disposable-network-fixture-key-for-local-tests")
	if err != nil {
		t.Fatal(err)
	}
	host, err := connectTextNetwork(ctx, endpoint, token, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer host.close()
	if err = host.control(ctx, "room_create", v2.LobbySettings{ModeID: gamecontract.ModeTopThat, Size: 4, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}); err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	result := make(chan error, 1)
	code := host.lobby.Code
	go func() {
		result <- runManualBots(runCtx, endpoint, code, 3, 5, "disposable-network-fixture-key-for-local-tests")
	}()
	ready := false
	for !ready {
		if err = host.control(ctx, "resync", struct{}{}); err != nil {
			t.Fatal(err)
		}
		ready = len(host.lobby.Lobby.Seats) == 4
		for _, seat := range host.lobby.Lobby.Seats {
			if seat.Seat != host.lobby.Seat {
				ready = ready && seat.Ready != nil
			}
		}
		select {
		case err := <-result:
			t.Fatal("companions stopped before ready", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	networkZero(t, f.db, `SELECT count(*) FROM text_matches`)
	stop()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation cause lost", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err = host.control(ctx, "resync", struct{}{}); err != nil {
		t.Fatal(err)
	}
	if len(host.lobby.Lobby.Seats) != 1 {
		t.Fatal("cancelled companions retained lobby seats")
	}
}
