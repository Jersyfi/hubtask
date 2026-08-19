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
	queueport "github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
	"github.com/Jersyfi/hubtask/presentation/worker"
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
	// gets seconds, background work gets a minute. A process in several roles opens a second pool
	// below for the background ones, so that a runaway job cannot take the connections the
	// interactive path needs (bulkheads, observability-reliability.md §6).
	pool, err := postgres.NewPool(ctx, cfg, primaryRole(cfg))
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	// The background roles get a pool of their own whenever this process also serves the API.
	// Their budget is the long one, and their saturation is theirs alone: a worker holding ten
	// connections for a minute each is normal, and the same behaviour on the API pool is an
	// outage.
	backgroundPool := pool
	if cfg.HasRole(envport.RoleAPI) && runsBackgroundWork(cfg) {
		backgroundPool, err = postgres.NewPool(ctx, cfg, envport.RoleWorker)
		if err != nil {
			return fmt.Errorf("database (background): %w", err)
		}
		defer backgroundPool.Close()
	}

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
	// The queue is wired into the write path before any worker exists: an event carries its own
	// wake-up, so the dispatcher finds work whether or not this process is the one that runs it.
	jobs := postgres.NewQueue(ids, clockadapter.System{})

	// One observer for both channels: a use case gets its span through the registry middleware, a
	// job through the runner's hook. Two constructions would be two tracers with the same name.
	observer := observability.NewObserver(metrics, tracing)

	// The identity use cases share their dependencies, so they are built once rather than seven
	// times: the same authoriser, the same audit sink, the same clock.
	authorizer := access.Service{
		Memberships: postgres.NewMembershipRepository(),
		UnitOfWork:  unitOfWork,
		Audit:       auditSink,
		Clock:       clockadapter.System{},
	}
	accounts := postgres.NewAccountRepository()
	groups := postgres.NewGroupRepository()
	grants := postgres.NewMembershipGrantRepository()

	// The work management use cases share theirs the same way. The capability profiles in
	// particular: /meta/capabilities answers from the same reader that decides whether a
	// placement is permitted, so what an installation advertises and what it accepts cannot
	// drift apart (ADR-0006).
	//
	// The cursor codec is shared by both list repositories rather than derived twice: it is keyed on
	// the installation secret, and one derivation means one place where that key comes from
	// (api-guidelines.md §4).
	cursors := security.NewCursorCodec(cfg.SecretKey)
	containers := postgres.NewContainerRepository(cursors)
	items := postgres.NewItemRepository(cursors)
	profiles := postgres.NewCapabilityProfileRepository()
	buckets := postgres.NewBucketRepository()
	labels := postgres.NewLabelRepository()
	itemLabels := postgres.NewItemLabelRepository()
	outbox := postgres.NewOutbox(jobs)
	changes := postgres.NewChangeLog()

	// Both directions of completion share one dependency set. The two operations are the same walk in
	// opposite directions, and wiring them separately would be two places to get one of eleven fields
	// wrong (work.CompletionWriter).
	completion := work.CompletionWriter{
		Items: items, Containers: containers, Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Moving and reordering share one dependency set, on the reasoning that keeps them one event type: a
	// reorder is a move that keeps its parent (work.PlacementWriter).
	placement := work.PlacementWriter{
		Items: items, Buckets: buckets, Containers: containers, Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Every verb that moves an entry between the archive and the trash shares one dependency set.
	// They are the same walk with a different transition in the middle, and wiring them separately
	// would be one more place for "restoring records what trashing records" to stop being true
	// (work.LifecycleWriter).
	lifecycle := work.LifecycleWriter{
		Items: items, Containers: containers, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Every verb that changes an existing container shares one dependency set: they read the same
	// container, ask the same permission question, and owe the same four writes
	// (work.ContainerWriter).
	containerWriter := work.ContainerWriter{
		Containers: containers, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Every verb that changes an existing column shares one dependency set: they read the same
	// bucket, ask the same permission question of the same collection, and owe the same four writes
	// (work.BucketWriter).
	bucketWriter := work.BucketWriter{
		Buckets: buckets, Containers: containers, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both verbs that change an existing label share one dependency set, for the reason the bucket
	// verbs do (work.LabelWriter).
	labelWriter := work.LabelWriter{
		Labels: labels, Containers: containers, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both directions of an entry's labels share one dependency set. They are the same write in
	// opposite directions - the same capability gate, the same collection check, the same tag -
	// and wiring them separately would be two places to get one of thirteen fields wrong
	// (work.ItemLabelWriter).
	itemLabelWriter := work.ItemLabelWriter{
		Items: items, ItemLabels: itemLabels, Labels: labels, Containers: containers,
		Profiles: profiles, Authorizer: authorizer, Events: outbox, Changes: changes,
		Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		HLC: hybrid,
	}

	useCases, err := usecase.NewRegistry(
		observer.Registry(),
		identity.InviteAccount{
			Accounts: accounts, Authorizer: authorizer, Notifier: jobs, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		identity.UpdateAccountPreferences{
			Accounts: accounts, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		identity.GrantMembership{
			Grants: grants, Accounts: accounts, Groups: groups, Authorizer: authorizer,
			Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		identity.RevokeMembership{
			Grants: grants, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		identity.CreateGroup{
			Groups: groups, Accounts: accounts, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		identity.UpdateGroup{
			Groups: groups, Accounts: accounts, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		identity.DeleteGroup{
			Groups: groups, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		work.CreateContainer{
			Containers: containers,
			Authorizer: authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
		work.CreateWorkItem{
			Items:      items,
			Buckets:    buckets,
			Containers: containers,
			Profiles:   profiles,
			Authorizer: authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
		work.UpdateWorkItem{
			Items:      items,
			Buckets:    buckets,
			Containers: containers,
			Profiles:   profiles,
			Authorizer: authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
		work.RenameContainer{Writer: containerWriter}.Descriptor(),
		work.UpdateContainerPolicies{Writer: containerWriter}.Descriptor(),
		work.ArchiveContainer{Writer: containerWriter}.Descriptor(),
		work.UnarchiveContainer{Writer: containerWriter}.Descriptor(),
		work.MoveContainer{Writer: containerWriter}.Descriptor(),
		work.TrashContainer{Writer: containerWriter}.Descriptor(),
		work.RestoreContainer{Writer: containerWriter}.Descriptor(),
		work.CreateBucket{
			Buckets:    buckets,
			Containers: containers,
			Authorizer: authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
		work.ListBuckets{
			Buckets: buckets, Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateBucket{Writer: bucketWriter}.Descriptor(),
		work.ReorderBucket{Writer: bucketWriter}.Descriptor(),
		work.DeleteBucket{Writer: bucketWriter}.Descriptor(),
		work.CreateLabel{
			Labels:     labels,
			Containers: containers,
			Authorizer: authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
		}.Descriptor(),
		work.ListLabels{
			Labels: labels, Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateLabel{Writer: labelWriter}.Descriptor(),
		work.DeleteLabel{Writer: labelWriter}.Descriptor(),
		work.AddLabel{Writer: itemLabelWriter}.Descriptor(),
		work.RemoveLabel{Writer: itemLabelWriter}.Descriptor(),
		work.GetContainer{
			Containers: containers, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		// The authorisation service twice, under two names: Authorize for the level a client named,
		// Permitted for the hub level, which is anchored to nothing and is narrowed to what the actor
		// may see rather than refused outright (ReadContainers.Execute).
		work.ListContainers{
			Containers: containers, Authorizer: authorizer, Reader: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetWorkItem{
			Items: items, ItemLabels: itemLabels, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListWorkItems{
			Items: items, ItemLabels: itemLabels, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.CompleteWorkItem{Completion: completion}.Descriptor(),
		work.ReopenWorkItem{Completion: completion}.Descriptor(),

		work.MoveWorkItem{Placement: placement}.Descriptor(),
		work.ReorderWorkItem{Placement: placement}.Descriptor(),
		work.ArchiveWorkItem{Lifecycle: lifecycle}.Descriptor(),
		work.UnarchiveWorkItem{Lifecycle: lifecycle}.Descriptor(),
		work.TrashWorkItem{Lifecycle: lifecycle}.Descriptor(),
		work.RestoreWorkItem{Lifecycle: lifecycle}.Descriptor(),
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
			Profiles:   profiles,
			UnitOfWork: unitOfWork,
			Config:     cfg,
		}
		// The MCP endpoint is mounted beside the specification's routes rather than on them: it is
		// JSON-RPC over one path, not a REST resource, so it belongs in no OpenAPI document - and
		// it still travels through the whole middleware chain, which is what makes an agent's call
		// authenticated, rate limited and observed exactly like a person's (ai-first.md §1.1).
		apiRoutes := rest.Mounted{
			Router: controller.Routes(),
			Path:   mcp.Path,
			Mount: mcp.Server{
				Catalogue: useCases,
				Name:      "hubtask",
				Version:   version,
			},
		}

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

	// The loops this process runs, so that the shutdown can wait for them. Without that the grace
	// period would protect the request path only, and a pod being replaced would cut a job in
	// half and leave its lease to expire (deployment.md §5).
	var background []<-chan struct{}

	// The queue channel. It is wired whatever the roles are, because the pieces are the same;
	// what the roles decide is which loops run (ADR-0014).
	backgroundWork := postgres.NewUnitOfWork(backgroundPool)
	dispatcher := eventbus.Dispatcher{
		Events:   postgres.NewOutbox(jobs),
		Consumed: postgres.NewConsumption(clockadapter.System{}),
		// No consumers yet: automation, webhooks, the live stream and the search index register
		// here as they arrive. Until then a round still marks its events as delivered - "nobody
		// subscribes" is an answer, and an outbox that only grows would raise the backlog alert
		// for a system that is working.
		Subscribers: nil,
		Clock:       clockadapter.System{},
		Batch:       cfg.Queue.OutboxBatch,
		MinInterval: cfg.Queue.OutboxMinInterval,
		MaxInterval: cfg.Queue.OutboxMaxInterval,
		Lag:         metrics.OutboxLag,
	}

	// The kinds this build knows. The scheduler publishes a zero for each of them, so that the
	// backlog gauge exists before there is a backlog.
	handlers := map[queueport.Kind]queueport.Handler{queueport.KindOutboxDispatch: dispatcher}
	kinds := make([]queueport.Kind, 0, len(handlers))
	for kind := range handlers {
		kinds = append(kinds, kind)
	}

	if cfg.HasRole(envport.RoleWorker) {
		// The backoff policy is the resilience adapter's, handed to the runner as a function: the
		// presentation layer decides when to retry, not how far apart (project-structure.md §2).
		backoff := resilience.Backoff{
			Attempts: cfg.Queue.MaxAttempts,
			Base:     cfg.Queue.RetryBase,
			Max:      cfg.Queue.RetryMax,
		}
		runner := worker.Runner{
			Queue:        jobs,
			UnitOfWork:   backgroundWork,
			Handlers:     handlers,
			Clock:        clockadapter.System{},
			Signals:      metrics,
			Batch:        cfg.Queue.BatchSize,
			PollInterval: cfg.Queue.PollInterval,
			JobTimeout:   cfg.Queue.JobTimeout,
			Lease:        cfg.Queue.Lease(),
			NextAttempt:  backoff.Delay,
			Observe:      observer.Job,
		}
		background = append(background, start(ctx, "worker.runner", runner.Run))
	}

	if cfg.HasRole(envport.RoleScheduler) {
		// The leader holds one connection of the background pool for as long as it leads, which is
		// why that pool is never sized at one.
		scheduler := worker.Scheduler{
			Leadership:   postgres.NewLeader(backgroundPool, postgres.SchedulerLock),
			Queue:        jobs,
			UnitOfWork:   backgroundWork,
			Clock:        clockadapter.System{},
			Signals:      metrics,
			Kinds:        kinds,
			TickInterval: cfg.Queue.SchedulerTick,
		}
		background = append(background, start(ctx, "worker.scheduler", scheduler.Run))
	}

	// TODO(0.1.0): start the automation loop for the automation role.

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

	// The background loops stopped claiming when the context ended; what they had already claimed
	// runs to its own deadline. Waiting for it here is what makes terminationGracePeriodSeconds
	// mean something for jobs rather than only for requests.
	for _, done := range background {
		select {
		case <-done:
		case <-shutdownCtx.Done():
			slog.Warn("a background loop did not finish within the grace period")
		}
	}

	slog.Info("stopped")
	return shutdownErr
}

// start runs a loop and hands back the channel that closes when it has finished. concurrency.Go
// is the only place a goroutine may be created (rule 5); what it does not offer is a way to wait
// for one, and a shutdown that cannot wait is a shutdown that only looks graceful.
func start(ctx context.Context, name string, loop func(context.Context)) <-chan struct{} {
	done := make(chan struct{})
	concurrency.Go(ctx, name, func(ctx context.Context) {
		defer close(done)
		loop(ctx)
	})
	return done
}

// runsBackgroundWork reports whether this process runs a loop of its own rather than only serving
// requests.
func runsBackgroundWork(cfg envport.Config) bool {
	return cfg.HasRole(envport.RoleWorker) || cfg.HasRole(envport.RoleScheduler)
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
