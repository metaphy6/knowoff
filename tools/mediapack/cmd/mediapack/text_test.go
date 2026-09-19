package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/server/pkg/textcert"
	"github.com/knowoff/knowoff/tools/mediapack/internal/generator"
)

func TestMediapackCLIProcess(t *testing.T) {
	if os.Getenv("KNOWOFF_MEDIAPACK_CLI_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"mediapack"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}

func mediapackCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestMediapackCLIProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "KNOWOFF_MEDIAPACK_CLI_TEST=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("CLI exceeded bounded test deadline", ctx.Err())
	}
	return string(out), err
}

func TestMediapackCLIRetiredCommandsRefuseBeforePaths(t *testing.T) {
	for _, command := range []string{"build", "certify", "simulate", "publish"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(root, "output")
			out, err := mediapackCLI(t, command, "-out", output, "missing-pack")
			if err == nil || !strings.Contains(out, "retired") {
				t.Fatalf("expected explicit retired command refusal: %v %s", err, out)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("retired command created output", entries, err)
			}
		})
	}
}

func TestMediapackCLITextFixtureRemainsCanonical(t *testing.T) {
	out := filepath.Join(t.TempDir(), "text-fixture")
	message, err := mediapackCLI(t, "text-build-fixture", "-tuning", "../../../../configs/gameplay/tuning.yaml", "-language", "en", "-rules", "text-v1", "-release", "synthetic-text-en", "-out", out)
	if err != nil {
		t.Fatal(message, err)
	}
	for _, name := range []string{"manifest.json", "media.jsonl", "cards.jsonl", "suitability.jsonl"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("../../../../server/pkg/media/testdata/text-en", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("actual CLI changed canonical %s", name)
		}
	}
}

func TestTextCheckedInFixtureGeneratorParity(t *testing.T) {
	limits, _, err := loadTextTuning("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "tr", "ar"} {
		b, err := generator.SyntheticTextBundle(language, "text-v1", "synthetic-text-"+language, limits)
		if err != nil {
			t.Fatal(err)
		}
		generated := filepath.Join(t.TempDir(), "pack")
		if err := media.WriteTextBundle(generated, b, limits); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"manifest.json", "media.jsonl", "cards.jsonl", "suitability.jsonl"} {
			got, err := os.ReadFile(filepath.Join(generated, name))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("../../../../server/pkg/media/testdata", "text-"+language, name))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("regenerated fixture differs: %s/%s", language, name)
			}
		}
	}
}

