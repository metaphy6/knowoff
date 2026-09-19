package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

const frozenPolicyV1SHA256 = "aeeb1bbfc2bc881635a04d504aa61a73168ea1ed36b00565dee44a5b6778fb7b"

func TestHistoricalTuningPolicyCanonicalIdentity(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.TrimSpace(raw)
	var reordered map[string]any
	if err := json.Unmarshal(raw, &reordered); err != nil {
		t.Fatal(err)
	}
	sorted, err := json.Marshal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{raw, sorted} {
		var policy TuningConfig
		if err := json.Unmarshal(input, &policy); err != nil {
			t.Fatal(err)
		}
		for _, copy := range []TuningConfig{policy, policy.Clone()} {
			got, err := json.Marshal(copy)
			if err != nil || !bytes.Equal(got, raw) {
				t.Fatalf("version1 canonical bytes changed: %s %v", got, err)
			}
			hash, err := copy.SHA256()
			if err != nil || hash != frozenPolicyV1SHA256 {
				t.Fatalf("version1 hash changed: %s %v", hash, err)
			}
		}
		copy := policy.Clone()
		if copy.retiredPolicy != nil {
			copy.Hand.SpecialtyWeights["shuffle"] = 91
			if policy.Hand.SpecialtyWeights["shuffle"] != 0.75 {
				t.Fatal("cloned historical policy aliases original")
			}
		}
		copy.Game.DonowersBySize[4] = 2
		copy.Noin.CorrectVote = 37
		copy.Economy.PlayPassPrices["day_1"] = 431
		copy.Progression.LevelThresholds[0] = 100
		if policy.Game.DonowersBySize[4] != 1 || policy.Noin.CorrectVote != 5 || policy.Economy.PlayPassPrices["day_1"] == 431 || policy.Progression.LevelThresholds[0] == 100 {
			t.Fatal("cloned live policy aliases original")
		}
		encoded, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		var replay TuningConfig
		if err := json.Unmarshal(encoded, &replay); err != nil {
			t.Fatal(err)
		}
		if replay.Noin.CorrectVote != 37 || replay.Game.DonowersBySize[4] != 2 {
			t.Fatal("historical metadata overwrote live policy values")
		}
	}
}

func TestHistoricalTuningPolicyUnknownFieldsRefuseWithoutChangingReceiver(t *testing.T) {
	raw, err := os.ReadFile("testdata/policy_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(p map[string]any) { p["UnrecognizedFuturePolicy"] = 99 },
		func(p map[string]any) { p["Noin"].(map[string]any)["UnrecognizedFutureAward"] = 99 },
	} {
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		mutate(fields)
		bad, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		var policy TuningConfig
		policy.Noin.CorrectVote = 731
		if err := json.Unmarshal(bad, &policy); err == nil {
			t.Fatal("unknown historical policy was silently discarded")
		}
		if policy.Noin.CorrectVote != 731 {
			t.Fatal("refused decode partially changed policy")
		}
	}
}

func TestHistoricalTuningPolicyOmissionAndNilCollections(t *testing.T) {
	var policy TuningConfig
	before, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("VoteResultFalling")) {
		t.Fatal("optional zero field was added to historical identity")
	}
	if err := json.Unmarshal(before, &policy); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(policy.Clone())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("nil or omitted historical fields changed: %v", err)
	}
}
