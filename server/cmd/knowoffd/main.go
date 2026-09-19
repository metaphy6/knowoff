package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knowoff/knowoff/server/internal/admin"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/avatar"
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

func run() (runErr error) {
	if len(os.Args) > 1 && os.Args[1] == "release-manifest" {
		if len(os.Args) != 2 {
			return fmt.Errorf("usage: knowoffd release-manifest")
		}
		return writeReleaseManifest(os.Stdout)
	}
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

	// Only successfully upgraded text sockets contribute to the connection gauge.
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

	startup, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	textService, err := newTextRuntime(startup, db, cfg, os.Getenv("KNOWOFF_TEXT_PROTOTYPE_PACK"))
	startupCancel()
	if err != nil {
		return fmt.Errorf("initialize text runtime: %w", err)
	}
	lifecycleManaged := false
	defer func() {
		if lifecycleManaged {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace())
		defer cancel()
		if err := textService.Close(cleanup); err != nil {
			logger.Error("text runtime shutdown failed", "error", err)
		}
	}()

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
				GraphVersion: cfg.Security.OAuth.Facebook.GraphVersion,
			},
		})
	if err := authManager.ConfigureDevelopment(cfg.App.Env, textService.Lobby.Prototype()); err != nil {
		return fmt.Errorf("configure development authentication: %w", err)
	}
	profileManager := profile.NewManager(db)
	leaderboardManager := leaderboard.NewManager(db)
	economyManager := economy.NewManager(db, cfg)
	verifiedPurchases, err := economy.NewPlatformPurchases(db, cfg)
	if err != nil {
		return fmt.Errorf("initialize billing: %w", err)
	}
	economyManager.Purchases = verifiedPurchases

	redisClient := store.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	defer redisClient.Close()
	deps := transport.Deps{
		Config: cfg, Logger: logger, DB: db,
		RedisPing:    redisClient.Ping,
		RuntimeReady: textService.Lobby.RuntimeReady,
	}

	adminManager := admin.NewManager(db, cfg, redisClient)
	noticesManager := notices.NewManager(db, cfg, textService.Lobby)
	noticesManager.SetChangeNotifier(textService.Lobby.NotifyNotices)
	reportsManager := reports.NewManager(db)
	avatarManager := avatar.NewManager(db, cfg, economyManager)
	portalManager := portal.NewManager(portal.Deps{
		DB:       db,
		Config:   cfg,
		Auth:     authManager,
		Profile:  profileManager,
		Economy:  economyManager,
		Admin:    adminManager,
		Screener: portal.NewTextScreener(cfg.Moderation.ContentScreening),
	})
	textService.bindModeration(portalManager, authManager)
	// Contribution intake stays closed until an admin publishes real terms.

	rewardHTTP, err := newTextRewardHTTP(cfg.Rewarded, db, authManager, textService.Values, nil)
	if err != nil {
		return fmt.Errorf("initialize text rewards: %w", err)
	}
	publicMux := http.NewServeMux()
	rewardHTTP.register(publicMux)
	lifecycle := newRuntimeLifecycle()
	textHandlerDeps := handler.TextHandlerDeps{Config: cfg, Lobby: textService.Lobby, Auth: authManager, ConnLimiter: connLimiter, Deliveries: textService.Values, DeliveryWorker: textService.Lobby.Owner(), Connections: connections, ConnectionRejections: connLimiterRejections}
	handler.RegisterTextRealtimeRoutes(publicMux, textHandlerDeps)
	handler.RegisterAuthRoutes(publicMux, handler.AuthDeps{Auth: authManager, DevBotKey: cfg.Security.DevBotKey, OAuthTrustedProxyCIDRs: cfg.Security.OAuth.TrustedProxyCIDRs})
	handler.RegisterSafetyRoutes(publicMux, handler.SafetyDeps{Auth: authManager, Trust: textService.Trust, Config: cfg.Trust, RoomAccount: textService.Lobby.RoomAccount})
	handler.RegisterProfileRoutes(publicMux, handler.ProfileDeps{Profile: profileManager, Leaderboard: leaderboardManager}, authManager)
	handler.RegisterPublicRoutes(publicMux, handler.PublicRouteDeps{
		VisibleText: textService.Lobby.VisibleText,
		Auth:        authManager,
		Notices:     noticesManager,
		Reports:     reportsManager,
		Avatar:      avatarManager,
	})
	handler.RegisterEconomyRoutes(publicMux, handler.EconomyDeps{
		WalletAccess: textService.Lobby.WithWalletAccess,
		Config:       cfg,
		Auth:         authManager,
		Economy:      economyManager,
	})
	publicMux.Handle("/portal/", portalManager.Handler())
	publicMux.Handle("POST /api/portal/connect", portalManager.ConnectHandler())
	handler.RegisterChallengeRoutes(publicMux, handler.ChallengeDeps{
		Auth:   authManager,
		Portal: portalManager,
	})

	adminMux := http.NewServeMux()
	operatorHandler := adminManager.OperatorHandler(textService.operatorHooks(db, authManager, economyManager))
	adminMux.Handle("/admin/operators", operatorHandler)
	adminMux.Handle("/admin/operators/", operatorHandler)
	adminMux.Handle("/admin/runtime/", adminManager.RuntimeHandler(textService.adminRuntimeHooks()))
	adminMux.Handle("/admin/portal/", adminManager.PortalHandler(portalManager))
	adminMux.Handle("/admin/", adminManager.Handler(noticesManager))

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	publicServer := &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.BindAddr, cfg.Server.Port),
		Handler: transport.CORS(
			transport.RecoverPanic(lifecycle.Handler(publicMux, deps), logger),
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
		Handler: transport.RecoverPanic(lifecycle.Handler(adminMux, deps), logger),
	}

	metricsServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.MetricsBindAddr, cfg.Server.MetricsPort),
		Handler: metricsMux,
	}

	refreshRuntimeHealth(context.Background(), deps, textService.Lobby, noticesManager.MarkMaintenanceDrain, logger)
	shutdownGrace := cfg.ShutdownGrace()
	lifecycleManaged = true
	defer func() {
		runErr = errors.Join(runErr, lifecycle.Shutdown(shutdownGrace, textService.Close, publicServer, adminServer, metricsServer))
		if runErr == nil {
			logger.Info("server stopped")
		}
	}()
	// The worker gate owns child cancellation so grace never cancels Tick early.
	runCtx := context.Background()
	if !textService.startBonusRecovery(runCtx, lifecycle.Workers, func(err error) { logger.Error("text bonus recovery pending", "error", err) }) {
		return fmt.Errorf("start text bonus recovery: ownership unavailable")
	}

	errCh := make(chan error, 3)
	for _, server := range []*http.Server{publicServer, adminServer, metricsServer} {
		lifecycle.Workers.Go(runCtx, func(context.Context) { errCh <- server.ListenAndServe() })
	}

	transport.SetReady(true)
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		runHealthWatcher(ctx, deps, textService.Lobby, noticesManager.MarkMaintenanceDrain, logger)
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		noticesManager.Run(ctx, func(err error) { logger.Error("notice refresh pending", "error", err) })
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		verifiedPurchases.RunProviderTasks(ctx, func(err error) { logger.Error("billing reconciliation pending", "error", err) })
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		portalManager.RunCommunity(ctx, func(err error) { logger.Error("community maintenance pending", "error", err) })
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		textService.runOperatorMaintenance(ctx, db, func(err error) { logger.Error("operator delivery pending", "error", err) })
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		textService.Lobby.Run(ctx, func(err error) { logger.Error("text match tick failed", "error", err) })
	})
	lifecycle.Workers.Go(runCtx, func(ctx context.Context) {
		textService.Lobby.RunDeliveries(ctx, textService.Values, func(err error) { logger.Error("text delivery retry pending", "error", err) })
	})

	logger.Info("server started",
		"public", publicServer.Addr,
		"admin", adminServer.Addr,
		"metrics", metricsServer.Addr,
		"env", cfg.App.Env,
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	case <-textService.Owner.Done():
		shutdownGrace = 0 // Lost authority cannot progress gameplay grace.
		return fmt.Errorf("text match ownership lost")
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

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

// refreshRuntimeHealth changes dependency and maintenance admission fences only.
// Process shutdown readiness and permanent owner loss cannot be undone by a probe.
func refreshRuntimeHealth(ctx context.Context, deps transport.Deps, manager *lobby.TextManager, maintenance func(context.Context) error, logger *slog.Logger) {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
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
	manager.SetDependencyReady(ok)
	if maintenance != nil {
		if err := maintenance(checkCtx); err != nil {
			manager.SetReady(false)
			logger.Warn("maintenance status unavailable", "error", err)
		}
	}
}

func runHealthWatcher(ctx context.Context, deps transport.Deps, manager *lobby.TextManager, maintenance func(context.Context) error, logger *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshRuntimeHealth(ctx, deps, manager, maintenance, logger)
		}
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
