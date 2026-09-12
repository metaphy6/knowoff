package main

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/media"
)

type textRuntime struct {
	Owner     *store.TextOwner
	Values    *store.TextValueStore
	Releases  *store.TextReleaseStore
	Trust     *store.TextTrustStore
	Lobby     *lobby.TextManager
	closeOnce sync.Once
	closeErr  error
}

// No listener or worker is opened until the confirmed old owner work is closed.
// The optional prototype path is a server environment choice, never client input.
func newTextRuntime(ctx context.Context, db *sql.DB, cfg *config.Config, prototypePath string) (out *textRuntime, err error) {
	if cfg == nil || cfg.Text == nil || db == nil {
		return nil, lobby.ErrTextUnavailable
	}
	prototype, err := loadTextPrototype(cfg, prototypePath)
	if err != nil {
		return nil, err
	}
	screener := portal.NewTextScreener(cfg.Moderation.ContentScreening)
	var screen func(context.Context, string) error
	if screener != nil {
		screen = screener.ScreenText
	}
	releases := store.NewTextReleaseStore(db, cfg.Tuning, screen)
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = owner.Release(cleanup)
		}
	}()
	values, err := store.NewTextValueStore(db, cfg.Tuning).WithStartGuard(releases.ValidateStart).WithOwner(owner)
	if err != nil {
		return nil, err
	}
	if err = recoverTextRuntime(ctx, func(ctx context.Context, limit int) (store.TextOwnerRecovery, error) {
		return owner.RecoverLostOwners(ctx, values, limit)
	}); err != nil {
		return nil, err
	}
	trust := store.NewTextTrustStore(db)
	manager, err := lobby.NewTextManager(lobby.TextDeps{
		Owner: owner.Token().IncarnationID, Authority: owner, Config: cfg, Values: values, Prototype: prototype,
		Resolve: releases.Resolve, ResolveRelease: releases.ResolveRelease,
		CanMatch: func(ctx context.Context, accounts []string) error { return trust.CanMatch(ctx, accounts, time.Now()) },
		CheckAccess: func(ctx context.Context, account string, settings v2.LobbySettings, path string) error {
			return releases.CheckAccess(ctx, account, settings, path, time.Now())
		},
		ModerateChat: game.NewTextChatModerator(cfg.Moderation.WordLists, func(ctx context.Context, account string) error {
			return trust.RequireTerms(ctx, account, cfg.Trust.UserTermsVersion)
		}),
		HideChat: func(ctx context.Context, viewer, author string) (bool, error) {
			visible, err := trust.VisibleAuthored(ctx, viewer, author)
			return !visible, err
		},
	})
	if err != nil {
		return nil, err
	}
	return &textRuntime{Owner: owner, Values: values, Releases: releases, Trust: trust, Lobby: manager}, nil
}
func recoverTextRuntime(ctx context.Context, batch func(context.Context, int) (store.TextOwnerRecovery, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := batch(ctx, 100)
		if err != nil {
			return err
		}
		if result.Done {
			return nil
		}
	}
}

type textDrainer interface {
	Drain()
	ActiveMatches() int
	Close(context.Context) error
}

func drainTextRuntime(ctx context.Context, m textDrainer) error {
	m.Drain()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for m.ActiveMatches() > 0 {
		select {
		case <-ctx.Done():
			goto close
		case <-ticker.C:
		}
	}
close:
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return m.Close(cleanup)
}
func (r *textRuntime) Close(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.closeErr = drainTextRuntime(ctx, r.Lobby)
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r.closeErr = errors.Join(r.closeErr, r.Owner.Release(cleanup))
	})
	return r.closeErr
}

func loadTextPrototype(cfg *config.Config, path string) (*media.TextSnapshot, error) {
	if cfg == nil {
		return nil, lobby.ErrTextUnavailable
	}
	if path == "" {
		return nil, nil
	}
	if cfg.App.Env == "prod" || cfg.App.Env == "production" {
		return nil, lobby.ErrTextUnavailable
	}
	t := cfg.Tuning
	prototype, err := media.LoadTextPack(path, media.TextLimits{MaxTextBytes: t.Contract.MaxTextBytes, MaxRecords: t.TextCatalog.MaxRecords, MaxFileBytes: t.TextCatalog.MaxFileBytes, MaxBundleBytes: t.TextCatalog.MaxBundleBytes})
	if err != nil {
		return nil, err
	}
	if !prototype.Manifest().Synthetic {
		return nil, lobby.ErrTextUnavailable
	}
	return prototype, nil
}
