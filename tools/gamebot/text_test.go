package main

import (
	"encoding/json"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"os"
	"path/filepath"
	"testing"
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
