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

	"github.com/knowoff/knowoff/server/internal/config"
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

	db, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	mediaManager := media.NewManager(nil)
	if cfg.Media.LocalBundlePath != "" {
		pack, err := media.LoadPack(cfg.Media.LocalBundlePath, dealingTuningFromConfig(cfg))
		if err != nil {
			logger.Error("failed to load media pack", "path", cfg.Media.LocalBundlePath, "error", err)
		} else {
			mediaManager.Load(pack)
			logger.Info("media pack loaded", "tag", pack.Manifest.PackTag)
		}
	}

	deps := transport.Deps{
		Config:      cfg,
		Logger:      logger,
		DB:          db,
		RedisPing:   redisPingFunc(cfg),
		StoragePing: storagePingFunc(cfg),
		Connections: connections,
		Media:       mediaManager,
	}

	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/healthz", transport.HealthzHandler(deps))
	publicMux.HandleFunc("/readyz", transport.ReadyzHandler(deps))
	publicMux.HandleFunc("/ws", transport.WebSocketHandler(deps))

	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/healthz", transport.HealthzHandler(deps))
	adminMux.HandleFunc("/readyz", transport.ReadyzHandler(deps))
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
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.BindAddr, cfg.Server.Port),
		Handler:      transport.RecoverPanic(publicMux, logger),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutS) * time.Second,
	}

	adminServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.AdminBindAddr, cfg.Server.AdminPort),
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

	transport.SetReady(true)

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
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