func TestTextCommandsUseSharedValidationAndKeepSyntheticUnreleased(t *testing.T) {
	dir := t.TempDir()
	pack := filepath.Join(dir, "pack")
	tuning := "../../../../configs/gameplay/tuning.yaml"
	var output, errors bytes.Buffer
	run := func(command string, args ...string) int {
		t.Helper()
		output.Reset()
		errors.Reset()
		return runTextCommand(command, append([]string{"-tuning", tuning}, args...), &output, &errors)
	}
	if code := run("text-build-fixture", "-out", pack, "-language", "tr", "-rules", "text-v2", "-release", "synthetic-cli-test"); code != 0 {
		t.Fatalf("build=%d %s", code, errors.String())
	}
	if code := run("text-certify", "-samples", "20", "-seed", "71", "-out", filepath.Join(dir, "technical.json"), "-replay-out", filepath.Join(dir, "replay.json"), pack); code != 0 {
		t.Fatalf("certify=%d %s", code, errors.String())
	}
	raw, err := os.ReadFile(filepath.Join(dir, "technical.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report media.TextCertification
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.ProductionReady || !report.Synthetic {
		t.Fatal("synthetic certificate claimed release readiness")
	}
	if bytes.Contains(output.Bytes(), []byte(`"seed"`)) {
		t.Fatal("privileged replay seed entered general output")
	}
	if info, err := os.Stat(filepath.Join(dir, "replay.json")); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("privileged replay file permissions")
	}
	if code := run("text-publish", "-rules", "text-v2", "-out", filepath.Join(dir, "published"), pack); code == 0 {
		t.Fatal("synthetic fixture published for production")
	}
	if _, err := os.Stat(filepath.Join(dir, "published")); !os.IsNotExist(err) {
		t.Fatal("refused publish created output")
	}
	before, err := os.ReadFile(filepath.Join(pack, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if code := run("text-build-fixture", "-out", pack, "-language", "en", "-rules", "text-v2", "-release", "rewrite"); code == 0 {
		t.Fatal("existing immutable bundle overwritten")
	}
	after, err := os.ReadFile(filepath.Join(pack, "manifest.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed preparation changed old manifest")
	}
}

func TestTextCommandsFailClosedOnMissingBoundsAndInvalidInput(t *testing.T) {
	var out, errors bytes.Buffer
	for _, tc := range []struct {
		command string
		args    []string
	}{{"text-prepare", nil}, {"text-unknown", nil}, {"text-certify", []string{"-tuning", "../../../../configs/gameplay/tuning.yaml", "-samples", "0", "missing"}}, {"text-build-fixture", []string{"-tuning", "missing", "-out", t.TempDir()}}} {
		if code := runTextCommand(tc.command, tc.args, &out, &errors); code == 0 {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestTextPreparePreservesAcceptedInputAndRejectsMutation(t *testing.T) {
	limits, _, err := loadTextTuning("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := generator.SyntheticTextBundle("en", "text-v2", "prepare-fixture", limits)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "candidate.json")
	raw, _ := json.Marshal(b)
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errors bytes.Buffer
	prepared := filepath.Join(dir, "prepared")
	args := []string{"-tuning", "../../../../configs/gameplay/tuning.yaml", "-input", input, "-out", prepared}
	if code := runTextCommand("text-prepare", args, &out, &errors); code != 0 {
		t.Fatal(errors.String())
	}
	snapshot, err := media.LoadTextPack(prepared, limits)
	if err != nil {
		t.Fatal(err)
	}
	copied := snapshot.Bundle()
	if copied.Cards[0].Provenance != b.Cards[0].Provenance || copied.Cards[0].Text != b.Cards[0].Text {
		t.Fatal("preparation lost original consent/provenance/wording")
	}
	b.Cards[0].Text = "changed after acceptance"
	raw, _ = json.Marshal(b)
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	args[len(args)-1] = filepath.Join(dir, "mutated")
	if code := runTextCommand("text-prepare", args, &out, &errors); code == 0 {
		t.Fatal("mutated accepted wording was bundled")
	}
}

func TestTextDuplicateCommandInspectsUnapprovedDraftWithoutRewriting(t *testing.T) {
	limits, _, err := loadTextTuning("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := generator.SyntheticTextBundle("en", "text-v2", "duplicate-fixture", limits)
	if err != nil {
		t.Fatal(err)
	}
	b.Cards[0].Text = "Spare key"
	b.Cards[1].Text = "SPARE  KEY!"
	raw, _ := json.Marshal(b)
	dir := t.TempDir()
	input := filepath.Join(dir, "draft.json")
	output := filepath.Join(dir, "duplicates.json")
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runTextCommand("text-duplicates", []string{"-tuning", "../../../../configs/gameplay/tuning.yaml", "-input", input, "-out", output}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	result, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var candidates []media.TextDuplicateCandidate
	if err := json.Unmarshal(result, &candidates); err != nil || len(candidates) != 1 || candidates[0].Reviewed {
		t.Fatal("draft comparison invented review")
	}
	after, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("duplicate inspection rewrote draft")
	}
}

func TestTextPublishIsImmutableAndIdempotent(t *testing.T) {
	limits, tuning, err := loadTextTuning("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := generator.SyntheticTextBundle("en", "text-v2", "simulated-publication-test", limits)
	if err != nil {
		t.Fatal(err)
	}
	// Simulated gate records are unit-test fixtures, never real editorial proof.
	b.Manifest.Synthetic = false
	fill := func(p *media.TextProvenance) {
		p.SourceKind = "contribution"
		p.TermsVersion = "simulated-terms"
		p.ConsentReference = "simulated-consent"
		p.ConsentAtMS = 1
		p.ApprovalReference = "simulated-approval"
		p.EditorReference = "simulated-editor"
		p.ReviewedAtMS = 2
	}
	for i := range b.Nowns {
		fill(&b.Nowns[i].Provenance)
	}
	for i := range b.Cards {
		fill(&b.Cards[i].Provenance)
	}
	b, err = media.SealTextBundle(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := media.NewTextSnapshot(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	fullTuning, err := loadActionTuning("../../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	actionEvidence, err := textcert.Certify(snapshot, fullTuning, 1, 71)
	if err != nil {
		t.Fatal(err)
	}
	b.Artifacts["action-replay.json"], _ = json.Marshal(actionEvidence)
	report := media.CertifyText(snapshot, tuning, 20, 71)
	b.Artifacts["technical.json"], _ = json.Marshal(report)
	b.Artifacts["replay.json"], _ = json.Marshal(media.NewTextReplay(snapshot, tuning, 20, 71))
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		e := media.TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: snapshot.SHA256(), RulesVersion: "text-v2", Language: "en", Gate: gate, ActorReference: "simulated-editor", TuningSHA256: media.TextTuningSHA256(tuning)}
		for _, mode := range b.Manifest.Modes {
			for _, size := range []int{4, 6} {
				e.Cells = append(e.Cells, media.TextGateCell{Mode: mode, TableSize: size, Passed: true, RecordReference: "simulated-record", EvidenceSHA256: media.ContentHash([]byte("simulated evidence"))})
			}
		}
		b.Artifacts[gate+".json"], _ = json.Marshal(e)
	}
	b.Manifest.CertificationArtifacts = map[string]string{}
	for name, raw := range b.Artifacts {
		b.Manifest.CertificationArtifacts[name] = media.ContentHash(raw)
	}
	b, err = media.SealTextBundle(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "published")
	if err := media.WriteTextBundle(source, b, limits); err != nil {
		t.Fatal(err)
	}
	var out, errors bytes.Buffer
	args := []string{"-tuning", "../../../../configs/gameplay/tuning.yaml", "-rules", "text-v2", "-out", target, source}
	for attempt := 0; attempt < 2; attempt++ {
		if code := runTextCommand("text-publish", args, &out, &errors); code != 0 {
			t.Fatal(errors.String())
		}
	}
	first, err := media.LoadTextPack(target, limits)
	if err != nil {
		t.Fatal(err)
	}
	if first.ManifestSHA256() != snapshotWithArtifacts(t, b, limits).ManifestSHA256() {
		t.Fatal("published artifact changed")
	}
	if err := os.WriteFile(filepath.Join(target, "manifest.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runTextCommand("text-publish", args, &out, &errors); code == 0 {
		t.Fatal("conflicting destination overwritten")
	}
	still, err := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err != nil || string(still) != "{}" {
		t.Fatal("refused publication changed destination")
	}
}

func snapshotWithArtifacts(t *testing.T, b media.TextBundle, l media.TextLimits) *media.TextSnapshot {
	t.Helper()
	s, err := media.NewTextSnapshot(b, l)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTextActionCommandProducesPrivateExecutableEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "action-replay.json")
	var out, errors bytes.Buffer
	args := []string{"-tuning", "../../../../configs/gameplay/tuning.yaml", "-samples", "1", "-seed", "71", "-out", path, "../../../../server/pkg/media/testdata/text-en"}
	if code := runTextCommand("text-actions", args, &out, &errors); code != 0 {
		t.Fatal(code, errors.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evidence media.TextActionEvidence
	if err = json.Unmarshal(raw, &evidence); err != nil || len(evidence.Cells) != 10 {
		t.Fatal("invalid executable evidence", err)
	}
	if bytes.Contains(out.Bytes(), []byte(`"seed"`)) || bytes.Contains(out.Bytes(), []byte(`"steps"`)) {
		t.Fatal("private action inputs entered logs")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("private artifact permissions", err)
	}
	if code := runTextCommand("text-actions", args, &out, &errors); code == 0 {
		t.Fatal("immutable evidence overwritten")
	}
}
