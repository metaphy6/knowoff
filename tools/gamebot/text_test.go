package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestTextBotsAllModesAndSizesReplay(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			a, e := simulateText("../../server/pkg/media/testdata/text-en", "../../configs/gameplay/tuning.yaml", mode, size, 42)
			if e != nil {
				t.Fatal(e)
			}
			b, e := simulateText("../../server/pkg/media/testdata/text-en", "../../configs/gameplay/tuning.yaml", mode, size, 42)
			if e != nil {
				t.Fatal(e)
			}
			x, _ := json.Marshal(a)
			y, _ := json.Marshal(b)
			if string(x) != string(y) {
				t.Fatal("seeded ordered policy replay diverged")
			}
			if a.Outcome != "completed" || len(a.Steps) == 0 || a.Contract.Eligibility.Rewards || a.Contract.Eligibility.Leaderboard {
				t.Fatal("simulation didn't finish with zero live effects")
			}
		}
	}
}

func TestTextCandidateScheduleMatrixReplaysFromPrivateArtifacts(t *testing.T) {
	// The production-candidate command accepts a validated private bundle path.
	// These labelled synthetic languages prove that path and its replay contract,
	// not editorial quality, human outcomes, or exhaustive schedule coverage.
	for _, language := range []string{"en", "tr", "ar"} {
		for _, mode := range gamecontract.AllModes() {
			for _, size := range []int{4, 6} {
				t.Run(fmt.Sprintf("%s/%s/%d", language, mode, size), func(t *testing.T) {
					pack := "../../server/pkg/media/testdata/text-" + language
					tuning := "../../configs/gameplay/tuning.yaml"
					actions, clockSteps, completed := 0, 0, 0
					for _, seed := range []int64{17, 43} {
						original, err := simulateText(pack, tuning, mode, size, seed)
						if err != nil {
							t.Fatal("candidate schedule refused", seed, err)
						}
						if original.Contract.ContentLanguage != language || original.Contract.ModeID != mode || original.Contract.OriginalSize != size || original.Contract.Eligibility.Rewards || original.Contract.Eligibility.Leaderboard || original.Outcome != "completed" || original.Rounds < 1 || original.Rounds > size/2 || len(original.Steps) == 0 {
							t.Fatal("candidate simulation omitted identity or zero-effect completion")
						}
						path := filepath.Join(t.TempDir(), "private.json")
						if err := writeTextScript(path, original); err != nil {
							t.Fatal(err)
						}
						stored, err := readTextScript(path)
						if err != nil {
							t.Fatal(err)
						}
						replayed, err := replayText(pack, tuning, stored)
						if err != nil {
							t.Fatal("persisted ordered replay diverged", seed, err)
						}
						a, err := json.Marshal(original)
						if err != nil {
							t.Fatal(err)
						}
						b, err := json.Marshal(replayed)
						if err != nil || string(a) != string(b) {
							t.Fatal("replay changed pinned contract, ordered inputs or terminal evidence", err)
						}
						completed++
						for _, step := range original.Steps {
							if step.Advance {
								clockSteps++
							} else {
								actions++
							}
						}
					}
					if completed != 2 || actions == 0 || clockSteps == 0 {
						t.Fatal("missing simulation denominator or action/clock coverage")
					}
					t.Logf("synthetic=true scope=sampled_candidate_schedules attempted=2 completed=%d replayed=%d actions=%d clock_steps=%d", completed, completed, actions, clockSteps)
				})
			}
		}
	}
}

func TestGamebotCLIProcess(t *testing.T) {
	if os.Getenv("KNOWOFF_GAMEBOT_CLI_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"gamebot"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}

func gamebotCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestGamebotCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "KNOWOFF_GAMEBOT_CLI_TEST=1", "KNOWOFF_DEV_BOT_KEY=synthetic-cli-refusal-key")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("CLI failed to return within its test bound", ctx.Err())
	}
	return string(out), err
}

func TestGamebotCLIRefusesRetiredAndAmbiguousCommandsBeforeIO(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected CLI network request", http.StatusForbidden)
	}))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/v2"
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"room", []string{"-room", "ABCDEF", "-text-simulate", "top_that"}, "retired"},
		{"queue", []string{"-queue", "4", "-text-simulate", "top_that"}, "retired"},
		{"count", []string{"-count", "0", "-text-simulate", "top_that"}, "retired"},
		{"ambiguous_replay", []string{"-text-simulate", "top_that", "-text-replay", "missing.json"}, "exactly one"},
		{"ambiguous_network", []string{"-text-network", "top_that", "-text-simulate", "top_that"}, "exactly one"},
		{"missing_command", nil, "exactly one"},
		{"positional", []string{"-text-simulate", "top_that", "unexpected"}, "positional"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "private.json")
			args := []string{"-server", endpoint, "-text-pack", "missing-pack", "-text-out", output}
			args = append(args, tc.args...)
			out, err := gamebotCLI(t, args...)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("expected explicit %s refusal, got %v: %s", tc.want, err, out)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) || calls.Load() != 0 {
				t.Fatal("invalid command caused file/network effects", err, calls.Load())
			}
		})
	}
}

