package textcert

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/knowoff/knowoff/server/pkg/media"
	"os"
	"reflect"
	"testing"
	"time"
)

func fixture(t *testing.T) (*media.TextSnapshot, Tuning) {
	t.Helper()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tuning, err := DecodeTuning(raw)
	if err != nil {
		t.Fatal(err)
	}
	s, err := media.LoadTextPack("../media/testdata/text-en", Limits(tuning))
	if err != nil {
		t.Fatal(err)
	}
	return s, tuning
}
func TestActionsDeterministicFullScheduleAndCoverage(t *testing.T) {
	s, tuning := fixture(t)
	first, err := Certify(s, tuning, 1, 71)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Certify(s, tuning, 1, 71)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("non-deterministic certificate", err)
	}
	if len(first.Cells) != 10 || first.Scope != media.TextActionScope {
		t.Fatal("incorrect scope/cells")
	}
	raw, _ := json.Marshal(first)
	if err := Verify(s, tuning, raw); err != nil {
		t.Fatal(err)
	}
	for _, c := range first.Cells {
		if len(c.Witnesses) != 2 {
			t.Fatal("missing branch witness")
		}
		for _, w := range c.Witnesses {
			if w.Rounds != c.TableSize/2 {
				t.Fatal("not full schedule")
			}
		}
	}
}
func TestActionsFailClosed(t *testing.T) {
	s, tuning := fixture(t)
	cert, err := Certify(s, tuning, 1, 71)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing-cell", "duplicate-cell", "history", "step", "scope", "algorithm", "budget", "tuning", "missing-branch", "rounds", "clock", "foreign-seat", "missing-request"} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(cert)
			var changed media.TextActionEvidence
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing-branch":
				changed.Cells[0].Witnesses = changed.Cells[0].Witnesses[:1]
			case "rounds":
				changed.Cells[0].Witnesses[0].Rounds = 1
			case "clock":
				changed.Cells[0].Witnesses[0].Steps[0].AtMS = -1
			case "foreign-seat":
				changed.Cells[0].Witnesses[0].Steps[0].Seat = 99
			case "missing-request":
				changed.Cells[0].Witnesses[0].Steps[0].Seat = 0
			case "missing-cell":
				changed.Cells = changed.Cells[1:]
			case "duplicate-cell":
				changed.Cells[0] = changed.Cells[1]
			case "history":
				changed.Cells[0].Witnesses[0].HistorySHA256 = media.ContentHash([]byte("forged"))
			case "step":
				changed.Cells[0].Witnesses[0].Steps[0].AtMS++
			case "scope":
				changed.Scope = "exhaustive-production"
			case "algorithm":
				changed.Algorithm = "unknown"
			case "budget":
				changed.Samples = tuning.TextCatalog.MaxSearchNodes + 1
			case "tuning":
				changed.TuningSHA256 = media.ContentHash([]byte("forged"))
			}
			raw, _ = json.Marshal(changed)
			if Verify(s, tuning, raw) == nil {
				t.Fatal("tampered evidence accepted")
			}
		})
	}
	if _, err := Certify(s, tuning, 1, 0); err == nil {
		t.Fatal("nondeterministic zero seed")
	}
	tuning.Contract.MaxHistoryEvents = 1
	if _, err := Certify(s, tuning, 1, 71); err == nil {
		t.Fatal("exhausted budget accepted")
	}
}

func TestActionsRejectPolicyDriftAndMalformedArtifacts(t *testing.T) {
	s, tuning := fixture(t)
	cert, err := Certify(s, tuning, 1, 71)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cert)
	drift := tuning.Clone()
	drift.Timers.TradeResponseS++
	if Verify(s, drift, raw) == nil {
		t.Fatal("full action tuning drift accepted")
	}
	tiny := tuning.Clone()
	tiny.TextCatalog.MaxFileBytes = int64(len(raw) - 1)
	if Verify(s, tiny, raw) == nil {
		t.Fatal("artifact byte bound ignored")
	}
	tiny = tuning.Clone()
	tiny.TextCatalog.MaxSearchNodes = 1
	if Verify(s, tiny, raw) == nil {
		t.Fatal("aggregate replay bound ignored")
	}
	for _, malformed := range [][]byte{nil, []byte(`{}`), append(append([]byte(nil), raw...), []byte(` {}`)...), append([]byte(`{"schema_version":2,`), raw[1:]...), append([]byte(`{"unexpected":true,`), raw[1:]...)} {
		if Verify(s, tuning, malformed) == nil {
			t.Fatal("malformed/unknown/duplicate JSON accepted")
		}
	}
	if ValidateActivation(s, s.Manifest().RulesVersion, tuning) == nil {
		t.Fatal("technical success bypassed human/synthetic release gate")
	}
}

func TestActionsEmptySnapshotRefused(t *testing.T) {
	_, tuning := fixture(t)
	if _, err := Certify(&media.TextSnapshot{}, tuning, 1, 71); err == nil {
		t.Fatal("empty snapshot accepted")
	}
}

func TestActionsAllFixtureLanguages(t *testing.T) {
	_, tuning := fixture(t)
	for _, language := range []string{"tr", "ar"} {
		t.Run(language, func(t *testing.T) {
			snapshot, err := media.LoadTextPack("../media/testdata/text-"+language, Limits(tuning))
			if err != nil {
				t.Fatal(err)
			}
			evidence, err := Certify(snapshot, tuning, 1, 71)
			if err != nil {
				t.Fatal(err)
			}
			if evidence.Language != language || len(evidence.Cells) != 10 {
				t.Fatal("language/cell coverage")
			}
			raw, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			if err := Verify(snapshot, tuning, raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Deterministically cancel while the runner checks successive engine steps;
// no scheduler timing or retry-until-green is needed for this regression.
type stepCancellation struct {
	context.Context
	checks int
}

func (c *stepCancellation) Err() error {
	c.checks++
	if c.checks >= 20 {
		return context.Canceled
	}
	return nil
}
func TestActionsContextCancellation(t *testing.T) {
	s, tuning := fixture(t)
	expired, finish := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer finish()
	if _, err := CertifyContext(expired, nil, tuning, 0, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired context did work", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CertifyContext(canceled, nil, tuning, 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not precede work", err)
	}
	if err := VerifyContext(canceled, nil, tuning, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("verification ignored cancellation", err)
	}
	if err := ValidateActivationContext(canceled, nil, "", tuning); !errors.Is(err, context.Canceled) {
		t.Fatal("activation ignored cancellation", err)
	}
	ctx := &stepCancellation{Context: context.Background()}
	result, err := CertifyContext(ctx, s, tuning, 20, 71)
	if !errors.Is(err, context.Canceled) || ctx.checks != 20 || len(result.Cells) != 0 {
		t.Fatal("partial work/certificate survived cancellation", ctx.checks, err)
	}
}
