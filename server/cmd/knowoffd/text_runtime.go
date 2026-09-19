package main

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/internal/admin"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
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
	bonus     textBonusWorker
}

// No listener or worker is opened until the confirmed old owner work is closed.
// The optional prototype path is a server environment choice, never client input.
func newTextRuntime(ctx context.Context, db *sql.DB, cfg *config.Config, prototypePath string) (out *textRuntime, err error) {
	if cfg == nil || cfg.Text == nil || db == nil {
		return nil, lobby.ErrTextUnavailable
	}
	if err := store.CheckRuntimeSchema(ctx, db); err != nil {
		return nil, err
	}
	if err := store.CheckRuntimeCutover(ctx, db); err != nil {
		return nil, err
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
	values, err := store.NewTextValueStore(db, cfg.Tuning).RequireAdmissionBindings().WithStartGuard(releases.ValidateStart).WithOwner(owner)
	if err != nil {
		return nil, err
	}
	if err = recoverTextRuntime(ctx, func(ctx context.Context, limit int) (store.TextOwnerRecovery, error) {
		return owner.RecoverLostOwners(ctx, values, limit)
	}); err != nil {
		return nil, err
	}
	if err = recoverRoomRuntime(ctx, values.RecoverRoomOperations); err != nil {
		return nil, err
	}
	trust := store.NewTextTrustStore(db)
	manager, err := lobby.NewTextManager(lobby.TextDeps{
		Owner: owner.Token().IncarnationID, Authority: owner, Config: cfg, Values: values, Prototype: prototype,
		Operations: store.NewAdminOperationStore(db),
		Resolve:    releases.Resolve, ResolveRelease: releases.ResolveRelease,
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

func recoverRoomRuntime(ctx context.Context, batch func(context.Context, int) (int, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := batch(ctx, 100)
		if err != nil {
			return err
		}
		if n < 100 {
			return nil
		}
	}
}

type textDrainer interface {
	DrainContext(context.Context) error
	ActiveMatchesContext(context.Context) (int, error)
	Close(context.Context) error
}

func drainTextRuntime(ctx context.Context, m textDrainer) error {
	drainErr := m.DrainContext(ctx)
	entered := drainErr == nil
	if ctx.Err() != nil && errors.Is(drainErr, ctx.Err()) {
		// Grace expiration still permits the existing independently bounded
		// compensation/closure path. Its successful completion is authoritative.
		drainErr = nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for entered && drainErr == nil {
		active, err := m.ActiveMatchesContext(ctx)
		if err != nil {
			if ctx.Err() == nil {
				drainErr = err
			}
			break
		}
		if active == 0 {
			break
		}
		select {
		case <-ctx.Done():
			goto close
		case <-ticker.C:
		}
	}
close:
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return errors.Join(drainErr, m.Close(cleanup))
}
func (r *textRuntime) Close(ctx context.Context) error {
	r.closeOnce.Do(func() {
		r.closeErr = drainTextRuntime(ctx, r.Lobby)
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Gameplay keeps its clock through drain. This separate value worker
		// must finish before releasing the process authority it observed.
		if err := r.bonus.stop(cleanup); err != nil {
			r.closeErr = errors.Join(r.closeErr, err)
			return
		}
		r.closeErr = errors.Join(r.closeErr, r.Owner.Release(cleanup))
		r.closeErr = errors.Join(r.closeErr, r.Owner.Wait(cleanup))
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

// Install before listeners open. The lobby serializes peer admission/closure
// around the durable sanction check; the callback commits before socket cleanup.
func (r *textRuntime) bindModeration(pm *portal.Manager, am *auth.Manager) {
	pm.SetAccountDisconnect(func(ctx context.Context, account string) error {
		return r.Lobby.EnforceAccount(ctx, account, am.RevokeEnforcedSessions)
	})
}

func (r *textRuntime) sanctionDelivery(s *store.AccountSanctionStore) func(context.Context, string) error {
	return func(ctx context.Context, id string) error {
		return r.Lobby.EnforceSanction(ctx, id, func(ctx context.Context, id string, b lobby.TextPeerBinding) (bool, error) {
			return s.ActiveForBinding(ctx, id, b.AccountID, b.DeviceHash)
		})
	}
}

func (r *textRuntime) operatorHooks(db *sql.DB, am *auth.Manager, em *economy.Manager) admin.OperatorHooks {
	sanctions := store.NewAccountSanctionStore(db)
	return admin.OperatorHooks{Decide: func(ctx context.Context, actor string, c store.AdminOperationCommand) (store.AdminOperationReceipt, error) {
		switch c.Kind {
		case "room_kick", "room_close":
			return r.Lobby.DecideRoomOperation(ctx, actor, c)
		case "noin_grant", "noin_refund":
			return em.CorrectNoin(ctx, actor, c)
		case "account_sanction", "sanction_lift":
			receipt, err := sanctions.Apply(ctx, actor, c, am.RevokeSessionsTx)
			if err != nil {
				return store.AdminOperationReceipt{}, err
			}
			if c.Kind == "account_sanction" && receipt.Status == "pending" {
				if err = sanctions.DeliverPending(ctx, c.ID, r.sanctionDelivery(sanctions)); err == nil {
					if completed, e := store.NewAdminOperationStore(db).Get(ctx, c.ID); e == nil {
						return completed, nil
					}
				}
			}
			return receipt, nil
		default:
			return store.AdminOperationReceipt{}, store.ErrAdminOperation
		}
	}}
}

func (r *textRuntime) runOperatorMaintenance(ctx context.Context, db *sql.DB, onError func(error)) {
	sanctions := store.NewAccountSanctionStore(db)
	weekly := store.NewLeaderboardAdminStore(db)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		for _, err := range []error{sanctions.ResumePending(ctx, 20, r.sanctionDelivery(sanctions)), weekly.ResumePending(ctx, 20)} {
			if err != nil && ctx.Err() == nil && onError != nil {
				onError(err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
