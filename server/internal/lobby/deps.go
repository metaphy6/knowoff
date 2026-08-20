package lobby

import (
	"log/slog"

	"github.com/knowoff/knowoff/server/internal/audit"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// Deps bundles the external services a Manager needs.
type Deps struct {
	Config       *config.Config
	Logger       *slog.Logger
	Pack         *media.Pack
	Manager      *media.Manager
	Issuer       *media.SignedURLIssuer
	AssetBaseURL string
	Redis        *store.RedisClient
	NodeID       string
	Auth         *auth.Manager
	Profile      *profile.Manager
	Audit        *audit.Logger
	Leaderboard  *leaderboard.Manager
	Economy      *economy.Manager
}
