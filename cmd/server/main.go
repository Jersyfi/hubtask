// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command server starts Hubtask in one or more roles (ADR-0014).
//
// What is already binding here and must not be watered down:
//   - Fail closed: if a required setting is missing, the process does not start.
//   - Separate health levels; /healthz never checks dependencies.
//   - Graceful shutdown: deregister from /readyz first, allow a grace period for the
//     load balancer, only then drain in-flight requests.
//   - Operations endpoints on a dedicated, non-public port.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// Set at build time via ldflags.
var (
	version   = "0.0.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Subcommand for the container health check: no extra tool needed in the
	// distroless image.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(selfCheck())
	}
	if err := run(); err != nil {
		slog.Error("startup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := envadapter.New(version, commit).Load()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	setupLogging(cfg)

	roles := make([]string, 0, len(cfg.Roles))
	for _, r := range cfg.Roles {
		roles = append(roles, string(r))
	}

	slog.Info("Hubtask starting",
		slog.String("commit", commit),
		slog.String("build_date", buildDate),
		slog.Any("roles", roles),
		slog.String("tenancy", string(cfg.Tenancy)),
	)

	registry := healthadapter.NewRegistry(version, roles)
	registry.SetWarnings(toPortWarnings(envadapter.New(version, commit).Warnings(cfg)))

	// TODO(0.1.0): register real probes once the adapters exist.
	// PostgreSQL is the only mandatory dependency; everything else is optional and may
	// only lead to reduced functionality when it fails (ADR-0016).
	registry.Register(healthadapter.StaticProbe{
		ProbeName: "postgres", IsRequired: true, Fixed: healthport.StatusOK,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	concurrency.SetPanicObserver(func(component string, _ any) {
		// TODO(0.1.0): increment hubtask_panics_recovered_total{component}.
		_ = component
	})

	ops := &http.Server{
		Addr:              cfg.OpsAddr,
		Handler:           rest.OpsController{Health: registry}.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	concurrency.Go(ctx, "server.ops", func(context.Context) {
		slog.Info("operations endpoints ready", slog.String("addr", cfg.OpsAddr))
		if err := ops.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("ops: %w", err)
		}
	})

	var api *http.Server
	if cfg.HasRole(envport.RoleAPI) {
		// TODO(0.1.0): mount the generated router from openapi.yaml, plus middleware for
		// auth, tenant context, locale, rate limit, idempotency, request ID.
		api = &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           http.NotFoundHandler(),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		concurrency.Go(ctx, "server.api", func(context.Context) {
			slog.Info("API ready", slog.String("addr", cfg.HTTPAddr))
			if err := api.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("api: %w", err)
			}
		})
	}

	// TODO(0.1.0): start the worker, scheduler and automation loops depending on the role.

	registry.MarkStarted()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("SIGTERM received, shutting down gracefully")
	}

	// The order matters: deregister from /readyz first, then wait out a grace period so
	// the load balancer stops sending new requests, and only then drain the in-flight
	// ones.
	registry.MarkClosing()
	time.Sleep(2 * time.Second)

	grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	var shutdownErr error
	if api != nil {
		if err := api.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if err := ops.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}

	slog.Info("stopped")
	return shutdownErr
}

func setupLogging(cfg envport.Config) {
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}

	var h slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(h).With(
		slog.String("service", "hubtask"),
		slog.String("version", cfg.Version),
	))
}

func toPortWarnings(in []envport.Warning) []healthport.Warning {
	out := make([]healthport.Warning, 0, len(in))
	for _, w := range in {
		out = append(out, healthport.Warning{Code: w.Code, Severity: w.Severity, Params: w.Params})
	}
	return out
}

// selfCheck queries the process's own liveness endpoint (container health check).
func selfCheck() int {
	addr := os.Getenv("HUBTASK_OPS_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
