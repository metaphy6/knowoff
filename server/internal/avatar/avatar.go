// Package avatar handles custom avatar uploads: decode, crop/resize to
// 256x256, WebP re-encode, blob storage, and entitlement grant.
package avatar

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"

	"github.com/chai2010/webp"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	xdraw "golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"
)

const (
	maxAvatarBytes     = 2 * 1024 * 1024
	maxAvatarDimension = 2048
	avatarSize         = 256
)

// Manager owns custom avatar uploads.
type Manager struct {
	db      *sql.DB
	cfg     *config.Config
	economy *economy.Manager
}

// NewManager returns an avatar manager.
func NewManager(db *sql.DB, cfg *config.Config, economy *economy.Manager) *Manager {
	return &Manager{db: db, cfg: cfg, economy: economy}
}

// Handler returns the HTTP handler for POST /api/avatar.
func (m *Manager) Handler(authMgr *auth.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, authMgr)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("avatar")
		if err != nil {
			http.Error(w, "missing avatar field", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Size > maxAvatarBytes {
			http.Error(w, "avatar too large", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		if err := m.ProcessUpload(ctx, accountID, file, header.Header.Get("Content-Type")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// ProcessUpload decodes, crops/resizes, encodes to WebP, stores the blob, and
// grants the custom_avatar entitlement.
func (m *Manager) ProcessUpload(ctx context.Context, accountID string, r io.Reader, contentType string) error {
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}

	data, err := io.ReadAll(io.LimitReader(r, maxAvatarBytes+1))
	if err != nil {
		return fmt.Errorf("read avatar: %w", err)
	}
	if len(data) > maxAvatarBytes {
		return fmt.Errorf("avatar too large")
	}

	img, format, err := decodeImage(bytes.NewReader(data), contentType)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	if img == nil {
		return fmt.Errorf("unsupported image format")
	}
	_ = format

	bounds := img.Bounds()
	if bounds.Dx() > maxAvatarDimension || bounds.Dy() > maxAvatarDimension {
		return fmt.Errorf("image dimensions too large")
	}

	square := cropToSquare(img)
	sized := resize(square, avatarSize)

	encoded, err := webp.EncodeRGBA(sized, 80)
	if err != nil {
		return fmt.Errorf("encode webp: %w", err)
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin avatar tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO custom_avatars (account_id, blob, content_type, moderated, created_at, updated_at)
		 VALUES ($1, $2, 'image/webp', false, now(), now())
		 ON CONFLICT (account_id) DO UPDATE SET
		   blob = EXCLUDED.blob,
		   content_type = EXCLUDED.content_type,
		   moderated = false,
		   updated_at = now()`,
		accountID, encoded); err != nil {
		return fmt.Errorf("store avatar: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit avatar tx: %w", err)
	}

	if m.economy != nil {
		price := m.cfg.Tuning.Economy.UnlockPrices["custom_avatar"]
		if price <= 0 {
			price = 1000
		}
		// GrantUnlock debits Noin atomically. Failures here do not rollback the
		// stored blob; the client can retry the unlock separately.
		_ = m.economy.Entitlements.GrantUnlock(ctx, accountID, economy.EntitlementCustomAvatar, "", price)
	}
	return nil
}

func decodeImage(r io.Reader, contentType string) (image.Image, string, error) {
	switch {
	case strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg"):
		img, err := jpeg.Decode(r)
		return img, "jpeg", err
	case strings.Contains(contentType, "png"):
		img, err := png.Decode(r)
		return img, "png", err
	case strings.Contains(contentType, "webp"):
		img, err := xwebp.Decode(r)
		return img, "webp", err
	default:
		// Try standard decode.
		img, format, err := image.Decode(r)
		return img, format, err
	}
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

func bearerAccount(r *http.Request, authMgr *auth.Manager) (string, bool) {
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		accountID, err := authMgr.ValidateAccessToken(r.Context(), token[7:])
		if err == nil {
			return accountID, true
		}
	}
	return "", false
}