func TestGamebotCLISimulationAndReplayPreservePrivateArtifact(t *testing.T) {
	pack, tuning := "../../server/pkg/media/testdata/text-en", "../../configs/gameplay/tuning.yaml"
	output := filepath.Join(t.TempDir(), "private.json")
	out, err := gamebotCLI(t, "-text-simulate", "top_that", "-text-size", "4", "-seed", "42", "-text-pack", pack, "-text-tuning", tuning, "-text-out", output)
	if err != nil {
		t.Fatal(out, err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("CLI artifact is not private", err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	script, err := readTextScript(output)
	if err != nil || script.Outcome != "completed" || script.Contract.Eligibility.Rewards || script.Contract.Eligibility.Leaderboard {
		t.Fatal("CLI bypassed zero-effect simulation", err)
	}
	out, err = gamebotCLI(t, "-text-replay", output, "-text-pack", pack, "-text-tuning", tuning)
	if err != nil || !strings.Contains(out, "text replay verified") {
		t.Fatal(out, err)
	}
	after, err := os.ReadFile(output)
	if err != nil || string(before) != string(after) {
		t.Fatal("replay changed the private artifact", err)
	}
}

func TestGamebotCLIRejectsScopeFlagsAndDefaultsToV2(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"simulation_server", []string{"-text-simulate", "top_that", "-server", "ws://127.0.0.1:1/ws/v2"}, "only valid"},
		{"network_pack", []string{"-text-network", "top_that", "-text-pack", "missing"}, "incompatible"},
		{"network_tuning", []string{"-text-network", "top_that", "-text-tuning", "missing"}, "incompatible"},
		{"replay_output", []string{"-text-replay", "missing", "-text-out", "missing"}, "incompatible"},
		{"replay_size", []string{"-text-replay", "missing", "-text-size", "4"}, "incompatible"},
		{"replay_seed", []string{"-text-replay", "missing", "-seed", "42"}, "incompatible"},
		{"invalid_mode", []string{"-text-network", "association"}, "valid text mode"},
		{"invalid_size", []string{"-text-network", "top_that", "-text-size", "5"}, "valid text mode"},
		{"empty_command", []string{"-text-simulate="}, "exactly one"},
		{"empty_second_command", []string{"-text-simulate=top_that", "-text-network="}, "exactly one"},
		{"missing_output", []string{"-text-simulate", "top_that"}, "-text-out is required"},
		{"missing_pack", []string{"-text-replay", "missing"}, "-text-pack is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := gamebotCLI(t, tc.args...)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("expected %s, got %v: %s", tc.want, err, out)
			}
		})
	}
	out, err := gamebotCLI(t, "-h")
	if err != nil || !strings.Contains(out, "ws://127.0.0.1:8080/ws/v2") {
		t.Fatal("default endpoint is not the literal-loopback v2 endpoint", out, err)
	}
}

func TestTextOrderedReplayRejectsChangedInput(t *testing.T) {
	path := "../../server/pkg/media/testdata/text-en"
	tuning := "../../configs/gameplay/tuning.yaml"
	original, err := simulateText(path, tuning, gamecontract.ModeBadBargains, 6, 42)
	if err != nil {
		t.Fatal(err)
	}
	again, err := replayText(path, tuning, original)
	if err != nil || again.EvidenceSHA256 != original.EvidenceSHA256 {
		t.Fatal("ordered replay failed", err)
	}
	for i := range original.Steps {
		if original.Steps[i].Request != nil {
			original.Steps[i].Request.MatchID = "tampered"
			break
		}
	}
	if _, err = replayText(path, tuning, original); err == nil {
		t.Fatal("tampered replay accepted")
	}
}

func TestTextReplayRejectsTamperedTerminalSummary(t *testing.T) {
	path := "../../server/pkg/media/testdata/text-en"
	tuning := "../../configs/gameplay/tuning.yaml"
	script, err := simulateText(path, tuning, gamecontract.ModeMissedTheBriefing, 4, 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"outcome", "rounds"} {
		bad := script
		if field == "outcome" {
			bad.Outcome = "interrupted"
		} else {
			bad.Rounds = 99
		}
		if _, err = replayText(path, tuning, bad); err == nil {
			t.Fatalf("verified tampered %s", field)
		}
	}
}

func TestTextReplayArtifactExclusivePrivateAndStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.json")
	script := textSimulation{Seed: 42}
	if err := writeTextScript(path, script); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("replay permissions are not private")
	}
	if err = writeTextScript(path, textSimulation{Seed: 99}); err == nil {
		t.Fatal("overwrote existing replay")
	}
	got, err := readTextScript(path)
	if err != nil || got.Seed != 42 {
		t.Fatal("changed existing replay")
	}
	if err = os.WriteFile(path, []byte(`{"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readTextScript(path); err == nil {
		t.Fatal("unknown replay field accepted")
	}
}
