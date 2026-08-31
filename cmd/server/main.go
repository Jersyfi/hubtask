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
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	auditrepo "github.com/Jersyfi/hubtask/core/application/repository/audit"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	idempotencyrepo "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/application/service/idempotency"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/application/service/meta"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	integrationmodel "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	eventbusport "github.com/Jersyfi/hubtask/core/port/eventbus"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	mailport "github.com/Jersyfi/hubtask/core/port/mail"
	persistenceport "github.com/Jersyfi/hubtask/core/port/persistence"
	queueport "github.com/Jersyfi/hubtask/core/port/queue"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	dbfiles "github.com/Jersyfi/hubtask/db"
	auditadapter "github.com/Jersyfi/hubtask/infrastructure/audit"
	"github.com/Jersyfi/hubtask/infrastructure/automation"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	celexpression "github.com/Jersyfi/hubtask/infrastructure/expression"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	mailadapter "github.com/Jersyfi/hubtask/infrastructure/mail"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/infrastructure/stepup"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
	"github.com/Jersyfi/hubtask/infrastructure/webhook"
	"github.com/Jersyfi/hubtask/presentation/intake"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/rest"
	"github.com/Jersyfi/hubtask/presentation/webui"
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

	// The object store exists from here on, although the first use case that consumes it is
	// C-06's: the configuration surface has existed since A-02, and an operator who pointed the
	// process at a bucket deserves to read in /meta/health that it is unreachable rather than to
	// find out at the first upload (QS-11). Local storage carries no breaker and no probe - a
	// directory has no circuit to trip, and its failures are the disk's, reported per call.
	// Who mints the URLs the bytes travel through comes back with the store, because the two
	// answers are one decision: a bucket signs its own transfers, and a local installation lets
	// this server's content routes stand in for it with a token (C-06, arc42 §8.4).
	mediaTokens := security.NewMediaTokenIssuer(cfg.SecretKey)
	mediaStore, mediaTransfers, err := buildObjectStore(cfg, mediaTokens, registry, metrics)
	if err != nil {
		return fmt.Errorf("object storage: %w", err)
	}
	mediaGuard := storageadapter.NewUploadGuard()

	// The stream's cursor codec and its wake-up. The codec is keyed on the installation secret
	// like every other opaque value this server mints; the listener holds one connection of the
	// pool for as long as the process serves streams (ADR-0007).
	streamCursors := streamCursorAdapter{codec: security.NewStreamCursorCodec(cfg.SecretKey)}
	changeListener := postgres.NewChangeListener(pool)

	// The change stream's process-local bookkeeping (C-10). Built before the chain, because the
	// shutdown path needs it as much as the handler does: a stream has no natural end, so nothing
	// but CloseAll tells it there is one.
	streams := rest.NewStreamRegistry(rest.StreamLimits{
		// Four per client, so an ordinary application with a couple of tabs open is never refused
		// and a client reconnecting in a loop is. Sixty-four per workspace and two hundred and
		// fifty-six per pod: a bound on the resource rather than a tuning knob, sized so that the
		// file descriptors and the goroutine stacks of a full complement stay well inside what a
		// container with the default limits has.
		PerCredential: 4,
		PerTenant:     64,
		PerProcess:    256,
	})

	// The one channel that sends in this milestone, and the renderer that decides its language
	// (C-09). Both are built whatever the roles are, because the pieces are the same; what the
	// roles decide is which loops run (ADR-0014).
	mailSender := buildMailSender(cfg, registry, metrics)
	renderer, err := i18n.NewRenderer()
	if err != nil {
		return fmt.Errorf("message catalogue: %w", err)
	}

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
	// The same rows, asked the other question: jobRecords is what a caller polls, and it
	// shares no statement with the queue - see infrastructure/postgres/JobRepository.go.
	jobRecords := postgres.NewJobRepository()

	// The backup targets (E-03). The guard is built here rather than inside the registry because
	// it is the installation's egress policy and not the backup context's: the day a webhook
	// leaves this process, it leaves through the same one.
	//
	// A backup target's address arrives through the API, which is what makes it a T-07 subject
	// where the media endpoint is not - so a target on a private network needs
	// HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS, deliberately and once per installation.
	outboundGuard := httpclient.NewGuard(cfg.Outbound)
	outboundClient := httpclient.NewGuardedClient(cfg.Outbound, outboundGuard).
		WithTracer(tracing.Tracer("backup"))
	backupTargets := postgres.NewBackupTargetRepository()
	backupAdapters := backupstorage.NewRegistry(
		outboundClient, outboundGuard, cfg.Backup.LocalRoot, cfg.Outbound.Timeout, time.Now)
	// The envelope, whose keyring is empty on an installation that configured none - and which
	// then refuses to seal rather than writing a credential in the clear (E-02).
	keyring, err := crypto.NewKeyring(masterKeys(cfg))
	if err != nil {
		return fmt.Errorf("encryption keyring: %w", err)
	}
	encryptor := crypto.NewEnvelope(keyring, clockadapter.CryptoRandom{})

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
	// The three backup use cases share one writer, so that they cannot disagree about which
	// encryptor sealed a credential - three that did would be three chances to seal one under a
	// key the others cannot open.
	backupWriter := backupservice.Writer{
		Targets: backupTargets, Opener: backupAdapters, Encryptor: encryptor,
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, Config: cfg,
	}
	// The run use cases share theirs for the same reason: three that disagreed about the clock
	// would record a run at a moment nothing else agrees with (E-05).
	backupRuns := postgres.NewBackupRunRepository()
	backupSchedules := postgres.NewBackupScheduleRepository()
	backupRunner := backupservice.Runner{
		Runs: backupRuns, Targets: backupTargets, Jobs: jobs,
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}
	backupScheduling := backupservice.Scheduling{
		Schedules: backupSchedules, Targets: backupTargets, Jobs: jobs,
		Expander: recurrenceadapter.New(), Authorizer: authorizer, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
	}
	// And the restore side (E-06). It gets the same cipher the run job uses, because listing an
	// archive and restoring one have to agree about how a member was closed - two ciphers here
	// would look exactly like a wrong key.
	backupRestores := postgres.NewRestoreRunRepository()
	backupRestorer := backupservice.Restorer{
		Targets: backupTargets, Restores: backupRestores,
		Workspace: postgres.NewWorkspaceRepository(), Jobs: jobs,
		// The step-up nothing can satisfy yet, as a type rather than a nil: a missing verifier
		// would make "this installation has no step-up" indistinguishable from "somebody forgot to
		// wire one up", and the second is how a destructive restore ends up permitted by omission
		// (E-06, backup-restore.md §8.3).
		StepUp:    stepup.Unavailable{},
		Encryptor: encryptor, Opener: backupAdapters,
		Cipher: crypto.NewStream(clockadapter.CryptoRandom{}), Authorizer: authorizer,
		Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
	}

	accounts := postgres.NewAccountRepository()
	groups := postgres.NewGroupRepository()
	grants := postgres.NewMembershipGrantRepository()

	// The webhook subscriptions (G-03). One dependency set for the same reason the credentials
	// have one: the rule that decides who may touch a subscription is a single rule.
	//
	// The encryptor is the one E-02 built. A signing secret is sealed exactly as a backup
	// target's credential is, under a purpose that names the row - so a ciphertext lifted out of
	// one subscription and dropped into another no longer opens.
	webhookWriter := integrationservice.Writer{
		Subscriptions: postgres.NewWebhookSubscriptionRepository(),
		Deliveries:    postgres.NewWebhookDeliveryRepository(),
		Authorizer:    authorizer, Encryptor: encryptor, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		Entropy: clockadapter.CryptoRandom{},
	}

	// The three credential use cases share one dependency set, because the rule about whose
	// tokens somebody may touch is one rule (G-01). The known scopes come from the catalogue
	// rather than a list: a use case cannot read the catalogue it is part of, and a list beside
	// the descriptors is one that grows a scope no operation checks.
	accessTokenWriter := identity.AccessTokenWriter{
		Tokens:   postgres.NewAccessTokenRepository(security.NewTokenHasher(cfg.SecretKey)),
		Accounts: accounts, Authorizer: authorizer, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		Entropy:     clockadapter.CryptoRandom{},
		KnownScopes: catalogue.Scopes(),
	}

	// The service accounts share theirs for the same reason: creating one and listing them are
	// the same permission over the same store.
	serviceAccounts := identity.ServiceAccounts{
		Accounts: accounts, Authorizer: authorizer, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
	}

	// The work management use cases share theirs the same way. The capability profiles in
	// particular: /meta/capabilities answers from the same reader that decides whether a
	// placement is permitted, so what an installation advertises and what it accepts cannot
	// drift apart (ADR-0006).
	//
	// The cursor codec is shared by every list repository rather than derived once each: it is keyed
	// on the installation secret, and one derivation means one place where that key comes from
	// (api-guidelines.md §4).
	cursors := security.NewCursorCodec(cfg.SecretKey)
	// The automation rules (G-05). One dependency set for the webhook writer's reason: the six use
	// cases are one aggregate's writers, and the rule that decides who may write one is a single
	// rule - including the composition half of it, which reads accounts and memberships to answer
	// whether a writer may delegate to the account a rule would run as.
	//
	// The catalogue is deferred for BulkUpdateWorkItems' reason and it is the same circle: a rule's
	// actions are use cases, so writing one has to consult the registry - and these seven are
	// entries of the registry, so it cannot exist yet.
	// The address an INBOUND_WEBHOOK rule answers on (G-08). Its own hasher, derived from the
	// installation secret under the inbound trigger's purpose label, so a value from this column
	// can never be replayed as a calendar feed token, a personal access token or a page cursor
	// (security.md §5).
	automationInbound := postgres.NewAutomationInboundRepository(
		security.NewInboundTokenHasher(cfg.SecretKey))

	ruleCatalogue := &deferredCatalogue{}
	ruleReader := automationservice.Reader{
		Runs:       postgres.NewAutomationRunRepository(cursors),
		Rules:      postgres.NewAutomationRuleRepository(cursors),
		Authorizer: authorizer, UnitOfWork: unitOfWork,
	}
	ruleWriter := automationservice.Writer{
		Rules:       postgres.NewAutomationRuleRepository(cursors),
		Schedules:   postgres.NewAutomationRuleRepository(cursors),
		Accounts:    accounts,
		Memberships: postgres.NewMembershipRepository(),
		Catalogue:   ruleCatalogue,
		// The one place the expression engine is constructed. A rule's conditions are compiled
		// when it is written, so a mistake reaches its author rather than a log (G-06, ADR-0009).
		Conditions: celexpression.New(),
		// The one schedule engine this installation has (ADR-0008, decision 5 of the 0.5.0
		// backlog). A SCHEDULE rule's next moment is worked out here, so a recurrence this build
		// cannot read is refused to its author rather than failing on a worker (G-08).
		Expander: recurrenceadapter.New(),
		Jobs:     jobs,
		// Seals an HTTP_REQUEST's header secret at the write (E-02, T-21): the rule stores
		// ciphertext or nothing, and the outbound sender opens it for the length of one call.
		Encryptor: encryptor,

		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}
	containers := postgres.NewContainerRepository(cursors)
	items := postgres.NewItemRepository(cursors)
	trash := postgres.NewTrashRepository(cursors)
	// The item history. One repository for both halves of the port - the append every writer makes
	// and the page ListActivity reads (B-11).
	history := postgres.NewActivityRepository(cursors)
	// The reading half of the audit trail, beside the sink rather than part of it: every use case
	// that writes holds the sink, and a sink that could also read would put the whole trail one
	// call away from code that has no business reading it (E-09).
	auditTrail := postgres.NewAuditTrailRepository(cursors)
	// Data subject rights (E-10). One repository over four ports - the cases, the consents, the
	// account states an erasure and a restriction write, and the pseudonyms the audit trail reads
	// at the boundary - because they are one table group and one transaction's worth of work.
	privacyStore := postgres.NewPrivacyRepository(cursors)
	lifecycleStore := postgres.NewLifecycleRepository()
	profiles := postgres.NewCapabilityProfileRepository()
	buckets := postgres.NewBucketRepository()
	labels := postgres.NewLabelRepository()
	// The vocabulary a workspace adds to its entries. Its own repository beside the items rather
	// than a method on them: this is what the keys mean, and `work_item.custom_fields` is what an
	// entry says in it (C-07).
	customFields := postgres.NewCustomFieldRepository()
	// The bookmark shelf (D-07): stored queries, interpreted by nobody on this side of a client.
	savedViews := postgres.NewSavedViewRepository()
	// The subscriptions over those bookmarks (D-08). Its own hasher, derived from the
	// installation secret under the calendar feed's purpose label, so a value from this table can
	// never be replayed as a personal access token or a page cursor (security.md §5).
	calendarFeeds := postgres.NewCalendarFeedRepository(
		security.NewFeedTokenHasher(cfg.SecretKey))
	reminders := postgres.NewReminderRepository()
	recurrences := postgres.NewRecurrenceRepository()
	templates := postgres.NewTemplateRepository(cursors)
	itemLabels := postgres.NewItemLabelRepository()
	itemMembers := postgres.NewItemMemberRepository()
	// The media records, beside the bytes: this stores the rows, the object store the content, and
	// keeping the two apart is what keeps every byte operation outside a transaction (C-06).
	mediaObjects := postgres.NewMediaRepository(cursors)
	// The notification records and the preferences. Two repositories rather than one type with two
	// interfaces, because both need a Find and a Save (C-09).
	notifications := postgres.NewNotificationRepository()
	notificationPreferences := postgres.NewNotificationPreferenceRepository()
	outbox := postgres.NewOutbox(jobs)
	// The jumble (G-10). The writer is shared by every jumble use case; the media half is the
	// same repository the attachments use, so an entry's reference counts with theirs. The intake
	// hashes its tokens under the intake's own purpose label, so a rule's inbound token presented
	// at the jumble door matches nothing.
	jumbleIntake := postgres.NewJumbleIntakeRepository(
		security.NewJumbleIntakeHasher(cfg.SecretKey))
	jumbleWriter := jumbleservice.Writer{
		Entries:    postgres.NewJumbleRepository(cursors),
		Media:      mediaObjects,
		Events:     outbox,
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}
	changes := postgres.NewChangeLog()

	// What every writer of an entry needs in order to leave a step in its history. Held as one
	// value rather than two fields per writer, so that what the history says about a change cannot
	// depend on which use case made it (work.ActivityJournal).
	journal := work.ActivityJournal{Entries: history, IDs: ids}

	// Both directions of completion share one dependency set. The two operations are the same walk in
	// opposite directions, and wiring them separately would be two places to get one of eleven fields
	// wrong (work.CompletionWriter).
	completion := work.CompletionWriter{
		Items: items, Containers: containers, Profiles: profiles, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, Activity: journal, Jobs: jobs,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Moving and reordering share one dependency set, on the reasoning that keeps them one event type: a
	// reorder is a move that keeps its parent (work.PlacementWriter).
	placement := work.PlacementWriter{
		Items: items, Buckets: buckets, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Events: outbox, Changes: changes, Audit: auditSink,
		Activity: journal, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		HLC: hybrid,
	}

	// One engine behind every path that removes for good: a person purging an entry, a person
	// emptying a trash, and the retention job. They differ in what they select and in whether a
	// refusal is an error or a number - not in what removing owes (lifecycle.Purger).
	purger := lifecycle.Purger{
		Trash: trash, Expired: lifecycleStore, Holds: lifecycleStore, Removals: lifecycleStore,
		Events: outbox, Audit: auditSink, Clock: clockadapter.System{}, IDs: ids,
		TombstoneWindow: cfg.Retention.TombstoneWindow, BatchSize: cfg.Retention.BatchSize,
	}

	// The rule model of data-retention.md §2 (E-07). One set for the three use cases, so that the
	// share a newly created rule reports and the share its preview reports come from the same
	// reading - RE-7 is exactly that they agree.
	retentionRules := lifecycle.Rules{
		Conditions: celexpression.New(),
		Rules:      postgres.NewRetentionRuleRepository(),
		Policies:   lifecycleStore,
		Marking:    postgres.NewRetentionMarkingRepository(),
		Holds:      lifecycleStore,
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}

	// The safeguard that outranks every rule above (E-08). One set for the three use cases, so
	// that placing a hold and lifting one cannot disagree about which clock recorded them.
	legalHolds := lifecycle.Holds{
		Holds:      postgres.NewLegalHoldRepository(),
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}

	// Every verb that moves an entry between the archive and the trash shares one dependency set.
	// They are the same walk with a different transition in the middle, and wiring them separately
	// would be one more place for "restoring records what trashing records" to stop being true
	// (work.LifecycleWriter).
	itemLifecycle := work.LifecycleWriter{
		Items: items, Containers: containers, Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: auditSink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid, Queue: jobs,
	}

	// Every verb that changes an existing container shares one dependency set: they read the same
	// container, ask the same permission question, and owe the same four writes
	// (work.ContainerWriter).
	containerWriter := work.ContainerWriter{
		Queue:      jobs,
		Containers: containers, Policies: postgres.AutoAssignPolicyRepository{}, Authorizer: authorizer,
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
		Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both directions of an entry's assignee share one dependency set, for the reason the completion
	// pair does: they are the same write in opposite directions, and the only thing that differs is
	// whether a person arrives or leaves (work.AssignmentWriter). `authorizer` appears twice on
	// purpose - the actor's own permission and the question about the second person are two
	// different questions answered by the same service.
	assignment := work.AssignmentWriter{
		Items: items, Containers: containers, Profiles: profiles, Authorizer: authorizer,
		Visibility: authorizer, Events: outbox, Changes: changes, Audit: auditSink,
		Activity: journal, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		HLC: hybrid,
	}

	// One instance serves both callers: the :auto-assign route, and the create path that reuses
	// the machinery for the policy that applies itself and for an explicit assignee (C-02).
	autoAssign := work.AutoAssignWorkItem{
		Assignment: assignment, Policies: postgres.AutoAssignPolicyRepository{},
		Groups: groups, Random: clockadapter.CryptoRandom{},
		// Art. 18 as a technical state: a person under a restriction of processing is left out of
		// the draw rather than assigned work by machine (E-10, data-protection.md §4).
		Accounts: accounts,
	}

	// Both directions of an entry's due date share one dependency set, for the same reason
	// (work.DueDateWriter). The pair's own routes, the merge patch and the create path all
	// dispatch into this one instance, so a due date means the same thing whichever door it came
	// through (D-01).
	dueDateWriter := work.DueDateWriter{
		Items: items, Containers: containers, Profiles: profiles, Reminders: reminders,
		Jobs:       jobs,
		Authorizer: authorizer,
		Events:     outbox, Changes: changes, Audit: auditSink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both directions of an entry's cover share one dependency set, for the same reason
	// (work.CoverWriter). The media record store is here rather than in the media package's own
	// wiring, because a cover is a reference an item holds: this is where the counter moves
	// (C-06, data-protection.md §5).
	coverWriter := work.CoverWriter{
		Items: items, Containers: containers, Profiles: profiles, Media: mediaObjects,
		Authorizer: authorizer, Events: outbox, Changes: changes, Audit: auditSink,
		Activity: journal, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		HLC: hybrid,
	}

	// The three changes to an existing view share one dependency set (work.SavedViewWriter): the
	// same find, the same visibility, the same ownership question (D-07). `authorizer` appears
	// twice on purpose - the audited permission question and the silent visibility question are
	// two doors into the same service.
	savedViewWriter := work.SavedViewWriter{
		Views: savedViews, Containers: containers, Authorizer: authorizer, Permits: authorizer,
		Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
	}

	// The three feed use cases share one dependency set, and it sits beside the views' because a
	// feed is a read of a view: the visibility rule the minting asks is the one GetSavedView asks
	// (D-08).
	calendarFeedWriter := work.CalendarFeedWriter{
		Feeds: calendarFeeds, Views: savedViews, Containers: containers, Permits: authorizer,
		Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		Entropy: clockadapter.CryptoRandom{},
	}

	// An edit and a deletion of a definition share one dependency set (work.CustomFieldWriter):
	// the same read, the same scope resolution, the same permission question, and only the write
	// at the end differs (C-07).
	customFieldWriter := work.CustomFieldWriter{
		Fields: customFields, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{},
	}

	// Both directions of an entry's attachments share one dependency set (work.ItemAttachmentWriter).
	// The same repository serves both halves of it: MediaRepository stores the links and the
	// records, and the reference counter moves in the same transaction as the link (C-06).
	attachmentWriter := work.ItemAttachmentWriter{
		Items: items, Containers: containers, Profiles: profiles, Attachments: mediaObjects,
		Media: mediaObjects, Authorizer: authorizer, Events: outbox, Changes: changes,
		Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both directions of an entry's member list share one dependency set, for the reason the label
	// pair does: they are the same write in opposite directions, and wiring them separately would
	// be two places to get one of fourteen fields wrong (work.ItemMemberWriter).
	itemMemberWriter := work.ItemMemberWriter{
		Items: items, ItemMembers: itemMembers, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Visibility: authorizer, Events: outbox, Changes: changes,
		Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// The discussion's verbs share one dependency set (work.CommentWriter): the same reads, the
	// same permission question, the same records.
	// `authorizer` appears twice on purpose: the actor's own permission and the
	// author-or-administrator question are two different questions answered by the same service.
	commentWriter := work.CommentWriter{
		Comments: postgres.NewCommentRepository(cursors), Items: items, Containers: containers,
		Profiles: profiles, Authorizer: authorizer, Moderation: authorizer,
		Events: outbox, Changes: changes,
		Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// The reminder's three writes share one dependency set (work.ReminderWriter): the same reads,
	// the same permission question, the same two records. `authorizer` appears twice on purpose -
	// the actor's own permission and the question about whether a named recipient can see the
	// entry are two questions answered by the same service (D-02).
	reminderWriter := work.ReminderWriter{
		Reminders: reminders, Items: items, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Visibility: authorizer, Changes: changes, Audit: auditSink,
		Jobs:       jobs,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// Both directions of a series share one dependency set (work.RecurrenceWriter). The expander
	// is the library ADR-0008 chose, as a port: the writer asks it whether a text is a rule at all
	// before anything is stored, so a broken series is refused where somebody wrote it rather than
	// discovered by the scheduler (D-04).
	recurrenceWriter := work.RecurrenceWriter{
		Recurrences: recurrences, Items: items, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Expander: recurrenceadapter.New(),
		Changes: changes, Audit: auditSink, Activity: journal, Jobs: jobs,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// The copy reaches almost everything an entry has, because that is what it carries: the row,
	// its three sets, the vocabulary of the collection it lands in, and the counter of every file
	// it points at (C-11). It is a value rather than a literal in the registry because the
	// materialisation reuses it: an occurrence is a copy of its template (D-05).
	duplicate := work.DuplicateWorkItem{
		Items: items, ItemLabels: itemLabels, ItemMembers: itemMembers, Labels: labels,
		Buckets: buckets, Fields: customFields, Containers: containers,
		Attachments: mediaObjects, Media: mediaObjects, Profiles: profiles,
		// The authorisation service under three names: the permission, the question about the
		// role the actor holds, and whether a second person can see the destination.
		Authorizer: authorizer, Ownership: authorizer, Visibility: authorizer,
		Events: outbox, Changes: changes,
		Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// The templates' three definition verbs share one dependency set (work.TemplateWriter): the
	// same scope resolution, the same permission question, the same records (D-06).
	templateWriter := work.TemplateWriter{
		Templates: templates, Containers: containers, Profiles: profiles,
		Authorizer: authorizer, Changes: changes, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
	}

	// The bulk performs the other use cases, so it needs the catalogue that is built from its own
	// descriptor. The holder is what breaks that circle: it is passed in now and given the
	// catalogue the moment there is one, a few lines below.
	bulkCatalogue := &deferredCatalogue{}

	// The cases the privacy use cases share.
	privacyCases := privacyservice.Cases{
		Requests: privacyStore, Jobs: jobs, Authorizer: authorizer, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
	}

	// The sign-in flow (H-01). One dependency set for AccessTokenWriter's reason: the rules
	// about one credential pair belong in one place. Argon2id lives behind the port; its
	// construction draws the decoy, and a process that cannot draw randomness must not start.
	passwords, err := crypto.NewPasswords(clockadapter.CryptoRandom{})
	if err != nil {
		return fmt.Errorf("password hasher: %w", err)
	}
	sessionSigner := security.NewSessionTokenIssuer(cfg.SecretKey)
	sessions := postgres.NewSessionRepository()
	signInStore := postgres.NewSignInRepository(
		security.NewRedemptionTokenHasher(cfg.SecretKey),
		security.NewAuthAttemptHasher(cfg.SecretKey))
	sessionWriter := identity.SessionWriter{
		Accounts: signInStore,
		Sessions: sessions,
		Refresh:  postgres.NewRefreshTokenRepository(security.NewSessionRefreshHasher(cfg.SecretKey)),
		Attempts: signInStore, Tenants: signInStore,
		Passwords: passwords, Signer: sessionSigner,
		Audit: auditSink, Signals: metrics,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		Entropy: clockadapter.CryptoRandom{},
		Multi:   cfg.Tenancy == envport.TenancyMulti,
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
		identity.SignIn{Writer: sessionWriter}.Descriptor(),
		identity.RefreshSession{Writer: sessionWriter}.Descriptor(),
		identity.ListSessions{Writer: sessionWriter}.Descriptor(),
		identity.RevokeSession{Writer: sessionWriter}.Descriptor(),
		identity.RevokeAllSessions{Writer: sessionWriter}.Descriptor(),
		identity.CreateAccessToken{Writer: accessTokenWriter}.Descriptor(),
		identity.ListAccessTokens{Writer: accessTokenWriter}.Descriptor(),
		identity.RevokeAccessToken{Writer: accessTokenWriter}.Descriptor(),
		identity.CreateServiceAccount{Accounts: serviceAccounts}.Descriptor(),
		identity.ListServiceAccounts{Accounts: serviceAccounts}.Descriptor(),
		integrationservice.CreateWebhookSubscription{Writer: webhookWriter}.Descriptor(),
		integrationservice.GetWebhookSubscription{Writer: webhookWriter}.Descriptor(),
		integrationservice.ListWebhookSubscriptions{Writer: webhookWriter}.Descriptor(),
		integrationservice.UpdateWebhookSubscription{Writer: webhookWriter}.Descriptor(),
		integrationservice.DeleteWebhookSubscription{Writer: webhookWriter}.Descriptor(),
		integrationservice.ListWebhookDeliveries{Writer: webhookWriter}.Descriptor(),
		integrationservice.ReplayWebhookDelivery{Writer: webhookWriter, Jobs: jobs}.Descriptor(),
		integrationservice.SendWebhook{Writer: webhookWriter, Jobs: jobs, Events: outbox}.Descriptor(),
		integrationservice.RotateWebhookSecret{Writer: webhookWriter}.Descriptor(),
		automationservice.CreateRule{Writer: ruleWriter}.Descriptor(),
		automationservice.GetRule{Writer: ruleWriter}.Descriptor(),
		automationservice.ListRules{Writer: ruleWriter}.Descriptor(),
		automationservice.UpdateRule{Writer: ruleWriter}.Descriptor(),
		automationservice.EnableRule{Writer: ruleWriter}.Descriptor(),
		automationservice.DisableRule{Writer: ruleWriter}.Descriptor(),
		automationservice.DeleteRule{Writer: ruleWriter}.Descriptor(),
		automationservice.TriggerRuleManually{
			Rules: postgres.NewAutomationRuleRepository(cursors),
			Jobs:  jobs, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		automationservice.RotateInboundTrigger{
			Rules:      postgres.NewAutomationRuleRepository(cursors),
			Inbound:    automationInbound,
			Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
		}.Descriptor(),
		automationservice.ListRuleRuns{Reader: ruleReader}.Descriptor(),
		automationservice.GetRuleRun{Reader: ruleReader}.Descriptor(),
		automationservice.HttpRequest{
			Jobs: jobs, Authorizer: authorizer, Encryptor: encryptor,
			Conditions: celexpression.New(), Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		automationservice.TestRule{
			Rules:     postgres.NewAutomationRuleRepository(cursors),
			Catalogue: ruleCatalogue, Conditions: celexpression.New(),
			Entries: items, Containers: containers,
			Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		automationservice.ReplayRuleRun{
			Runs:  postgres.NewAutomationRunRepository(cursors),
			Rules: postgres.NewAutomationRuleRepository(cursors),
			Jobs:  jobs, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		jumbleservice.SubmitJumbleEntry{Writer: jumbleWriter}.Descriptor(),
		jumbleservice.ListJumbleEntries{Writer: jumbleWriter}.Descriptor(),
		jumbleservice.ConvertJumbleEntry{
			Writer: jumbleWriter, Catalogue: ruleCatalogue, Origins: items,
		}.Descriptor(),
		jumbleservice.DismissJumbleEntry{Writer: jumbleWriter}.Descriptor(),
		jumbleservice.RotateJumbleIntake{
			Intake:     jumbleIntake,
			Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, Entropy: clockadapter.CryptoRandom{},
		}.Descriptor(),
		integrationservice.PollTriggerEvents{
			Events: outbox, Policies: lifecycleStore,
			Cursors:   security.NewTriggerCursorCodec(cfg.SecretKey),
			Rendering: cloudEventRendering{source: cfg.BaseURL},
			// The pull half renders through the very function the push half delivers, so that one
			// schema really is two transports rather than two renderings that agree for now.
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
			Lag: cfg.Queue.TriggerPollLag,
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
			Ownership:  authorizer,
			Events:     outbox,
			Changes:    changes,
			Audit:      auditSink,
			Activity:   journal,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
			AutoAssign: autoAssign,
			DueDates:   dueDateWriter,
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
			Activity:   journal,
			UnitOfWork: unitOfWork,
			Clock:      clockadapter.System{},
			IDs:        ids,
			HLC:        hybrid,
			DueDates:   dueDateWriter,
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
		work.AssignWorkItem{Assignment: assignment}.Descriptor(),
		work.UnassignWorkItem{Assignment: assignment}.Descriptor(),
		autoAssign.Descriptor(),
		work.AddComment{Writer: commentWriter}.Descriptor(),
		work.EditComment{Writer: commentWriter}.Descriptor(),
		work.DeleteComment{Writer: commentWriter}.Descriptor(),
		work.AddMember{Writer: itemMemberWriter}.Descriptor(),
		work.RemoveMember{Writer: itemMemberWriter}.Descriptor(),
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
		work.QueryItems{
			Items: items, ItemLabels: itemLabels, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		// The authorisation service under three names, because a search asks the question in three
		// shapes: about a hub the client named, about how far the caller reaches into a collection,
		// and about each row of an unanchored page (SearchItems.reach).
		work.SearchItems{
			Items: items, Containers: containers,
			Authorizer: authorizer, Anchored: authorizer, Reader: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListActivity{
			History: history, Items: items, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListComments{
			Comments: commentWriter.Comments, Items: items, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.CompleteWorkItem{Completion: completion}.Descriptor(),
		work.ReopenWorkItem{Completion: completion}.Descriptor(),

		work.MoveWorkItem{Placement: placement}.Descriptor(),
		work.BulkUpdateWorkItems{
			Catalogue: bulkCatalogue, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{},
		}.Descriptor(),
		duplicate.Descriptor(),
		work.ReorderWorkItem{Placement: placement}.Descriptor(),
		work.ArchiveWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.UnarchiveWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.TrashWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.RestoreWorkItem{Lifecycle: itemLifecycle}.Descriptor(),
		work.ListTrash{Trash: trash, Reader: authorizer, UnitOfWork: unitOfWork}.Descriptor(),
		lifecycle.PurgeWorkItem{
			Items: items, Containers: containers, Purger: purger,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		lifecycle.EmptyTrash{
			Purger: purger, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		lifecycle.CreateRetentionPolicy{Rules: retentionRules}.Descriptor(),
		lifecycle.ListRetentionPolicies{Rules: retentionRules}.Descriptor(),
		lifecycle.PreviewRetentionPolicy{Rules: retentionRules}.Descriptor(),
		lifecycle.PlaceLegalHold{Holds: legalHolds}.Descriptor(),
		lifecycle.ReleaseLegalHold{Holds: legalHolds}.Descriptor(),
		lifecycle.ListLegalHolds{Holds: legalHolds}.Descriptor(),
		auditservice.ListAuditEntries{
			Trail: auditTrail, Authorizer: authorizer, Pseudonyms: privacyStore,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		auditservice.VerifyAuditChain{
			Trail: auditTrail, Chain: auditadapter.Links{}, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		privacyservice.CreateDataSubjectRequest{Cases: privacyCases}.Descriptor(),
		privacyservice.ListDataSubjectRequests{Cases: privacyCases}.Descriptor(),
		privacyservice.UpdateDataSubjectRequest{Cases: privacyCases}.Descriptor(),
		privacyservice.RestrictProcessing{
			Subjects: privacyStore, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		privacyservice.WithdrawConsent{
			Consents: privacyStore, Authorizer: authorizer, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		auditservice.ExportAuditTrail{
			Jobs: jobs, Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		lifecycle.RetainItem{
			Items: items, Containers: containers,
			Marking: postgres.NewRetentionMarkingRepository(), Authorizer: authorizer,
			Audit: auditSink, Changes: changes, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, HLC: hybrid,
		}.Descriptor(),

		mediaservice.RequestMediaUpload{
			Objects: mediaObjects, Transfers: mediaTransfers, Audit: auditSink, Jobs: jobs,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, Config: cfg,
		}.Descriptor(),
		mediaservice.ConfirmMediaUpload{
			Objects: mediaObjects, Store: mediaStore, Guard: mediaGuard, Audit: auditSink,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, Config: cfg,
		}.Descriptor(),
		work.SetCover{Cover: coverWriter}.Descriptor(),
		work.ClearCover{Cover: coverWriter}.Descriptor(),
		work.SetDueDate{Due: dueDateWriter}.Descriptor(),
		work.ClearDueDate{Due: dueDateWriter}.Descriptor(),
		work.CreateReminder{Writer: reminderWriter}.Descriptor(),
		work.ListReminders{
			Reminders: reminders, Items: items, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateReminder{Writer: reminderWriter}.Descriptor(),
		work.DeleteReminder{Writer: reminderWriter}.Descriptor(),
		work.SetRecurrence{Writer: recurrenceWriter}.Descriptor(),
		work.RemoveRecurrence{Writer: recurrenceWriter}.Descriptor(),
		work.SkipOccurrence{Writer: recurrenceWriter}.Descriptor(),
		work.CreateTemplate{Writer: templateWriter}.Descriptor(),
		work.ListTemplates{
			Templates: templates, Containers: containers, Authorizer: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetTemplate{Writer: templateWriter}.Descriptor(),
		work.UpdateTemplate{Writer: templateWriter}.Descriptor(),
		work.DeleteTemplate{Writer: templateWriter}.Descriptor(),
		work.InstantiateTemplate{
			Writer: templateWriter, Items: items, ItemMembers: itemMembers,
			Visibility: authorizer, Events: outbox, Activity: journal,
		}.Descriptor(),
		work.GetRecurrence{
			Recurrences: recurrences, Items: items, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.AttachMedia{Writer: attachmentWriter}.Descriptor(),

		work.DefineCustomField{
			Fields: customFields, Containers: containers, Profiles: profiles,
			Authorizer: authorizer, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		work.ListCustomFields{
			Fields: customFields, Containers: containers, Authorizer: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.UpdateCustomField{Writer: customFieldWriter}.Descriptor(),
		work.DeleteCustomField{Writer: customFieldWriter}.Descriptor(),
		work.CreateSavedView{
			Views: savedViews, Containers: containers, Authorizer: authorizer,
			Audit: auditSink, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		}.Descriptor(),
		work.ListSavedViews{
			Views: savedViews, Containers: containers, Authorizer: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.GetSavedView{
			Views: savedViews, Containers: containers, Permits: authorizer,
			UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ExportView{
			Views: savedViews, Containers: containers, Permits: authorizer,
			Query: work.QueryItems{
				Items: items, ItemLabels: itemLabels, Containers: containers,
				Authorizer: authorizer, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
			},
			ItemLabels: itemLabels, Audit: auditSink, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{},
		}.Descriptor(),
		work.CreateCalendarFeed{Writer: calendarFeedWriter}.Descriptor(),
		work.ListCalendarFeeds{Writer: calendarFeedWriter}.Descriptor(),
		work.RevokeCalendarFeed{Writer: calendarFeedWriter}.Descriptor(),
		work.UpdateSavedView{Writer: savedViewWriter}.Descriptor(),
		work.DeleteSavedView{Writer: savedViewWriter}.Descriptor(),
		work.ShareSavedView{Writer: savedViewWriter}.Descriptor(),
		work.SetCustomField{
			Items: items, Containers: containers, Profiles: profiles, Fields: customFields,
			Authorizer: authorizer, Visibility: authorizer, Events: outbox, Changes: changes,
			Audit: auditSink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
		}.Descriptor(),
		work.DetachMedia{Writer: attachmentWriter}.Descriptor(),

		mediaservice.GetMedia{
			Objects: mediaObjects, Containers: containers, Transfers: mediaTransfers,
			Reader: authorizer, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		mediaservice.DeleteMedia{
			Objects: mediaObjects, Authorizer: authorizer, Audit: auditSink, Jobs: jobs,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		}.Descriptor(),
		mediaservice.ListAttachments{
			Objects: mediaObjects, Items: items, Containers: containers,
			Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),

		backupservice.CreateBackupTarget{Writer: backupWriter}.Descriptor(),
		backupservice.ListBackupTargets{Writer: backupWriter}.Descriptor(),
		backupservice.TestBackupTarget{Writer: backupWriter}.Descriptor(),
		backupservice.StartBackup{Runner: backupRunner}.Descriptor(),
		backupservice.GetBackupRun{Runner: backupRunner}.Descriptor(),
		backupservice.VerifyBackup{Runner: backupRunner}.Descriptor(),
		backupservice.CreateBackupSchedule{Scheduling: backupScheduling}.Descriptor(),
		backupservice.ListBackupsAtTarget{Restorer: backupRestorer}.Descriptor(),
		backupservice.StartRestore{Restorer: backupRestorer}.Descriptor(),
		backupservice.GetRestoreRun{Restorer: backupRestorer}.Descriptor(),

		jobservice.GetJob{
			Jobs: jobRecords, Authorizer: authorizer, UnitOfWork: unitOfWork,
		}.Descriptor(),
		jobservice.CancelJob{
			Jobs: jobRecords, Authorizer: authorizer, Audit: auditSink,
			Clock: clockadapter.System{}, UnitOfWork: unitOfWork,
		}.Descriptor(),
	)
	if err != nil {
		// A use case registered without its audit declaration or its handler stops the process
		// rather than being discovered later by whoever needed the audit entry (ADR-0015).
		return fmt.Errorf("use case catalogue: %w", err)
	}
	// And now the bulk can reach the catalogue it is part of. Every operation it performs therefore
	// goes through the same registry a REST call or an MCP tool call goes through, with the same
	// input check, the same permission check and the same metric (C-11).
	bulkCatalogue.catalogue = useCases
	ruleCatalogue.catalogue = useCases

	var api *http.Server
	if cfg.HasRole(envport.RoleAPI) {
		// The routes come from api/openapi.yaml through the generated registration list; nothing
		// here names a path (ADR-0004). Operations without a use case yet answer 404 - the route
		// exists because the contract declares it, not because it works.
		controller := rest.NewRestController()
		controller.UseCases = useCases
		// The two content routes are not catalogue entries: they take a stream and answer a
		// stream, which is neither what MCP nor what an automation rule could do with them. On an
		// object-storage installation they are never reached - the token this server would have to
		// have minted is one it never mints there (C-06).
		controller.MediaContent = mediaservice.MediaContent{
			Objects: mediaObjects, Store: mediaStore, Guard: mediaGuard,
			UnitOfWork: unitOfWork, Config: cfg,
		}
		controller.MediaTokens = mediaTokens
		controller.Clock = clockadapter.System{}
		// The address a calendar client is handed. Configured rather than taken from the
		// request's Host, so that one caller cannot decide what the next person's client stores.
		controller.BaseURL = cfg.BaseURL
		// The host alone, for reading a tenant subdomain off a multi-mode sign-in (H-01,
		// multi-tenancy.md §3). An unreadable base URL means no subdomain is ever read, which
		// fails closed into "no workspace answers here" rather than into a guess.
		if parsed, urlErr := url.Parse(cfg.BaseURL); urlErr == nil {
			controller.BaseHost = strings.ToLower(parsed.Hostname())
		}
		// The public .ics route. Not a catalogue entry: it answers a credential nobody in this
		// system holds, and every question it asks is asked inwards of the controller (D-08).
		controller.CalendarFeeds = work.ReadCalendarFeed{
			Feeds: calendarFeeds, Accounts: postgres.NewAccountRepository(),
			Export: work.ExportView{
				Views: savedViews, Containers: containers, Permits: authorizer,
				Query: work.QueryItems{
					Items: items, ItemLabels: itemLabels, Containers: containers,
					Authorizer: authorizer, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
				},
				ItemLabels: itemLabels, Audit: auditSink, UnitOfWork: unitOfWork,
				Clock: clockadapter.System{},
			},
			UnitOfWork: unitOfWork,
		}
		// The public inbound-webhook route, for the same reason and with the same discipline: it
		// answers a credential nobody in this system holds, it can do exactly one thing - start
		// that one rule's run - and every question it asks is asked inwards of the controller
		// (G-08, automation.md §1.1).
		controller.InboundRuns = automationservice.StartInboundRun{
			Inbound: automationInbound, Jobs: jobs, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, IDs: ids,
		}
		// The jumble's public door (G-10): a delivery on the tenant's address becomes an entry.
		controller.JumbleIntake = intake.WebhookIntake{
			Deliveries: jumbleservice.IntakeJumbleEntry{
				Intake: jumbleIntake, Entries: postgres.NewJumbleRepository(cursors),
				Events: outbox, UnitOfWork: unitOfWork,
				Clock: clockadapter.System{}, IDs: ids,
			},
		}
		// The mail door beside it (G-11): the message is parsed here, its files go through the
		// media pipeline's server-side end, and what lands is an EMAIL entry.
		controller.MailIntake = intake.MailIntake{
			Deliveries: jumbleservice.IntakeMail{
				Intake: jumbleIntake, Entries: postgres.NewJumbleRepository(cursors),
				Media: mediaservice.IngestMedia{
					Objects: mediaObjects, Store: mediaStore, Guard: mediaGuard, Jobs: jobs,
					UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids, Config: cfg,
				},
				Events: outbox, UnitOfWork: unitOfWork,
				Clock: clockadapter.System{}, IDs: ids,
			},
		}
		// The change stream is not a catalogue entry either: it is a connection being held rather
		// than an operation being invoked, so there is nothing for MCP or an automation rule to
		// call (C-10). The listener is the wake-up; without it the stream still works, at its idle
		// poll interval.
		controller.Stream = &rest.StreamController{
			Stream: syncservice.StreamChanges{
				Changes: changes, Containers: containers, Authorizer: authorizer,
				UnitOfWork: unitOfWork, Cursors: streamCursors,
				Clock: clockadapter.System{},
				// The maximum offline window, which is also the minimum tombstone period: beyond
				// it the log no longer holds everything that happened (offline-sync.md §7).
				Window: cfg.Retention.TombstoneWindow,
				Batch:  cfg.Queue.OutboxBatch,
			},
			Registry: streams,
			Wakeups:  changeListener,
			Signals:  metrics,
		}
		controller.Capabilities = meta.GetCapabilities{
			Profiles:   profiles,
			Languages:  postgres.NewTextLanguageRepository(),
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
			// The session half (H-01): the signature refuses forgeries before any lookup, the
			// row answers whether the session is still alive. A session carries every declared
			// scope, because it is the person rather than a bounded credential.
			Sessions:      sessions,
			Signer:        sessionSigner,
			SessionScopes: catalogue.Scopes(),
		}

		// One limiter, two levels: per credential or client address before authentication, per
		// tenant after it. The first needs no database - it keys on a fingerprint of the
		// presented string - so a flood of invalid tokens costs no lookups.
		limiter := rest.NewRateLimiter()

		// The web interface, when this installation serves one (ADR-0028). It is built from the
		// bundle embedded at link time, so it is the same version as the API by construction -
		// there is no second artefact that could be a release behind.
		var ui http.Handler
		if cfg.UI.Enabled {
			files, fsErr := webui.FS()
			if fsErr != nil {
				return fmt.Errorf("web interface: %w", fsErr)
			}
			handler, uiErr := webui.NewHandler(files, rest.WriteSecurityHeaders)
			if uiErr != nil {
				return fmt.Errorf("web interface: %w", uiErr)
			}
			ui = handler
			if webui.IsPlaceholder(files) {
				// Worth saying once at startup rather than leaving somebody to discover it in a
				// browser: this binary was built without a frontend build, so "/" answers the
				// placeholder. It is the normal state of a `go build` and a defect in an image.
				slog.Warn("no user interface bundle was built into this binary; / serves the placeholder")
			}
		}

		// The chain, from the outside in. Observed stays outermost: a panic anywhere below it
		// still becomes a problem document, and every answer carries a request ID and a metric -
		// including the ones no handler produced. Authentication sits inside the bounds and
		// inside the first limit, so neither an oversized body nor a flood reaches a database
		// lookup.
		//
		// Fallback sits directly beneath it, above everything else, because the interface is
		// static: it needs no actor, no tenant and no idempotency key, and a page load that spent
		// six requests of the anonymous budget would make the first visit the last one. The API
		// keeps every path it owns; the interface gets what is left.
		api = &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: rest.Observed{
				Router: rest.Fallback{
					API:      apiRoutes,
					Reserved: []string{rest.APIBasePath + "/", mcp.Path},
					UI:       ui,
					Serve: rest.Secured{CORS: cfg.CORS, Next: rest.Bounded{
						MaxBodyBytes: cfg.Request.MaxBodyBytes,
						// The mail door's own bound (G-11): a message is not a document, and the
						// route reads one whole.
						MaxMailBytes: cfg.Request.MaxMailBytes,
						Timeout:      cfg.Request.Timeout,
						Next: rest.Limited{
							Limiter: limiter,
							Level:   "credential",
							Bucket: rest.CredentialBucket(
								cfg.RateLimit.AnonymousPerMinute,
								cfg.RateLimit.TokenPerMinute,
								cfg.RateLimit.Burst),
							// The auth bucket (T-02): stricter than the anonymous budget, on the
							// three routes where a credential is guessed rather than presented.
							// In front of the ledger and the Argon2id work, so a fast guesser is
							// shed before either is reached. It applies to those routes and
							// passes everything else through.
							Next: rest.Limited{
								Limiter: limiter,
								Level:   "auth",
								Bucket: rest.AuthBucket(
									cfg.RateLimit.AuthPerMinute, cfg.RateLimit.Burst),
								// The feed's own bucket, in front of the lookup rather than
								// behind it: a subscription polls, and one client polling hard
								// must not shed the calendar of somebody else behind the same
								// address (D-08, T-21).
								Next: rest.Limited{
									Limiter: limiter,
									Level:   "feed",
									Bucket: rest.FeedBucket(
										cfg.RateLimit.TokenPerMinute, cfg.RateLimit.Burst),
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
	// Who is told about what happens. The first subscriber there has ever been: automation,
	// webhooks, the live stream and the search index register beside it as they arrive (C-09).
	notify := notification.RecordNotifications{
		Notifications: notifications, Preferences: notificationPreferences, Accounts: accounts,
		Items: items, ItemMembers: itemMembers, Jobs: jobs,
		Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
	}

	// The automation engine (G-07). The subscriber turns one event into one job per matching rule;
	// the engine that runs a job is the handler below, and the two are separate because a
	// subscriber runs inside the dispatcher's transaction and may not reach the use case registry.
	automationRuns := postgres.NewAutomationRunRepository(cursors)
	matchRules := automationservice.MatchRules{
		Rules: automationRuns, Containers: containers, Jobs: jobs,
		Conditions: celexpression.New(),
		Jumble:     postgres.NewJumbleRepository(cursors),
		Clock:      clockadapter.System{},
	}

	// The relative-date producer (G-08). A second subscriber rather than a branch inside the first,
	// because it answers a different question: MatchRules asks which rules *want* this event, and
	// this one asks what the entry's deadline now means for the rules that measure from it.
	relativeDates := automationservice.RelativeDates{
		Rules: automationRuns, Occurrences: postgres.NewAutomationRuleRepository(cursors),
		Entries: items, Containers: containers, Jobs: jobs,
		Clock: clockadapter.System{}, IDs: ids,
	}

	webhookFanOut := integrationservice.FanOut{
		Subscriptions: postgres.NewWebhookSubscriptionRepository(),
		Deliveries:    postgres.NewWebhookDeliveryRepository(),
		Jobs:          jobs, Clock: clockadapter.System{}, IDs: ids,
	}

	dispatcher := eventbus.Dispatcher{
		Events:   postgres.NewOutbox(jobs),
		Consumed: postgres.NewConsumption(clockadapter.System{}),
		// The webhook fan-out is a subscriber like the notifications: one event in, a delivery job
		// per interested subscription out. It deliberately does not implement TakesReplays, so a
		// restore reaches no external system (backup-restore.md §8.4).
		// The automation engine beside them, and it deliberately does not implement TakesReplays
		// either: no rule fires for a restore's events (backup-restore.md §8.4, BK-5).
		Subscribers: []eventbusport.Subscriber{notify, webhookFanOut, matchRules, relativeDates},
		Clock:       clockadapter.System{},
		Batch:       cfg.Queue.OutboxBatch,
		MinInterval: cfg.Queue.OutboxMinInterval,
		MaxInterval: cfg.Queue.OutboxMaxInterval,
		Lag:         metrics.OutboxLag,
	}

	// The kinds this build knows. The scheduler publishes a zero for each of them, so that the
	// backlog gauge exists before there is a backlog.
	// The retention run, and the queue's way into it. One pass per job execution, because the
	// handler runs inside the transaction the runner opened - the job comes back for the next pass
	// rather than looping inside one transaction (data-retention.md §5).

	// The reclamation of unreferenced files. The one handler that runs outside the runner's
	// transaction (queue.Detached): the pass deletes bytes from a bucket between two writes, and a
	// transaction held open across that call is what observability-reliability.md §8 forbids.
	mediaReconciliation := worker.MediaReconciliation{
		Reconciliation: mediaservice.ReconcileMedia{
			Objects: mediaObjects, Store: mediaStore, Removals: lifecycleStore,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{},
			Config: cfg.Media, Retention: cfg.Retention, Signals: metrics,
		},
		Interval:     cfg.Media.Interval,
		Continuation: cfg.Queue.OutboxMinInterval,
	}

	// The two halves of telling somebody something. The invitation writes a record inside the
	// runner's transaction; the delivery reaches an SMTP server and therefore owns its own
	// (queue.Detached, observability-reliability.md §8).
	invitationMessage := worker.InvitationMessage{
		Invitation: notification.RecordInvitation{
			Notifications: notifications, Accounts: accounts, Jobs: jobs,
			Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
		},
	}
	notificationDelivery := worker.NotificationDelivery{
		Delivery: notification.DeliverNotification{
			Notifications: notifications, Preferences: notificationPreferences,
			Accounts: accounts, Items: items, Mail: mailSender, Renderer: renderer,
			UnitOfWork: backgroundWork, Clock: clockadapter.System{}, BaseURL: cfg.BaseURL,
			Signals: metrics,
		},
	}

	// The firing of what is due. It reads the reminders, writes the records through the same
	// notification path everything else uses, and decides when to come back - which is why it
	// holds its own job row for the pass (D-03).
	reminderFiring := worker.ReminderFiring{
		Firing: work.FireReminders{
			Reminders: reminders, Items: items, Schedule: items, Containers: containers,
			ItemMembers: itemMembers,
			Visibility:  authorizer,
			Notifier: notification.RecordReminder{
				Notifications: notifications, Preferences: notificationPreferences,
				Accounts: accounts, Jobs: jobs,
				Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
			},
			Events: outbox, Changes: changes,
			Clock: clockadapter.System{}, IDs: ids, HLC: hybrid, Signals: metrics,
			BatchSize: work.DefaultReminderBatch,
		},
		Queue: jobs,
		Clock: clockadapter.System{},
		// A batch that filled comes straight back, at the same short wait the other pollers use
		// for the same reason; a wake-up that is due now waits a moment rather than spinning.
		Continuation: cfg.Queue.OutboxMinInterval,
		MinimumWait:  cfg.Queue.OutboxMinInterval,
	}

	// What a series owes. The copy is C-11's duplicate, wired with the same repositories the use
	// case uses: an occurrence is a copy of the template with its subtree, and rebuilding that
	// would be a second answer to what a copy carries (D-05).
	recurrenceMaterialisation := worker.RecurrenceMaterialisation{
		Materialisation: work.MaterializeOccurrences{
			Recurrences: recurrences, Items: items, Containers: containers,
			Copy:     duplicate,
			Expander: recurrenceadapter.New(),
			Events:   outbox,
			Clock:    clockadapter.System{}, IDs: ids, Signals: metrics,
			RuleBatch: work.DefaultRuleBatch, OccurrenceBatch: work.DefaultOccurrenceBatch,
		},
		Queue:        jobs,
		Clock:        clockadapter.System{},
		Continuation: cfg.Queue.OutboxMinInterval,
		MinimumWait:  cfg.Queue.OutboxMinInterval,
	}

	// The backup run and the two jobs around it (E-05). The run is Detached: it holds a
	// REPEATABLE READ snapshot while it streams to somebody else's machine, and doing that inside
	// the runner's own transaction would mean two open at once - the runner's for minutes, on the
	// pool the API shares.
	backupPerformer := backupservice.Performer{
		Runs: backupRuns, Targets: backupTargets,
		Export: postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Opener: backupAdapters, Encryptor: encryptor, Keys: encryptor,
		Cipher: crypto.NewStream(clockadapter.CryptoRandom{}), Objects: mediaStore,
		Snapshot: unitOfWork, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
		SchemaVersion: schemaVersion(), ProductVersion: version,
	}
	// Data subject requests (E-10): the erasure that serves every storage location, and the export
	// that is a Hubtask archive rather than a second format.
	privacyEraser := privacyservice.Eraser{
		Requests: privacyStore, Erasure: privacyStore, Pseudonyms: privacyStore,
		Removals: postgres.NewLifecycleRepository(), Objects: mediaStore, Audit: auditSink,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		TombstoneWindow: cfg.Retention.TombstoneWindow,
	}
	privacyExporter := privacyservice.Exporter{
		Requests: privacyStore, Subjects: privacyStore,
		Targets: backupservice.StoreOpener{
			Targets: backupTargets, Opener: backupAdapters, Encryptor: encryptor,
			UnitOfWork: unitOfWork,
		},
		Rows:    postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
		Objects: mediaStore, Cipher: crypto.NewStream(clockadapter.CryptoRandom{}),
		Audit: auditSink, Snapshot: unitOfWork, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
		SchemaVersion: schemaVersion(), ProductVersion: version,
	}
	privacyPerformer := privacyservice.Performer{
		Requests: privacyStore, Eraser: privacyEraser, Exporter: privacyExporter,
		UnitOfWork: unitOfWork, Clock: clockadapter.System{},
	}

	// The audit export (E-09). It writes to a backup target through the seam that owns a target's
	// credentials, because an export needs somewhere to put bytes and has no business with them.
	auditArchivist := auditservice.Archivist{
		Trail: auditTrail, Pseudonyms: privacyStore,
		Targets: backupservice.StoreOpener{
			Targets: backupTargets, Opener: backupAdapters, Encryptor: encryptor,
			UnitOfWork: unitOfWork,
		},
		Encryptor: encryptor, UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		ProductVersion: version,
	}

	// The restore, and the backup it takes before a destructive mode (E-06). It shares the
	// performer so that the safety copy is the same act a scheduled backup is - a second way of
	// writing an archive would be a second archive format to keep in step.
	backupApplier := backupservice.Applier{
		Restores: backupRestores, Targets: backupTargets,
		Import:  postgres.NewBackupImportRepository(),
		Journal: postgres.NewDeletionJournalRepository(postgres.DefaultJournalBatch),
		Opener:  backupAdapters, Encryptor: encryptor, Keys: encryptor,
		Cipher: crypto.NewStream(clockadapter.CryptoRandom{}), Objects: mediaStore,
		Safety: backupPerformer, UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
		SchemaVersion: schemaVersion(), Batch: backupservice.DefaultRestoreBatch,
	}
	retention := worker.RetentionSweep{
		Retention: lifecycle.RunRetention{
			Policies: lifecycleStore, Runs: lifecycleStore, Purger: purger,
			History: notifications,
			// The outbox's own rows (G-02). ADR-0007's second countermeasure, and until now the
			// one table in this schema that only ever grew.
			Events: postgres.NewDispatchedEvents(),
			// The jumble (G-10). Ninety days from the arrival for what was never converted, which
			// is the kind D-06 predicted and the one place raw inbound text would otherwise sit
			// for ever.
			Inbox: postgres.NewJumbleRepository(cursors),
			Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
			// The rule-driven half (E-07). It shares the purger, so a retention hard delete owes
			// exactly what a person's purge owes: a journal entry, a tombstone and an event per
			// row that goes.
			Rules: postgres.NewRetentionRuleRepository(),
			Sweeper: lifecycle.Sweeper{
				Rules:   postgres.NewRetentionRuleRepository(),
				Marking: postgres.NewRetentionMarkingRepository(),
				Holds:   lifecycleStore, Items: items, Purger: purger, Conditions: celexpression.New(), Changes: changes,
				// The advance warning of data-retention.md §6 (R-1), through the path C-09 built:
				// the preference is honoured, the record is deduplicated, and the send is a job.
				Warnings: notification.RecordRetentionWarning{
					Notifications: notifications, Accounts: accounts,
					Memberships: postgres.NewMembershipRepository(), Members: itemMembers,
					Preferences: notificationPreferences, Jobs: jobs,
					Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
				},
				Export: backupservice.RetentionExport{
					Performer: backupPerformer, IDs: ids,
				},
				Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
				Batch: cfg.Retention.BatchSize,
			},
		},
		Interval:     cfg.Retention.Interval,
		Continuation: cfg.Queue.OutboxMinInterval,
	}

	backupPass := backupservice.SchedulePass{
		Schedules: backupSchedules, Runs: backupRuns, Jobs: jobs,
		Expander: recurrenceadapter.New(), UnitOfWork: unitOfWork,
		Clock: clockadapter.System{}, IDs: ids,
	}
	backupRun := worker.BackupRun{
		Performer: backupPerformer,
		Progress:  jobs,
		Expiry: worker.BackupExpiry{
			Performer: backupPerformer,
			Schedules: backupservice.SchedulePlans{Schedules: backupSchedules, UnitOfWork: unitOfWork},
		},
	}

	// The webhook deliverer (G-03). Detached, because the call to somebody else's server happens
	// between two short transactions rather than inside one long one - holding a database
	// connection for as long as a subscriber's server feels like taking is what
	// observability-reliability.md §8 forbids.
	//
	// Through the guarded client, always: a webhook target is an egress channel exactly as a
	// backup target is, so a private range or the cloud metadata address is refused unless the
	// installation has deliberately released private networks (rule 6, T-07).
	webhookDelivery := webhook.Deliverer{
		Subscriptions: postgres.NewWebhookSubscriptionRepository(),
		Deliveries:    postgres.NewWebhookDeliveryRepository(),
		Events:        postgres.NewOutbox(jobs),
		Outcomes: integrationservice.Outcomes{
			Subscriptions: postgres.NewWebhookSubscriptionRepository(),
			Audit:         auditSink, Clock: clockadapter.System{},
			// The owner is told through the path C-09 built rather than a new channel: the
			// preference is honoured and the send is a job like every other.
			Notifier: notification.RecordWebhookDisabled{
				Notifications: notifications, Accounts: accounts,
				Preferences: notificationPreferences, Jobs: jobs,
				Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
			},
		},
		Encryptor: encryptor, Signer: security.NewWebhookSigner(),
		Client:     outboundClient,
		UnitOfWork: backgroundWork, Jobs: jobs,
		Clock: clockadapter.System{}, IDs: ids,
		Source: cfg.BaseURL,
		NextAttempt: resilience.Backoff{
			// automation.md §3.1's ladder: eight attempts with the backoff reaching a day, which
			// comes to a little over two days of trying before the dead letter.
			Attempts: integrationmodel.MaxDeliveryAttempts,
			Base:     30 * time.Second,
			Max:      24 * time.Hour,
		}.Delay,
	}

	// The outbound call (G-09): an HTTP_REQUEST action's HTTP, detached from every transaction and
	// through the guarded client, with the sealed header secret opened for the length of one call.
	outboundCall := automation.OutboundCall{
		Events:     postgres.NewOutbox(jobs),
		Encryptor:  encryptor,
		Compiler:   celexpression.New(),
		Signer:     security.NewWebhookSigner(),
		Client:     outboundClient,
		UnitOfWork: backgroundWork,
		Clock:      clockadapter.System{},
		Entries:    items,
		Containers: containers,
	}

	// The engine (G-07). It reaches the use case registry as the rule's own account, which is why
	// it is a queue handler rather than a subscriber: a subscriber runs inside the dispatcher's
	// transaction, and an action is a use case.
	automationRun := worker.AutomationRun{
		Engine: automationservice.RunRule{
			Rules:      postgres.NewAutomationRuleRepository(cursors),
			Runs:       automationRuns,
			Failures:   automationRuns,
			Events:     outbox,
			Source:     outbox,
			Dispatcher: dispatchActions{catalogue: ruleCatalogue},
			Scopes:     actionScopes{catalogue: ruleCatalogue},
			Conditions: celexpression.New(),
			Entries:    items,
			Containers: containers,
			Jumble:     postgres.NewJumbleRepository(cursors),
			Guard:      runClaims{store: postgres.NewIdempotencyStore()},
			Owners: notification.RecordRuleDisabled{
				Notifications: notifications, Accounts: accounts,
				Preferences: notificationPreferences, Jobs: jobs,
				Clock: clockadapter.System{}, IDs: ids, Signals: metrics,
			},
			// Where a WAIT parks its resume (G-09): the suspended run and the job that brings it
			// back commit together with the runner's transaction.
			Jobs:       jobs,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		},
		Rules: postgres.NewAutomationRuleRepository(cursors),
	}

	handlers := map[queueport.Kind]queueport.Handler{
		queueport.KindReminderFire:          reminderFiring,
		queueport.KindRecurrenceMaterialize: recurrenceMaterialisation,
		queueport.KindOutboxDispatch:        dispatcher,
		queueport.KindRetentionSweep:        retention,
		queueport.KindMediaReconcile:        mediaReconciliation,
		queueport.KindInvitationEmail:       invitationMessage,
		queueport.KindNotificationDeliver:   notificationDelivery,
		queueport.KindWebhookDeliver:        webhookDelivery,
		queueport.KindAutomationRun:         automationRun,
		queueport.KindAutomationHTTP:        outboundCall,
		queueport.KindBackupRun:             backupRun,
		queueport.KindBackupVerify:          worker.BackupVerify{Performer: backupPerformer},
		queueport.KindBackupRestore: worker.BackupRestore{
			Applier: backupApplier, Progress: jobs,
		},
		queueport.KindBackupSchedule: worker.BackupScheduling{
			Pass: backupPass, Fallback: cfg.Retention.Interval,
		},
		queueport.KindAutomationSchedule: worker.AutomationScheduling{
			Pass: automationservice.SchedulePass{
				Schedules:   postgres.NewAutomationRuleRepository(cursors),
				Occurrences: postgres.NewAutomationRuleRepository(cursors),
				Jobs:        jobs, Expander: recurrenceadapter.New(),
				UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
			},
			Fallback: cfg.Retention.Interval,
		},
		queueport.KindAuditExport:    worker.AuditExport{Archivist: auditArchivist},
		queueport.KindPrivacyRequest: worker.PrivacyRequest{Performer: privacyPerformer},
		queueport.KindPrivacyDeadlines: worker.PrivacyDeadlines{
			Watch: privacyservice.WatchDeadlines{
				Requests: privacyStore, Signals: metrics, UnitOfWork: unitOfWork,
				Clock: clockadapter.System{},
			},
			Queue: jobs, Clock: clockadapter.System{},
		},
	}
	kinds := make([]queueport.Kind, 0, len(handlers))
	for kind := range handlers {
		kinds = append(kinds, kind)
	}

	if cfg.HasRole(envport.RoleAPI) {
		// The wake-up loop, and only where streams are served: a worker holding a LISTEN would be
		// a connection occupied for notifications nobody in that process is waiting for.
		background = append(background, start(ctx, "api.change_listener", changeListener.Run))
	}

	if cfg.HasRole(envport.RoleWorker) {
		// The dispatcher's wake-up, and only where jobs are run: an API process holding a LISTEN
		// for a queue it does not drain would be a connection occupied for notifications nobody
		// in it is waiting for - the mirror of the change listener above.
		//
		// On the background pool rather than the request pool. A held connection out of the pool
		// that serves requests is one fewer for them, and the background pool is where every
		// other long-lived hold already lives (the leader's).
		jobListener := postgres.NewJobListener(backgroundPool)
		background = append(background, start(ctx, "worker.job_listener", jobListener.Run))

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
			// The poll interval stays what it was. This shortens the wait when the notification
			// arrives and changes nothing when it does not (ADR-0007).
			Woken: jobListener.Woken(),
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
			// The leader's one duty beyond measurement, and the reading behind alert A-12 (E-05).
			InstanceBackups: backupPass,
			BackupFreshness: backupRunsInBackground{Runs: backupRuns, Work: backgroundWork},
			// The partition duty `0001_init` wrote down: next month's partition of `audit_log`
			// exists before the first entry of it does, and carries its own policy and its own
			// revokes (E-09, audit.md §3).
			AuditPartitions: auditPartitionsInBackground{
				Partitions: postgres.NewAuditPartitionRepository(), Work: backgroundWork,
			},
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
	//
	// How long that wait has to be is the load balancer's property, not this process's, which is
	// why it is configuration rather than a constant. It was two seconds, and RT-8 found what two
	// seconds buys: an ingress still holding the endpoint, and two requests answered with 502
	// during a rollout that was otherwise clean (docs/evidence/RT-8-2026-08-21.md).
	registry.MarkClosing()

	// The streams are asked to end at the same moment, and before the deregistration wait rather
	// than after it. A stream has no natural end, so Shutdown would otherwise sit out the whole
	// grace period on connections that are working exactly as designed - and the clients get the
	// length of that wait to notice, reconnect elsewhere and resume from their cursors
	// (observability-reliability.md §9).
	streams.CloseAll()

	time.Sleep(time.Duration(cfg.ShutdownDeregisterSeconds) * time.Second)

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
// buildObjectStore composes the configured store (C-05). For S3 that is the whole of A-05's
// vocabulary: the adapter, a breaker the health probe reads, and a bulkhead so the storage pool
// is this size and not the process (ADR-0016).
// The transfer issuer comes back with it, because which one applies is the same decision: the S3
// adapter signs its own presigned URLs, and a local installation has nothing to presign - so this
// server's token-protected content routes take that part instead. The resilient wrapper is
// deliberately not the issuer: signing a URL reaches nothing, and putting it behind a circuit
// breaker would refuse to mint a target because a previous byte transfer failed.
func buildObjectStore(
	cfg envport.Config, tokens security.MediaTokenIssuer,
	registry *healthadapter.Registry, metrics *observability.Metrics,
) (storageport.ObjectStore, storageport.TransferIssuer, error) {
	if cfg.Storage.Kind != envport.StorageS3 {
		return storageadapter.NewLocalStorage(cfg.Storage.LocalPath),
			storageadapter.NewLocalTransfers(tokens, cfg.BaseURL), nil
	}

	s3, err := storageadapter.NewS3Storage(cfg.Storage, cfg.Request.Timeout)
	if err != nil {
		return nil, nil, err
	}
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: "object_storage",
		OnStateChange: func(dependency string, state resilience.BreakerState) {
			metrics.CircuitBreakerState(context.Background(), dependency, state.Level())
		},
	})
	// Eight slots, failing fast: media is the interactive path's optional extra, and a request
	// that cannot get a slot should hear so now rather than queue behind an outage
	// (observability-reliability.md §6).
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "s3", Capacity: 8})

	registry.Register(storageadapter.NewProbe(breaker))
	return storageadapter.NewResilientStore(s3, breaker, bulkhead), s3, nil
}

// buildMailSender assembles the mail port: the SMTP adapter behind a breaker and a bulkhead, and
// the probe that reads the same breaker (C-09).
//
// Always built, even where nothing is configured. An installation with no HUBTASK_SMTP_HOST is not
// an installation without notifications - the records are written either way, and the delivery
// reports the dependency as down so that /meta/health says why nothing is arriving. A nil sender
// would be a nil check in the delivery path instead.
func buildMailSender(
	cfg envport.Config, registry *healthadapter.Registry, metrics *observability.Metrics,
) mailport.Sender {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Dependency: mailadapter.Dependency,
		OnStateChange: func(dependency string, state resilience.BreakerState) {
			metrics.CircuitBreakerState(context.Background(), dependency, state.Level())
		},
	})
	// Four slots. Notifications are never on the interactive path - every send is a job - so the
	// bulkhead is here to bound how much of the worker one slow mail server can hold rather than
	// to keep a request fast.
	bulkhead := resilience.NewBulkhead(resilience.BulkheadConfig{
		Name: mailadapter.Dependency, Capacity: 4,
	})

	registry.Register(mailadapter.NewProbe(breaker, cfg.Mail.Host != ""))
	return mailadapter.NewResilientSender(mailadapter.NewSMTP(cfg.Mail), breaker, bulkhead)
}

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

// streamCursorAdapter bridges the stream's cursor port to the keyed codec in
// infrastructure/security.
//
// Two lines of translation rather than one type implementing both sides, because the two are
// deliberately different shapes: the application says `sync.Position` and knows nothing about
// HMACs, and the adapter says `security.StreamPosition` and knows nothing about change logs. The
// alternative is one of them importing the other, and the one that would have to give is the core.
type streamCursorAdapter struct{ codec security.StreamCursorCodec }

func (a streamCursorAdapter) Encode(position syncservice.Position) string {
	return a.codec.Encode(security.StreamPosition{
		Seq: position.Seq, IssuedAt: position.IssuedAt,
	})
}

func (a streamCursorAdapter) Decode(cursor string) (syncservice.Position, error) {
	decoded, err := a.codec.Decode(cursor)
	if err != nil {
		return syncservice.Position{}, err
	}
	return syncservice.Position{Seq: decoded.Seq, IssuedAt: decoded.IssuedAt}, nil
}

// dispatchActions and actionScopes bridge the engine to the use case registry (G-07).
//
// Two small translations rather than the engine naming the adapter's types: the dispatcher lives in
// infrastructure and the application layer may not import one (ADR-0001). What crosses is a kind and
// a document, which is what the rule stored.
type dispatchActions struct{ catalogue *deferredCatalogue }

func (d dispatchActions) Dispatch(
	ctx context.Context, runAs appshared.ActorContext, kind string,
	params map[string]any, supplied map[string]any,
) (usecase.Output, error) {
	return automation.NewActionDispatcher(d.catalogue).
		Dispatch(ctx, runAs, automation.Action{Kind: kind, Params: params}, supplied)
}

// actionScopes answers which token scope an action's use case declares, which is the one the engine
// grants a run. See automationservice.Scopes for why a rule is granted a scope rather than narrowed
// by one.
type actionScopes struct{ catalogue *deferredCatalogue }

func (a actionScopes) ForAction(kind string) (string, bool) {
	descriptor, found := a.catalogue.ByAutomationAction(kind)
	if !found {
		return "", false
	}
	return descriptor.TokenScope, true
}

// runClaims is the engine's half of the idempotency store: reserve a key, and say whether this
// attempt is the first.
//
// It uses the store directly rather than the Guard, because the Guard's shape is the REST path's -
// a request hash to compare and an answer to replay. An action has neither: what it needs is the
// reservation, and the answer it would replay is the effect the first attempt already had.
type runClaims struct{ store postgres.IdempotencyStore }

func (c runClaims) Claim(
	ctx context.Context, _ appshared.ActorContext, key string,
) (bool, error) {
	_, reserved, err := c.store.Reserve(ctx,
		// A colon rather than a dot: the endpoint is a scope for the key, not a message code, and
		// the message-code gate reads anything shaped like one as a promise to translate.
		idempotencyrepo.Key{Key: key, Endpoint: "automation:run"}, []byte(key))
	return reserved, err
}

// Release lets a failed action's claim go, so a replay can perform what the first run never did
// (G-09). See the engine's Idempotency port for why a failed claim must not outlive its failure.
func (c runClaims) Release(ctx context.Context, _ appshared.ActorContext, key string) error {
	return c.store.Release(ctx, idempotencyrepo.Key{Key: key, Endpoint: "automation:run"})
}

// cloudEventRendering bridges the polling trigger's rendering port to the CloudEvents mapping the
// webhook deliverer already sends (G-04).
//
// The point is that there is one function. The pull half and the push half are two transports over
// one contract, and the way to keep that true is for both to call ToCloudEvent rather than for each
// to build a document that happens to match. The source is the installation's own identifier, the
// same value a delivery carries, so a consumer receiving from two installations tells them apart
// whichever way the event reached it.
type cloudEventRendering struct{ source string }

func (r cloudEventRendering) Render(envelope event.Envelope) map[string]any {
	return eventbus.ToCloudEvent(envelope, r.source)
}

// deferredCatalogue hands the bulk use case the catalogue it is itself an entry of.
//
// The circle is real rather than accidental: `BulkUpdateWorkItems` performs the other use cases, so
// it needs the registry - and the registry is built from its descriptor, so it cannot be handed one
// that does not exist yet. This holder is passed in at construction and filled the moment the
// registry is built, a few lines later and in the same function, so there is no window in which a
// request could find it empty.
//
// It lives in the composition root rather than in the application layer because that is what it is:
// wiring. The use case declares a narrow port (`work.Catalogue`) and knows nothing about how the
// thing behind it came to exist.
type deferredCatalogue struct{ catalogue *usecase.Registry }

func (d *deferredCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if d.catalogue == nil {
		// Unreachable: the holder is filled before the server accepts a request. A named internal
		// error rather than a nil dereference, because fail closed is the rule (ADR-0015).
		return nil, shared.ErrInternal.WithDetail("usecase.catalogue_unavailable")
	}
	return d.catalogue.Invoke(ctx, name, actor, in)
}

// ByAutomationAction is the same circle from the other side: the rule writer validates an action
// against the catalogue, and is itself an entry of it (G-05).
//
// A rule with no catalogue answers "no such action" for every kind, which is the fail-closed
// direction: unreachable, because the holder is filled before the server accepts a request, and if
// it ever were reached it would refuse rules rather than accept unvalidated ones.
// All is the third face of the same holder, and the last one the dispatcher needs. Empty before the
// registry exists, which is the fail-closed direction: an engine that ran then would find no action
// rather than an unvalidated one.
func (d *deferredCatalogue) All() []usecase.Descriptor {
	if d.catalogue == nil {
		return nil
	}
	return d.catalogue.All()
}

func (d *deferredCatalogue) ByAutomationAction(kind string) (usecase.Descriptor, bool) {
	if d.catalogue == nil {
		return usecase.Descriptor{}, false
	}
	return d.catalogue.ByAutomationAction(kind)
}

// masterKeys is the configured keyring as the envelope adapter takes it. A translation of two
// field names rather than a shared type, because the environment port and the cipher adapter have
// no business knowing each other (E-02).
func masterKeys(cfg envport.Config) []crypto.KeyMaterial {
	keys := make([]crypto.KeyMaterial, 0, len(cfg.Encryption.Keys))
	for _, key := range cfg.Encryption.Keys {
		keys = append(keys, crypto.KeyMaterial{ID: key.ID, Material: key.Material})
	}
	return keys
}

// schemaVersion is the migration this build was compiled against, and it goes into every archive's
// manifest: a restore compares it with its own and runs the migrations in between (E-04, §3).
//
// Read from the embedded migrations rather than from the database, and deliberately: what an
// archive has to record is the shape of the data this build writes, and a build knows that about
// itself. Asking the database would answer what it has been migrated to, which is the same number
// on every day except the one that matters.
func schemaVersion() string {
	entries, err := dbfiles.Migrations.ReadDir("migrations")
	if err != nil {
		return ""
	}
	latest := ""
	for _, entry := range entries {
		if number, _, found := strings.Cut(entry.Name(), "_"); found && number > latest {
			latest = number
		}
	}
	return latest
}

// backupRunsInBackground reads the backup freshness on the background pool, which is where every
// leader duty runs: the API's pool is for requests.
// auditPartitionsInBackground gives the partition duty the transaction it needs. A system scope,
// because a partition belongs to the installation rather than to a tenant - and a write one,
// because the duty creates a table when there is one to create.
type auditPartitionsInBackground struct {
	Partitions auditrepo.Partitions
	Work       persistenceport.UnitOfWork
}

func (a auditPartitionsInBackground) Ensure(ctx context.Context, month time.Time) (string, error) {
	var name string
	err := a.Work.Within(ctx, persistenceport.SystemScope(), func(ctx context.Context) error {
		var err error
		name, err = a.Partitions.Ensure(ctx, month)
		return err
	})
	return name, err
}

type backupRunsInBackground struct {
	Runs backuprepo.Runs
	Work persistenceport.UnitOfWork
}

func (b backupRunsInBackground) LastSuccessPerTarget(
	ctx context.Context,
) (map[shared.ID]time.Time, error) {
	var moments map[shared.ID]time.Time
	err := b.Work.WithinReadOnly(ctx, persistenceport.SystemScope(), func(ctx context.Context) error {
		var err error
		moments, err = b.Runs.LastSuccessPerTarget(ctx)
		return err
	})
	return moments, err
}
