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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// defaultOpsPort mirrors the default of HUBTASK_OPS_ADDR in infrastructure/environment.
const defaultOpsPort = 9090

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

	// The bootstrap logger exists before the configuration does, because the first thing that
	// can fail is reading the configuration - and that error is logged (T-18).
	slog.SetDefault(observability.NewLogger(
		envport.Config{Version: version, LogFormat: "json", LogLevel: "info"}, os.Stdout))

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
	// From here on everything goes through the redacting logger (T-18).
	slog.SetDefault(observability.NewLogger(cfg, os.Stdout))

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

	metrics, err := observability.NewMetrics(cfg)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metrics.Shutdown(shutdownCtx); err != nil {
			slog.Warn("flushing the metrics failed", slog.String("error", err.Error()))
		}
	}()

	registry := healthadapter.NewRegistry(version, roles)
	registry.SetWarnings(toPortWarnings(envadapter.New(version, commit).Warnings(cfg)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Tracing exists even when it is switched off: the W3C propagator is installed either way,
	// so an incoming traceparent still reaches the log and still travels onwards (§3.3). Off is
	// the documented self-hosting default (§13).
	startupCtx, cancelStartup := context.WithTimeout(ctx, 10*time.Second)
	tracing, err := observability.NewTracing(startupCtx, cfg)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracing.Shutdown(shutdownCtx); err != nil {
			slog.Warn("flushing the traces failed", slog.String("error", err.Error()))
		}
	}()
	slog.Info("tracing configured", slog.Bool("enabled", tracing.Enabled))

	// PostgreSQL is the only mandatory dependency (ADR-0003). Failing to reach it here stops
	// the process: a pod that starts without its database only moves the error to the first
	// request (ADR-0015).
	//
	// The pool belongs to the role that opens it, because the query budget differs - the API
	// gets seconds, background work gets a minute. A process in several roles runs the strictest
	// of them for now; A-08 gives the worker loops their own pool.
	pool, err := postgres.NewPool(ctx, cfg, primaryRole(cfg))
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	registry.Register(postgres.NewProbe(pool))

	// The panic metric is the one an alert watches, and its target value is 0 permanently
	// (ADR-0016). The recovered value itself is deliberately not logged here: a panic value can
	// carry anything, user content included (rule 10) - SafeGo logs it with the redacting
	// logger, and this only counts.
	concurrency.SetPanicObserver(func(component string, _ any) {
		metrics.PanicRecovered(context.Background(), component)
	})

	ops := &http.Server{
		Addr:              cfg.OpsAddr,
		Handler:           rest.OpsController{Health: registry, Metrics: metrics.Handler()}.Routes(),
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
		// TODO(0.1.0): mount the generated router from openapi.yaml into this mux, plus
		// middleware for auth, tenant context, locale, rate limit and idempotency.
		//
		// The observability middleware is already in place around it: from the first request
		// there is a RED metric, a span, and a request ID, and A-06 only adds routes inside.
		apiRoutes := http.NewServeMux()
		api = &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: rest.Observed{
				Router:  apiRoutes,
				Metrics: metrics,
				Tracer:  tracing.Tracer("rest"),
				Role:    string(envport.RoleAPI),
			},
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

// primaryRole picks the role whose query budget the pool runs under. The API is the strictest,
// so a process serving the API uses that budget for everything it does - being cut off early is
// the safer mistake on a shared pool.
func primaryRole(cfg envport.Config) envport.Role {
	if cfg.HasRole(envport.RoleAPI) {
		return envport.RoleAPI
	}
	return cfg.Roles[0]
}

func toPortWarnings(in []envport.Warning) []healthport.Warning {
	out := make([]healthport.Warning, 0, len(in))
	for _, w := range in {
		out = append(out, healthport.Warning{Code: w.Code, Severity: w.Severity, Params: w.Params})
	}
	return out
}

// selfCheck queries the process's own liveness endpoint (container health check).
//
// The target is always loopback: only the port is taken from the environment, and it goes
// through strconv, so nothing from the environment can reach the URL as text. That is also why
// this call does not use GuardedClient - its SSRF protection blocks loopback by design.
//
//nolint:gosec // G704: the host is the constant 127.0.0.1; the environment contributes a port number only
func selfCheck() int {
	port := defaultOpsPort
	if _, p, err := net.SplitHostPort(os.Getenv("HUBTASK_OPS_ADDR")); err == nil {
		if n, cerr := strconv.Atoi(p); cerr == nil && n > 0 && n <= 65535 {
			port = n
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
	if err != nil {
		return 1
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
