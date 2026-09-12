// Package config defines the single typed configuration struct for knowoffd.
// All configuration values are loaded from layered YAML files in configs/.
// Secrets are interpolated from environment variables at load time.
package config

import "time"

// Config is the single merged configuration struct. Every key from
// configs/base.yaml and gameplay/tuning.yaml is represented here.
type Config struct {
	Trust        TrustConfig        `yaml:"trust"`
	App          AppConfig          `yaml:"app"`
	Log          LogConfig          `yaml:"log"`
	Server       ServerConfig       `yaml:"server"`
	WebSocket    WebSocketConfig    `yaml:"websocket"`
	Protocol     ProtocolConfig     `yaml:"protocol"`
	Text         *TextConfig        `yaml:"text,omitempty"`
	Localization LocalizationConfig `yaml:"localization"`
	Moderation   ModerationConfig   `yaml:"moderation"`
	Database     DatabaseConfig     `yaml:"database"`
	Redis        RedisConfig        `yaml:"redis"`
	Storage      StorageConfig      `yaml:"storage"`
	Media        MediaConfig        `yaml:"media"`
	Security     SecurityConfig     `yaml:"security"`
	RateLimit    RateLimitConfig    `yaml:"rate_limit"`
	Tuning       TuningConfig       `yaml:"tuning"`
	Bots         *BotsConfig        `yaml:"bots,omitempty"`
}

// AppConfig identifies the running service.
type AppConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Env     string `yaml:"env"`
}

// LogConfig controls structured logging.
type LogConfig struct {
	Level     string `yaml:"level"`
	Format    string `yaml:"format"`
	AddCaller bool   `yaml:"add_caller"`
}

// ServerConfig is the public and admin HTTP server configuration.
type ServerConfig struct {
	BindAddr        string   `yaml:"bind_addr"`
	Port            int      `yaml:"port"`
	AdminAddr       string   `yaml:"admin_addr"`
	AdminBindAddr   string   `yaml:"admin_bind_addr"`
	AdminPort       int      `yaml:"admin_port"`
	MetricsBindAddr string   `yaml:"metrics_bind_addr"`
	MetricsPort     int      `yaml:"metrics_port"`
	ShutdownGraceS  int      `yaml:"shutdown_grace_s"`
	ReadTimeoutS    int      `yaml:"read_timeout_s"`
	WriteTimeoutS   int      `yaml:"write_timeout_s"`
	IdleTimeoutS    int      `yaml:"idle_timeout_s"`
	AllowedOrigins  []string `yaml:"allowed_origins"`
	// MaxConnections caps concurrent live WebSocket connections so a burst of
	// clients degrades gracefully (HTTP 503 at upgrade time) instead of
	// exhausting host CPU/memory. 0 means unlimited.
	MaxConnections int `yaml:"max_connections"`
}

// WebSocketConfig tunes the WebSocket codec and keepalive.
type WebSocketConfig struct {
	MaxMessageBytes int `yaml:"max_message_bytes"`
	PongWaitS       int `yaml:"pong_wait_s"`
	PingPeriodS     int `yaml:"ping_period_s"`
	WriteWaitS      int `yaml:"write_wait_s"`
}

// ProtocolConfig is the wire protocol version.
type ProtocolConfig struct {
	Version int `yaml:"version"`
}

// LocalizationConfig lists supported locales.
type LocalizationConfig struct {
	DefaultLocale       string   `yaml:"default_locale"`
	SupportedLocales    []string `yaml:"supported_locales"`
	PseudoLocaleEnabled bool     `yaml:"pseudo_locale_enabled"`
}

// ModerationConfig holds per-language free-chat word lists. English is always
// applied as the fallback list in addition to a player's selected language.
type ModerationConfig struct {
	DefaultLanguage  string                 `yaml:"default_language"`
	WordLists        map[string][]string    `yaml:"word_lists"`
	ContentScreening ContentScreeningConfig `yaml:"content_screening"`
}

// ContentScreeningConfig controls the optional server-only automated text reviewer.
// Enable it with an environment overlay; APIKey accepts the existing ${VAR}
// secret interpolation. Disabled or unavailable screening blocks approval.
type ContentScreeningConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key" json:"-"`
	TimeoutS int    `yaml:"timeout_s"`
}

// DatabaseConfig is the PostgreSQL connection pool.
type DatabaseConfig struct {
	Driver           string `yaml:"driver"`
	Host             string `yaml:"host"`
	Port             int    `yaml:"port"`
	Name             string `yaml:"name"`
	User             string `yaml:"user"`
	Password         string `yaml:"password"`
	SSLMode          string `yaml:"ssl_mode"`
	MaxOpenConns     int    `yaml:"max_open_conns"`
	MaxIdleConns     int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeS int    `yaml:"conn_max_lifetime_s"`
}

