package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/knowoff/knowoff/server/internal/admin"
	"github.com/knowoff/knowoff/server/internal/audit"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/avatar"
	"github.com/knowoff/knowoff/server/internal/bots"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/handler"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/logging"
	"github.com/knowoff/knowoff/server/internal/notices"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
	"github.com/knowoff/knowoff/server/internal/reports"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/internal/workbench"
	"github.com/knowoff/knowoff/server/pkg/media"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := os.Getenv("KNOWOFF_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/local.yaml"
	}

	cfg, err := config.Load("configs/base.yaml", cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return runMigrations(cfg, logger)
	}
	if len(os.Args) > 1 && (os.Args[1] == "close-week" || os.Args[1] == "nightly") {
		return closeWeek(cfg, logger)
	}
	if len(os.Args) > 1 && os.Args[1] == "seed-admin" {
		return seedAdmin(cfg, logger)
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewBuildInfoCollector())
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Connection gauge placeholder; incremented/decremented by transport layer.
	connections := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "knowoff_websocket_connections",
		Help: "Current number of open WebSocket connections.",
	})
	registry.MustRegister(connections)

	// Caps concurrent live WebSocket connections; sized in configs/base.yaml
	// for this deployment's hardware. 0 (unset) means unlimited.
	connLimiter := ratelimit.NewConnLimiter(cfg.Server.MaxConnections)
	connLimiterRejections := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "knowoff_websocket_connections_rejected_total",
		Help: "Total WebSocket upgrade attempts rejected because the server was at its connection cap.",
	})
	registry.MustRegister(connLimiterRejections)

	db, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	authManager := auth.NewManager(db, []byte(cfg.Security.JWTSigningKey), cfg.Security.JWTIssuer, cfg.Security.JWTAudience,
		time.Duration(cfg.Security.AccessTokenTTLM)*time.Minute,
		time.Duration(cfg.Security.RefreshTokenTTLH)*time.Hour,
		auth.OAuthProviders{
			Google: auth.OAuthProviderConfig{
				ClientID:     cfg.Security.OAuth.Google.ClientID,
				ClientSecret: cfg.Security.OAuth.Google.ClientSecret,
				RedirectURL:  cfg.Security.OAuth.Google.RedirectURL,
			},
			Facebook: auth.OAuthProviderConfig{
				ClientID:     cfg.Security.OAuth.Facebook.ClientID,
				ClientSecret: cfg.Security.OAuth.Facebook.ClientSecret,
				RedirectURL:  cfg.Security.OAuth.Facebook.RedirectURL,
			},
		})
	profileManager := profile.NewManager(db, cfg.Tuning.Progression)
	auditLogger := audit.NewLogger(db)
	leaderboardManager := leaderboard.NewManager(db)
	economyManager := economy.NewManager(db, cfg)

	mediaManager := media.NewManager(nil)

	issuer := media.NewSignedURLIssuer([]byte(cfg.Media.URLSigningKey), time.Duration(cfg.Media.SignedURLTTLS)*time.Second)
	redisClient := store.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	lobbyManager := lobby.NewManager(lobby.Deps{
		Config:       cfg,
		Logger:       logger,
		Pack:         mediaManager.Active(),
		Manager:      mediaManager,
		Issuer:       issuer,
		AssetBaseURL: cfg.Storage.AssetsURL,
		Redis:        redisClient,
		NodeID:       cfg.App.Name + "-" + cfg.App.Version + "-" + fmt.Sprintf("%d", time.Now().Unix()),
		Auth:         authManager,
		Profile:      profileManager,
		Audit:        auditLogger,
		Leaderboard:  leaderboardManager,
		Economy:      economyManager,
	})

	deps := transport.Deps{
		Config:      cfg,
		Logger:      logger,
		DB:          db,
		RedisPing:   redisPingFunc(cfg),
		StoragePing: storagePingFunc(cfg),
		Connections: connections,
		Media:       mediaManager,
	}

	handlerDeps := handler.HandlerDeps{
		Config:                cfg,
		Logger:                logger,
		Lobby:                 lobbyManager,
		Connections:           connections,
		Auth:                  authManager,
		Profile:               profileManager,
		Audit:                 auditLogger,
		Leaderboard:           leaderboardManager,
		Economy:               economyManager,
		Redis:                 redisClient,
		ConnLimiter:           connLimiter,
		ConnLimiterRejections: connLimiterRejections,
	}

	backfillManager := bots.NewBackfillManager(bots.Deps{
		Config: cfg,
		Logger: logger,
		Lobby:  lobbyManager,
	})

	adminManager := admin.NewManager(db, cfg, redisClient)
	noticesManager := notices.NewManager(db, cfg, lobbyManager)
	reportsManager := reports.NewManager(db)
	avatarManager := avatar.NewManager(db, cfg, economyManager)
	portalManager := portal.NewManager(portal.Deps{
		DB:      db,
		Config:  cfg,
		Auth:    authManager,
		Profile: profileManager,
		Economy: economyManager,
		Admin:   adminManager,
		Media:   mediaManager,
	})
	if err := portalManager.EnsureActiveTermsVersion(context.Background()); err != nil {
		logger.Error("failed to ensure active portal terms", "error", err)
		return err
	}
	_ = portalManager.ExpireFreezes(context.Background())

	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/healthz", transport.HealthzHandler(deps))
	publicMux.HandleFunc("/readyz", transport.ReadyzHandler(deps))
	publicMux.HandleFunc("/ws", handler.RealtimeHandler(handlerDeps))
	publicBaseURL := fmt.Sprintf("http://%s:%d", cfg.Server.BindAddr, cfg.Server.Port)
	publicMux.HandleFunc("/join/", handler.RoomJoinHandler(lobbyManager, publicBaseURL))
	publicMux.HandleFunc("/rooms/create", handler.RoomCreateHandler(lobbyManager))
	handler.RegisterAuthRoutes(publicMux, handler.AuthDeps{Auth: authManager})
	handler.RegisterProfileRoutes(publicMux, handler.ProfileDeps{Profile: profileManager, Leaderboard: leaderboardManager}, authManager)
	handler.RegisterPublicRoutes(publicMux, handler.PublicRouteDeps{
		Auth:    authManager,
		Notices: noticesManager,
		Reports: reportsManager,
		Avatar:  avatarManager,
	})
	handler.RegisterEconomyRoutes(publicMux, handler.EconomyDeps{
		Config:  cfg,
		Auth:    authManager,
		Economy: economyManager,
	})
	publicMux.Handle("/portal/", portalManager.Handler())
	handler.RegisterChallengeRoutes(publicMux, handler.ChallengeDeps{
		Auth:   authManager,
		Portal: portalManager,
	})

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/healthz", transport.HealthzHandler(deps))
	adminMux.HandleFunc("/readyz", transport.ReadyzHandler(deps))
	adminMux.Handle("/admin/portal/", adminManager.PortalHandler(portalManager))
	adminMux.Handle("/admin/", adminManager.Handler(noticesManager))
	if cfg.App.Env != "prod" {
		ingestPath := cfg.Media.WorkbenchIngestPath
		if ingestPath == "" {
			ingestPath = "content/ingest"
		}
		wb := workbench.New(deps.Media, ingestPath)
		defer wb.Close()
		wb.Register(adminMux)
		logger.Info("media workbench mounted", "env", cfg.App.Env, "ingest", ingestPath)
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	publicServer := &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.BindAddr, cfg.Server.Port),
		Handler: transport.CORS(
			transport.RecoverPanic(publicMux, logger),
			cfg.Server.AllowedOrigins,
		),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutS) * time.Second,
	}

	adminAddr := cfg.Server.AdminAddr
	if adminAddr == "" {
		adminAddr = fmt.Sprintf("%s:%d", cfg.Server.AdminBindAddr, cfg.Server.AdminPort)
	}
	adminServer := &http.Server{
		Addr:    adminAddr,
		Handler: transport.RecoverPanic(adminMux, logger),
	}

	metricsServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.MetricsBindAddr, cfg.Server.MetricsPort),
		Handler: metricsMux,
	}

	errCh := make(chan error, 3)
	go func() { errCh <- publicServer.ListenAndServe() }()
	go func() { errCh <- adminServer.ListenAndServe() }()
	go func() { errCh <- metricsServer.ListenAndServe() }()

	// A ready server must be able to create a room immediately. Loading this
	// synchronously keeps /readyz false until gameplay media is available.
	if cfg.Media.LocalBundlePath != "" {
		pack, err := media.LoadPack(cfg.Media.LocalBundlePath, dealingTuningFromConfig(cfg))
		if err != nil {
			return fmt.Errorf("load media pack %q: %w", cfg.Media.LocalBundlePath, err)
		}
		mediaManager.Load(pack)
		logger.Info("media pack loaded", "tag", pack.Manifest.PackTag)
	}

	if mediaManager.Active() == nil {
		return fmt.Errorf("no media pack configured")
	}
	transport.SetReady(true)
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go runHealthWatcher(runCtx, deps, lobbyManager, logger)
	backfillManager.Start(runCtx)
	defer backfillManager.Stop()

	logger.Info("server started",
		"public", publicServer.Addr,
		"admin", adminServer.Addr,
		"metrics", metricsServer.Addr,
		"env", cfg.App.Env,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	// Flip readiness off before draining so load balancers stop sending traffic.
	transport.SetReady(false)
	lobbyManager.SetReady(false)
	runCancel()
	logger.Info("readiness disabled, draining connections")
	// Brief pause so a probe can observe the 503 before listeners close.
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace())
	defer cancel()

	if err := publicServer.Shutdown(ctx); err != nil {
		logger.Error("public server shutdown error", "error", err)
	}
	if err := adminServer.Shutdown(ctx); err != nil {
		logger.Error("admin server shutdown error", "error", err)
	}
	if err := metricsServer.Shutdown(ctx); err != nil {
		logger.Error("metrics server shutdown error", "error", err)
	}

	logger.Info("server stopped")
	return nil
}

