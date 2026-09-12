// Package avatar normalizes and screens paid custom avatars before atomic activation.
package avatar

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxAvatarBytes     = 2 * 1024 * 1024
	maxAvatarDimension = 2048
	avatarSize         = 256
)

var (
	ErrBusy        = errors.New("avatar.busy")
	ErrInvalid     = errors.New("avatar.invalid")
	ErrUnavailable = errors.New("avatar.unavailable")
	ErrNotEntitled = errors.New("avatar.not_entitled")
	ErrStale       = errors.New("avatar.stale")
	ErrFlagged     = errors.New("avatar.flagged")
	ErrAccount     = errors.New("avatar.account_unavailable")
)

type Manager struct {
	db         *sql.DB
	slots      chan struct{}
	sqlTimeout time.Duration
	screen     func(context.Context, []byte) error
}

// Purchases use the economy route. Uploads never debit or grant value.
func NewManager(db *sql.DB, cfg *config.Config, _ *economy.Manager) *Manager {
	slots := 2
	if cfg != nil && cfg.Moderation.AvatarUploadSlots > 0 && cfg.Moderation.AvatarUploadSlots <= 16 {
		slots = cfg.Moderation.AvatarUploadSlots
	}
	m := &Manager{db: db, slots: make(chan struct{}, slots), sqlTimeout: 3 * time.Second}
	if cfg != nil {
		m.screen = newImageScreen(cfg.Moderation.AvatarScreening)
	}
	return m
}
func avatarError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "avatar.unavailable"
	switch {
	case errors.Is(err, ErrBusy):
		status = http.StatusTooManyRequests
		code = err.Error()
		w.Header().Set("Retry-After", "1")
	case errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
		code = err.Error()
	case errors.Is(err, ErrUnavailable):
		status = http.StatusServiceUnavailable
		code = err.Error()
	case errors.Is(err, ErrNotEntitled):
		status = http.StatusForbidden
		code = err.Error()
	case errors.Is(err, ErrStale):
		status = http.StatusConflict
		code = err.Error()
	case errors.Is(err, ErrFlagged):
		status = http.StatusUnprocessableEntity
		code = err.Error()
	case errors.Is(err, ErrAccount):
		status = http.StatusUnauthorized
		code = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
func (m *Manager) Handler(authMgr *auth.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := m.acquireUpload(r.Context()); err != nil {
			avatarError(w, err)
			return
		}
		defer m.releaseUpload()
		uploadCtx, uploadCancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer uploadCancel()
		r = r.WithContext(uploadCtx)
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") || authMgr == nil {
			avatarError(w, ErrAccount)
			return
		}
		authCtx, authCancel := context.WithTimeout(r.Context(), m.sqlTimeout)
		account, err := authMgr.ValidateAccessToken(authCtx, token)
		authCancel()
		if err != nil {
			avatarError(w, ErrAccount)
			return
		}
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
		defer controller.SetReadDeadline(time.Time{})
		r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+65536)
		reader, err := r.MultipartReader()
		if err != nil {
			avatarError(w, ErrInvalid)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			avatarError(w, ErrInvalid)
			return
		}
		if part.FormName() != "avatar" || part.FileName() == "" {
			part.Close()
			avatarError(w, ErrInvalid)
			return
		}
		data, err := io.ReadAll(io.LimitReader(part, maxAvatarBytes+1))
		part.Close()
		if err != nil || len(data) > maxAvatarBytes {
			avatarError(w, ErrInvalid)
			return
		}
		// Reject extra fields/files and malformed or oversized trailing framing.
		if extra, err := reader.NextPart(); err != io.EOF {
			if extra != nil {
				extra.Close()
			}
			avatarError(w, ErrInvalid)
			return
		}
		authorize := func(ctx context.Context, tx *sql.Tx) error {
			got, err := authMgr.ValidateAccessTokenTx(ctx, tx, token)
			if err != nil || got != account {
				return ErrAccount
			}
			return nil
		}
		if err := m.processUpload(r.Context(), account, bytes.NewReader(data), authorize); err != nil {
			avatarError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
func (m *Manager) acquireUpload(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrUnavailable
	}
	select {
	case m.slots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}
func (m *Manager) releaseUpload() { <-m.slots }
func (m *Manager) ProcessUpload(ctx context.Context, account string, r io.Reader, _ string) error {
	if err := m.acquireUpload(ctx); err != nil {
		return err
	}
	defer m.releaseUpload()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return m.processUpload(ctx, account, r, nil)
}

// accountState holds the account lock before dependent image/entitlement rows.
func avatarAccountState(ctx context.Context, tx *sql.Tx, account string) (revision, epoch int64, err error) {
	if err = tx.QueryRowContext(ctx, `SELECT avatar_revision,session_epoch FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&revision, &epoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, ErrAccount
		}
		return 0, 0, ErrUnavailable
	}
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp()) FROM accounts WHERE id=$1`, account).Scan(&allowed); err != nil {
		return
	}
	if !allowed {
		return 0, 0, ErrAccount
	}
	var owned bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar' AND active_until IS NULL)`, account).Scan(&owned)
	if err == nil && !owned {
		err = ErrNotEntitled
	}
	return
}
func (m *Manager) processUpload(ctx context.Context, account string, r io.Reader, authorize func(context.Context, *sql.Tx) error) error {
	if parsed, err := uuid.Parse(account); err != nil || parsed == uuid.Nil || parsed.String() != account {
		return ErrInvalid
	}
	if m.screen == nil {
		return ErrUnavailable
	}
	revision, epoch, err := func() (int64, int64, error) {
		sqlCtx, cancel := context.WithTimeout(ctx, m.sqlTimeout)
		defer cancel()
		capture, err := m.db.BeginTx(sqlCtx, nil)
		if err != nil {
			return 0, 0, err
		}
		defer capture.Rollback()
		revision, epoch, err := avatarAccountState(sqlCtx, capture, account)
		if err != nil {
			return 0, 0, err
		}
		if authorize != nil {
			if err = authorize(sqlCtx, capture); err != nil {
				return 0, 0, err
			}
		}
		if err = capture.Commit(); err != nil {
			return 0, 0, err
		}
		return revision, epoch, nil
	}()
	if err != nil {
		return err
	}
	encoded, err := normalizeAvatar(r)
	if err != nil {
		return err
	}
	if err = m.screen(ctx, encoded); err != nil {
		return err
	}
	sqlCtx, sqlCancel := context.WithTimeout(ctx, m.sqlTimeout)
	defer sqlCancel()
	ctx = sqlCtx
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	currentRevision, currentEpoch, err := avatarAccountState(ctx, tx, account)
	if err != nil {
		return err
	}
	if currentRevision != revision || currentEpoch != epoch {
		return ErrStale
	}
	if authorize != nil {
		if err = authorize(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET avatar='custom',avatar_revision=avatar_revision+1,updated_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO custom_avatars(account_id,blob,content_type,moderated,revision) VALUES($1,$2,'image/webp',true,$3) ON CONFLICT(account_id) DO UPDATE SET blob=EXCLUDED.blob,content_type=EXCLUDED.content_type,moderated=true,revision=EXCLUDED.revision,updated_at=clock_timestamp()`, account, encoded, revision+1); err != nil {
		return err
	}
	if authorize != nil {
		if err = authorize(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func normalizeAvatar(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxAvatarBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxAvatarBytes {
		return nil, ErrInvalid
	}
	dims, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || dims.Width < 1 || dims.Height < 1 || dims.Width > maxAvatarDimension || dims.Height > maxAvatarDimension {
		return nil, ErrInvalid
	}
	// DecodeConfig precedes full raster allocation, including malicious dimensions.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	encoded, err := webp.EncodeRGBA(resize(cropToSquare(img), avatarSize), 80)
	if err != nil || len(encoded) == 0 || len(encoded) > maxAvatarBytes {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func cropToSquare(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return src
	}
	size := w
	x0, y0 := b.Min.X, b.Min.Y
	if h < w {
		size = h
		x0 += (w - h) / 2
	} else {
		y0 += (h - w) / 2
	}
	r := image.Rect(x0, y0, x0+size, y0+size)
	dst := image.NewRGBA(r)
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}

func resize(src image.Image, size int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}
