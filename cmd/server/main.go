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

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/idempotency"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/meta"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// defaultOpsPort mirrors the default of HUBTASK_OPS_ADDR in infrastructure/environment.
const defaultOpsPort = 9090

// healthSampleInterval is how often the deep report is produced for the metrics. Not
// configurable: it is a sampling rate for a handful of gauges, and every dependency probe is
// already bounded by its own timeout.
const healthSampleInterval = 30 * time.Second

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
	registry.SetSignals(metrics)

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

	// The catalogue is built once for the process, not once per channel. Every entry is wrapped
	// with the observer on the way in, so no use case can run without its metric and its span
	// (gate RT-12), and REST, MCP and automation then execute the same handlers (arc42 §4).
	unitOfWork := postgres.NewUnitOfWork(pool)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	// The device identifier of this process. Two replicas are two devices, which is what breaks a
	// tie between two changes stamped in the same millisecond (offline-sync.md §4.1).
	hybrid, err := clockadapter.NewHybridClock(clockadapter.System{}, "server-"+ids.NewID().String())
	if err != nil {
		return fmt.Errorf("clock: %w", err)
	}
	auditSink := postgres.NewAuditSink(ids)

	useCases, err := usecase.NewRegistry(
		observability.NewObserver(metrics, tracing).Registry(),
		work.CreateContainer{
			Containers: postgres.NewContainerRepository(),
			Authorizer: access.Service{
				Memberships: postgres.NewMembershipRepository(),
				UnitOfWork:  unitOfWork,
				Audit:       auditSink,
				Clock:       clockadapter.System{},
			},
			Events:     postgres.NewOutbox(),
			Changes:    postgres.NewChangeLog(),
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
	)
	if err != nil {
		// A use case registered without its audit declaration or its handler stops the process
		// rather than being discovered later by whoever needed the audit entry (ADR-0015).
		return fmt.Errorf("use case catalogue: %w", err)
	}

	var api *http.Server
	if cfg.HasRole(envport.RoleAPI) {
		// The routes come from api/openapi.yaml through the generated registration list; nothing
		// here names a path (ADR-0004). Operations without a use case yet answer 404 - the route
		// exists because the contract declares it, not because it works.
		controller := rest.NewRestController()
		controller.UseCases = useCases
		controller.Capabilities = meta.GetCapabilities{
			Profiles:   postgres.NewCapabilityProfileRepository(),
			UnitOfWork: unitOfWork,
			Config:     cfg,
		}
		apiRoutes := controller.Routes()

		authenticate := identity.AuthenticateToken{
			Tokens:     postgres.NewAccessTokenRepository(security.NewTokenHasher(cfg.SecretKey)),
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
		}

		// One limiter, two levels: per credential or client address before authentication, per
		// tenant after it. The first needs no database - it keys on a fingerprint of the
		// presented string - so a flood of invalid tokens costs no lookups.
		limiter := rest.NewRateLimiter()

		// The chain, from the outside in. Observed stays outermost: a panic anywhere below it
		// still becomes a problem document, and every answer carries a request ID and a metric -
		// including the ones no handler produced. Authentication sits inside the bounds and
		// inside the first limit, so neither an oversized body nor a flood reaches a database
		// lookup.
		api = &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: rest.Observed{
				Router: rest.Chain{
					Routes: apiRoutes,
					Entry: rest.Secured{CORS: cfg.CORS, Next: rest.Bounded{
						MaxBodyBytes: cfg.Request.MaxBodyBytes,
						Timeout:      cfg.Request.Timeout,
						Next: rest.Limited{
							Limiter: limiter,
							Level:   "credential",
							Bucket: rest.CredentialBucket(
								cfg.RateLimit.AnonymousPerMinute,
								cfg.RateLimit.TokenPerMinute,
								cfg.RateLimit.Burst),
							Next: rest.Localised{
								Locale: cfg.Locale,
								Next: rest.Authenticated{
									Routes:        apiRoutes,
									Authenticator: authenticate,
									Locale:        cfg.Locale,
									Next: rest.Limited{
										Limiter: limiter,
										Level:   "tenant",
										Bucket: rest.TenantBucket(
											cfg.RateLimit.TenantPerMinute, cfg.RateLimit.Burst),
										Next: rest.Idempotent{
											Guard:  idempotency.Guard{Store: postgres.NewIdempotencyStore(), UnitOfWork: unitOfWork},
											Routes: apiRoutes,
											Next:   apiRoutes,
										},
									},
								},
							},
						},
					}},
				},
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

	// hubtask_dependency_up is described as "self-diagnosis as a time series" (§4), and a series
	// needs a regular sample. Nothing scrapes /meta/health, and /readyz only touches the
	// mandatory dependencies - so the full report is produced on a timer and mirrored into the
	// gauges from there.
	concurrency.Go(ctx, "health.sampler", func(ctx context.Context) {
		ticker := time.NewTicker(healthSampleInterval)
		defer ticker.Stop()
		for {
			registry.Report(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})

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