func closeWeek(cfg *config.Config, logger *slog.Logger) error {
	logger.Info("closing weekly leaderboard")
	db, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	m := leaderboard.NewManager(db)
	weekID := leaderboard.WeekID(time.Now().UTC().Add(-7 * 24 * time.Hour))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := m.CloseWeek(ctx, weekID); err != nil {
		return fmt.Errorf("close week %s: %w", weekID, err)
	}
	logger.Info("weekly leaderboard closed", "week_id", weekID)
	return nil
}

// seedAdmin creates (or reports) a dev-only admin_accounts row so a fresh
// local stack has a working Admin Console login without hand-writing SQL.
// Refuses to run against a prod config; email/password are overridable via
// env vars so CI or a second developer can seed a non-default login.
func seedAdmin(cfg *config.Config, logger *slog.Logger) error {
	if cfg.App.Env == "prod" {
		return fmt.Errorf("seed-admin refuses to run with app.env=prod")
	}

	email := os.Getenv("KNOWOFF_SEED_ADMIN_EMAIL")
	if email == "" {
		email = "admin@knowoff.local"
	}
	password := os.Getenv("KNOWOFF_SEED_ADMIN_PASSWORD")
	if password == "" {
		password = "knowoff-dev-admin"
	}

	db, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminManager := admin.NewManager(db, cfg, nil)

	if secret, otpauthURL, err := adminManager.TOTPSecretForEmail(ctx, email); err == nil {
		logger.Info("admin account already seeded", "email", email)
		printSeededAdmin(email, "(unchanged — see previous seed output for the password)", secret, otpauthURL)
		return nil
	}

	authManager := auth.NewManager(db, []byte(cfg.Security.JWTSigningKey), cfg.Security.JWTIssuer, cfg.Security.JWTAudience,
		time.Duration(cfg.Security.AccessTokenTTLM)*time.Minute,
		time.Duration(cfg.Security.RefreshTokenTTLH)*time.Hour,
		auth.OAuthProviders{})
	tokens, err := authManager.CreateAnonymousAccount(ctx, "dev-admin-seed:"+email)
	if err != nil {
		return fmt.Errorf("create backing account: %w", err)
	}

	if err := adminManager.CreateAdmin(ctx, tokens.AccountID, email, password, "admin"); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	secret, otpauthURL, err := adminManager.TOTPSecretForEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("read totp secret: %w", err)
	}

	logger.Info("seeded dev admin account", "email", email)
	printSeededAdmin(email, password, secret, otpauthURL)
	return nil
}

