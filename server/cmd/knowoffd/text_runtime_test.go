package main

import (
	"context"
	"errors"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
	"time"
)

func TestTextRuntimeRecoveryMustDrainBeforeAdmission(t *testing.T) {
	calls := 0
	err := recoverTextRuntime(context.Background(), func(ctx context.Context, limit int) (store.TextOwnerRecovery, error) {
		calls++
		if limit < 1 || limit > 1000 {
			t.Fatal("unbounded recovery")
		}
		return store.TextOwnerRecovery{Done: calls == 3}, nil
	})
	if err != nil || calls != 3 {
		t.Fatal("opened before all recovery batches", calls, err)
	}
	expected := errors.New("ownership unavailable")
	if err := recoverTextRuntime(context.Background(), func(context.Context, int) (store.TextOwnerRecovery, error) {
		return store.TextOwnerRecovery{}, expected
	}); !errors.Is(err, expected) {
		t.Fatal("recovery failure ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if err := recoverTextRuntime(ctx, func(context.Context, int) (store.TextOwnerRecovery, error) {
		calls++
		return store.TextOwnerRecovery{}, nil
	}); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("cancelled startup attempted recovery")
	}
}

type fakeTextDrain struct {
	drained           bool
	active            int
	closed            bool
	closeContextAlive bool
}

func (f *fakeTextDrain) Drain()             { f.drained = true }
func (f *fakeTextDrain) ActiveMatches() int { return f.active }
func (f *fakeTextDrain) Close(ctx context.Context) error {
	f.closed = true
	f.closeContextAlive = ctx.Err() == nil
	return nil
}
func TestTextRuntimeDrainUsesFreshCompensationDeadline(t *testing.T) {
	f := &fakeTextDrain{active: 1}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := drainTextRuntime(ctx, f); err != nil {
		t.Fatal(err)
	}
	if !f.drained || !f.closed || !f.closeContextAlive {
		t.Fatal("grace expiry prevented durable interruption")
	}
}
func TestTextRuntimePrototypeRefusedBeforeDatabaseAccess(t *testing.T) {
	cfg := &config.Config{}
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	path := "../../pkg/media/testdata/text-en"
	cfg.App.Env = "test"
	prototype, err := loadTextPrototype(cfg, path)
	if err != nil || prototype == nil || !prototype.Manifest().Synthetic {
		t.Fatal("synthetic prototype fixture did not load", err)
	}
	for _, env := range []string{"prod", "production"} {
		cfg.App.Env = env
		if _, err := loadTextPrototype(cfg, path); err == nil {
			t.Fatal("production prototype accepted")
		}
	}
	cfg.App.Env = "test"
	if prototype, err := loadTextPrototype(cfg, ""); err != nil || prototype != nil {
		t.Fatal("empty server prototype flag inferred a pack")
	}
}
