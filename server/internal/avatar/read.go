package avatar

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
)

// ImageHandler serves only the current approved revision. Optional authored
// imagery uses symmetric blocking; missing, removed and private images look alike.
func (m *Manager) ImageHandler(authMgr *auth.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if authMgr == nil || token == r.Header.Get("Authorization") {
			avatarError(w, ErrAccount)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		actor, err := authMgr.ValidateAccessToken(ctx, token)
		if err != nil {
			avatarError(w, ErrAccount)
			return
		}
		target := r.PathValue("account_id")
		parsed, err := uuid.Parse(target)
		if err != nil || parsed == uuid.Nil || parsed.String() != target {
			http.NotFound(w, r)
			return
		}
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			avatarError(w, ErrUnavailable)
			return
		}
		defer tx.Rollback()
		ids := []string{actor}
		if actor != target {
			ids = append(ids, target)
		}
		sort.Strings(ids)
		for _, id := range ids {
			var locked string
			if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
				http.NotFound(w, r)
				return
			}
		}
		if got, err := authMgr.ValidateAccessTokenTx(ctx, tx, token); err != nil || got != actor {
			avatarError(w, ErrAccount)
			return
		}
		var image []byte
		err = tx.QueryRowContext(ctx, `SELECT c.blob FROM custom_avatars c JOIN accounts a ON a.id=c.account_id
   WHERE a.id=$1 AND a.deleted_at IS NULL AND a.banned_at IS NULL AND (a.suspended_until IS NULL OR a.suspended_until<=clock_timestamp())
   AND a.avatar='custom' AND c.moderated AND c.content_type='image/webp' AND c.revision>0 AND c.revision=a.avatar_revision AND octet_length(c.blob) BETWEEN 1 AND $3
   AND EXISTS(SELECT 1 FROM entitlements e WHERE e.account_id=a.id AND e.entitlement_type='custom_avatar' AND e.active_until IS NULL)
   AND NOT EXISTS(SELECT 1 FROM player_blocks b WHERE (b.actor_id=$1 AND b.target_id=$2) OR (b.actor_id=$2 AND b.target_id=$1))`, target, actor, maxAvatarBytes).Scan(&image)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if got, err := authMgr.ValidateAccessTokenTx(ctx, tx, token); err != nil || got != actor {
			avatarError(w, ErrAccount)
			return
		}
		if err = tx.Commit(); err != nil {
			avatarError(w, ErrUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(image)
	})
}