func printSeededAdmin(email, password, totpSecret, otpauthURL string) {
	fmt.Println("--- knowoff admin console dev login ---")
	fmt.Printf("email:       %s\n", email)
	fmt.Printf("password:    %s\n", password)
	fmt.Printf("totp secret: %s\n", totpSecret)
	fmt.Printf("otpauth url: %s\n", otpauthURL)
	fmt.Println("scan the otpauth url with an authenticator app, or compute a code with: oathtool --totp -b \"" + totpSecret + "\"")
	fmt.Println("----------------------------------------")
}

func runMigrations(cfg *config.Config, logger *slog.Logger) error {
	logger.Info("running database migrations")
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.Name, cfg.Database.SSLMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer db.Close()

	migrationsPath := "/migrations"
	if _, err := os.Stat("migrations"); err == nil {
		migrationsPath = "migrations"
	}
	if err := store.MigrateUp(db, migrationsPath); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	logger.Info("migrations complete")
	return nil
}

func openDB(cfg *config.Config) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User,
		cfg.Database.Password, cfg.Database.Name, cfg.Database.SSLMode)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeS) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func dealingTuningFromConfig(cfg *config.Config) media.DealingTuning {
	return media.DealingTuning{
		BandHigh:          cfg.Tuning.Dealing.BandHigh,
		BandLow:           cfg.Tuning.Dealing.BandLow,
		MinHighPerNown:    cfg.Tuning.Dealing.MinHighPerNown,
		MinDistantPerNown: cfg.Tuning.Dealing.MinDistantPerNown,
	}
}

