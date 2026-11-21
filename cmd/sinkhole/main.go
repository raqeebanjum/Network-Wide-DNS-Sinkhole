package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/api"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/config"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/dnsserver"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/logging"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sinkhole: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat)
	logger.Info("starting network-wide DNS sinkhole",
		"dns_address", cfg.DNSListen,
		"http_address", cfg.HTTPListen,
		"data_dir", cfg.DataDir,
	)

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data directory %q: %w", cfg.DataDir, err)
	}

	database, err := store.Open(filepath.Join(cfg.DataDir, "sinkhole.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()

	ruleManager, err := rules.NewManager(context.Background(), database)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	logger.Info("rules loaded", "count", ruleManager.Count())

	dnsService, err := dnsserver.New(dnsserver.Config{
		Listen:         cfg.DNSListen,
		Upstreams:      cfg.Upstreams,
		RequestTimeout: cfg.DNSRequestTimeout,
		MaxInflight:    cfg.DNSMaxInflight,
		Matcher:        ruleManager,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("create DNS server: %w", err)
	}
	if err := dnsService.Start(); err != nil {
		return fmt.Errorf("start DNS server: %w", err)
	}
	logger.Info("DNS server listening",
		"address", dnsService.Address(),
		"upstreams", cfg.Upstreams,
	)

	httpServer := &http.Server{
		Addr: cfg.HTTPListen,
		Handler: api.NewHandler(api.Dependencies{
			Rules:  ruleManager,
			Logger: logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "address", cfg.HTTPListen)
		httpErrors <- httpServer.ListenAndServe()
	}()

	var serveErr error
	select {
	case serveErr = <-httpErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		} else {
			serveErr = fmt.Errorf("serve HTTP: %w", serveErr)
		}
	case serveErr = <-dnsService.Errors():
		serveErr = fmt.Errorf("serve DNS: %w", serveErr)
	case <-ctx.Done():
		logger.Info("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var shutdownErrors []error
	if err := dnsService.Shutdown(shutdownCtx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shut down DNS server: %w", err))
	}
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		_ = httpServer.Close()
		shutdownErrors = append(shutdownErrors, fmt.Errorf("shut down HTTP server: %w", err))
	}

	logger.Info("shutdown complete")
	return errors.Join(serveErr, errors.Join(shutdownErrors...))
}