// RedisConfig is the Redis connection pool.
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// StorageConfig is the S3-compatible object storage client.
type StorageConfig struct {
	Driver          string `yaml:"driver"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	UsePathStyle    bool   `yaml:"use_path_style"`
	AssetsURL       string `yaml:"assets_url"`
}

// MediaConfig controls pack loading and signed URLs.
type MediaConfig struct {
	ActivePackTag       string `yaml:"active_pack_tag"`
	PackCheckIntervalS  int    `yaml:"pack_check_interval_s"`
	SignedURLTTLS       int    `yaml:"signed_url_ttl_s"`
	LocalBundlePath     string `yaml:"local_bundle_path"`
	URLSigningKey       string `yaml:"url_signing_key"`
	WorkbenchIngestPath string `yaml:"workbench_ingest_path"`
}

// SecurityConfig holds JWT and crypto settings.
type SecurityConfig struct {
	DevBotKey         string              `yaml:"dev_bot_key"`
	JWTSigningKey     string              `yaml:"jwt_signing_key"`
	JWTIssuer         string              `yaml:"jwt_issuer"`
	JWTAudience       string              `yaml:"jwt_audience"`
	AccessTokenTTLM   int                 `yaml:"access_token_ttl_m"`
	RefreshTokenTTLH  int                 `yaml:"refresh_token_ttl_h"`
	BcryptCost        int                 `yaml:"bcrypt_cost"`
	AdminTOTPIssuer   string              `yaml:"admin_totp_issuer"`
	AdminSessionTTLH  int                 `yaml:"admin_session_ttl_h"`
	SSVCallbackKey    string              `yaml:"ssv_callback_key"`
	SSVAllowedSenders string              `yaml:"ssv_allowed_senders"`
	OAuth             OAuthSecurityConfig `yaml:"oauth"`
}

// OAuthSecurityConfig holds OAuth client settings. Secrets are interpolated
// from environment variables.
type OAuthSecurityConfig struct {
	Google   OAuthProviderSecurityConfig `yaml:"google"`
	Facebook OAuthProviderSecurityConfig `yaml:"facebook"`
}

// OAuthProviderSecurityConfig is one OAuth provider's client credentials.
type OAuthProviderSecurityConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

// RateLimitConfig tunes the per-connection and per-account intent rate limits.
type RateLimitConfig struct {
	Enabled             bool `yaml:"enabled"`
	MaxIntentsPerSecond int  `yaml:"max_intents_per_second"`
	MaxIntentsBurst     int  `yaml:"max_intents_burst"`
	MaxBytesPerFrame    int  `yaml:"max_bytes_per_frame"`
}

// BotsConfig enables external dev/test bot connections. Intentionally absent from prod.
type BotsConfig struct {
	Enabled        bool   `yaml:"enabled"`
	DevBotEndpoint string `yaml:"dev_bot_endpoint,omitempty"`
}

// TuningConfig is the gameplay and economy tuning loaded from gameplay/tuning.yaml.
type TuningConfig struct {
	TextCatalog TextCatalogTuning `yaml:"text_catalog"`
	Contract    ContractTuning    `yaml:"contract"`
	Seed        int               `yaml:"seed"`
	Game        GameTuning        `yaml:"game"`
	Timers      TimersTuning      `yaml:"timers"`
	Hand        HandTuning        `yaml:"hand"`
	Dealing     DealingTuning     `yaml:"dealing"`
	Points      PointsTuning      `yaml:"points"`
	Noin        NoinTuning        `yaml:"noin"`
	Economy     EconomyTuning     `yaml:"economy"`
	Liquidity   LiquidityTuning   `yaml:"liquidity"`
	LiveOps     LiveOpsTuning     `yaml:"liveops"`
	Portal      PortalTuning      `yaml:"portal"`
	Progression ProgressionTuning `yaml:"progression"`
}

// GameTuning is room structure and victory rules.
type GameTuning struct {
	RoomSizes              []int       `yaml:"room_sizes"`
	DonowersBySize         map[int]int `yaml:"donowers_by_size"`
	VotesBySize            map[int]int `yaml:"votes_by_size"`
	MinConnected           int         `yaml:"min_connected"`
	ReconnectGraceS        int         `yaml:"reconnect_grace_s"`
	PokesPerTargetPerRound int         `yaml:"pokes_per_target_per_round"`
	AbandonCooldownsS      []int       `yaml:"abandon_cooldowns_s"`
}

// TimersTuning is phase timers in seconds.
type TimersTuning struct {
	TradeResponseS      int `yaml:"trade_response_s"`
	RoundStartCountdown int `yaml:"round_start_countdown"`
	PlayTurn            int `yaml:"play_turn"`
	DiscussionPerPlayer int `yaml:"discussion_per_player"`
	KnowoffBallot       int `yaml:"knowoff_ballot"`
	KnowoffRunoff       int `yaml:"knowoff_runoff"`
	VoteResultWindow    int `yaml:"vote_result_window"`
	VoteResultFalling   int `yaml:"vote_result_falling" json:"VoteResultFalling,omitempty"`
	RevealLockout       int `yaml:"reveal_lockout"`
	RevealView          int `yaml:"reveal_view"`
	ShuffleBonusSeconds int `yaml:"shuffle_bonus_seconds"`
	PrefetchCountdown   int `yaml:"prefetch_countdown"`
}

// HandTuning is hand size and specialty dealing weights.
type HandTuning struct {
	Size             int                `yaml:"size"`
	DrawPile         int                `yaml:"draw_pile"`
	SpecialtyWeights map[string]float64 `yaml:"specialty_weights"`
}

// DealingTuning is the relevance mesh thresholds.
type DealingTuning struct {
	BandHigh          float64 `yaml:"band_high"`
	BandLow           float64 `yaml:"band_low"`
	MinHighPerNown    int     `yaml:"min_high_per_nown"`
	MinDistantPerNown int     `yaml:"min_distant_per_nown"`
}

// PointsTuning is match point awards and penalties.
type PointsTuning struct {
	CorrectVote    int `yaml:"correct_vote"`
	NowerWinBonus  int `yaml:"nower_win_bonus"`
	DonowerTeamWin int `yaml:"donower_team_win"`
	DrawPenalty    int `yaml:"draw_penalty"`
}

// NoinTuning is currency earn rates.
type NoinTuning struct {
	MatchCompleted           int `yaml:"match_completed"`
	NowerWin                 int `yaml:"nower_win"`
	DonowerTeamWin           int `yaml:"donower_team_win"`
	CorrectVote              int `yaml:"correct_vote"`
	DonowerVoteSurvived      int `yaml:"donower_vote_survived"`
	DailyFirstWin            int `yaml:"daily_first_win"`
	DailyEarnCap             int `yaml:"daily_earn_cap"`
	ChallengeWinner          int `yaml:"challenge_winner"`
	ContributorAcceptedAsset int `yaml:"contributor_accepted_asset"`
}

// EconomyTuning is monetization prices and caps.
type EconomyTuning struct {
	FreeDailyQuickplayMatches int            `yaml:"free_daily_quickplay_matches"`
	PointsToNoin              int            `yaml:"points_to_noin"`
	PlayPassPrices            map[string]int `yaml:"play_pass_prices"`
	PremiumYearlyDiscountPct  int            `yaml:"premium_yearly_discount_pct"`
	UnlockPrices              map[string]int `yaml:"unlock_prices"`
	NoinBundles               []int          `yaml:"noin_bundles"`
}

// LiquidityTuning is Quick Play backfill bot settings.
type LiquidityTuning struct {
	BackfillEnabled      bool    `yaml:"backfill_enabled"`
	QueueTimeoutS        int     `yaml:"queue_timeout_s"`
	MinHumans            int     `yaml:"min_humans"`
	LeaderboardMinHumans int     `yaml:"leaderboard_min_humans"`
	NoinMinHumans        int     `yaml:"noin_min_humans"`
	BotThinkMinS         float64 `yaml:"bot_think_min_s"`
	BotThinkMaxS         float64 `yaml:"bot_think_max_s"`
}

// LiveOpsTuning is leaderboard and challenge constants.
type LiveOpsTuning struct {
	LeaderboardDailyCountedMatches int `yaml:"leaderboard_daily_counted_matches"`
	ChallengeMaxEntries            int `yaml:"challenge_max_entries"`
	ChallengeVotesPerPlayer        int `yaml:"challenge_votes_per_player"`
}

// PortalTuning is contributor portal limits.
type PortalTuning struct {
	MinAccountLevelToApply          int    `yaml:"min_account_level_to_apply"`
	SubmissionsPerContributorPerDay int    `yaml:"submissions_per_contributor_per_day"`
	GuardFreezeMaxH                 int    `yaml:"guard_freeze_max_h"`
	MaxTextSubmissionLength         int    `yaml:"max_text_submission_length"`
	TermsVersion                    string `yaml:"terms_version"`
}

// ProgressionTuning controls XP awards and level thresholds.
type ProgressionTuning struct {
	XPBase           int   `yaml:"xp_base"`
	XPPerCorrectVote int   `yaml:"xp_per_correct_vote"`
	XPWinBonus       int   `yaml:"xp_win_bonus"`
	LevelThresholds  []int `yaml:"level_thresholds"`
}

// ShutdownGrace returns the configured graceful shutdown window.
func (c *Config) ShutdownGrace() time.Duration {
	return time.Duration(c.Server.ShutdownGraceS) * time.Second
}

// RequiredSecrets lists environment variable names that must be present.
func (c *Config) RequiredSecrets() []string {
	return []string{
		"KNOWOFF_DB_PASSWORD",
		"KNOWOFF_REDIS_PASSWORD",
		"KNOWOFF_STORAGE_ACCESS_KEY",
		"KNOWOFF_STORAGE_SECRET_KEY",
		"KNOWOFF_JWT_KEY",
		"KNOWOFF_MEDIA_URL_KEY",
	}
}

// TrustConfig names the independently versioned user terms and owner-provided
// public support/privacy locations. Empty defaults keep dependent flows closed.
type TrustConfig struct {
	UserTermsVersion string `yaml:"user_terms_version"`
	SupportURL       string `yaml:"support_url"`
	PrivacyURL       string `yaml:"privacy_url"`
}