func redisPingFunc(cfg *config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		d := net.Dialer{}
		conn, err := d.DialContext(ctx, "tcp", cfg.Redis.Addr)
		if err != nil {
			return err
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		if cfg.Redis.Password != "" {
			if _, err := fmt.Fprintf(conn, "AUTH %s\r\n", cfg.Redis.Password); err != nil {
				return err
			}
			line, err := br.ReadString('\n')
			if err != nil {
				return err
			}
			if !strings.HasPrefix(line, "+OK") {
				return fmt.Errorf("redis auth failed: %s", strings.TrimSpace(line))
			}
		}
		if _, err := fmt.Fprint(conn, "PING\r\n"); err != nil {
			return err
		}
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if !strings.HasPrefix(line, "+PONG") {
			return fmt.Errorf("redis ping failed: %s", strings.TrimSpace(line))
		}
		return nil
	}
}

func storagePingFunc(cfg *config.Config) func(context.Context) error {
	return func(ctx context.Context) error {
		url := cfg.Storage.Endpoint
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		url += "minio/health/live"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("storage unhealthy: %d", resp.StatusCode)
		}
		return nil
	}
}

func runHealthWatcher(ctx context.Context, deps transport.Deps, lobby *lobby.Manager, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ok := true
		if deps.DB != nil {
			if err := deps.DB.PingContext(checkCtx); err != nil {
				logger.Warn("dependency degraded", "dependency", "postgres", "error", err)
				ok = false
			}
		}
		if ok && deps.RedisPing != nil {
			if err := deps.RedisPing(checkCtx); err != nil {
				logger.Warn("dependency degraded", "dependency", "redis", "error", err)
				ok = false
			}
		}
		if ok && deps.StoragePing != nil {
			if err := deps.StoragePing(checkCtx); err != nil {
				logger.Warn("dependency degraded", "dependency", "storage", "error", err)
				ok = false
			}
		}
		cancel()
		transport.SetReady(ok)
		lobby.SetReady(ok)
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level, AddSource: cfg.Log.AddCaller}
	var handler slog.Handler
	if cfg.Log.Format == "text" {
		handler = logging.NewConsoleHandlerWithOptions(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
