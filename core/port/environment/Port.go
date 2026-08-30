// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package environment is the port for the entire configuration.
//
// The core never reads from os.Getenv itself. All values come from HUBTASK_* variables
// (12-factor) and are loaded and validated in the adapter. If a required secret is missing,
// the process does not start - fail closed rather than a silent default (ADR-0015).
//
// Every variable has a safe default for self-hosting (arc42 §7.4). The exceptions are the two
// secrets: a generated default would be worse than a startup error, because it would render all
// data unreadable after a restart.
package environment

import (
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Role denotes a process role. One image, several roles (ADR-0014).
type Role string

const (
	RoleAPI        Role = "api"
	RoleWorker     Role = "worker"
	RoleScheduler  Role = "scheduler"
	RoleAutomation Role = "automation"
)

// TenancyMode distinguishes self-hosting from provider operation (ADR-0010).
type TenancyMode string

const (
	TenancySingle TenancyMode = "single"
	TenancyMulti  TenancyMode = "multi"
)

// StorageKind selects the object storage adapter. Local storage is the self-hosting default;
// a Raspberry Pi has a disk, not a bucket.
type StorageKind string

const (
	StorageLocal StorageKind = "local"
	StorageS3    StorageKind = "s3"
)

// MailSecurity is how the SMTP connection is protected.
type MailSecurity string

const (
	MailSecurityStartTLS MailSecurity = "starttls"
	MailSecurityTLS      MailSecurity = "tls"
	MailSecurityNone     MailSecurity = "none" // only sensible for a relay on localhost
)

// Config is the complete configuration state of the process.
type Config struct {
	Version   string
	Commit    string
	Roles     []Role
	HTTPAddr  string
	OpsAddr   string
	BaseURL   string
	LogFormat string
	LogLevel  string

	Tenancy TenancyMode

	Database   DatabaseConfig
	Storage    StorageConfig
	Mail       MailConfig
	RateLimit  RateLimitConfig
	Request    RequestConfig
	CORS       CORSConfig
	Outbound   OutboundConfig
	Queue      QueueConfig
	Retention  RetentionConfig
	Encryption EncryptionConfig
	Backup     BackupConfig
	Media      MediaConfig
	Locale     LocaleConfig
	Metrics    MetricsConfig
	Tracing    TracingConfig
	UI         UIConfig

	SecretKey secret.Secret

	// ShutdownGraceSeconds is the deadline for in-flight requests after SIGTERM.
	ShutdownGraceSeconds int

	// ShutdownDeregisterSeconds is how long the process keeps serving after it has marked itself
	// not ready, before it stops accepting connections.
	//
	// It exists because removing a pod from a load balancer is not synchronous with stopping it.
	// Kubernetes sends SIGTERM and updates the endpoint list at the same time, and whatever routes
	// traffic learns about the second one a moment later - so a process that closes its listener
	// the instant it is asked to stop is still being sent requests it can no longer answer. The
	// client sees them as 502.
	//
	// The value is therefore a property of what sits in front of the process, not of the process:
	// zero is right where nothing does.
	ShutdownDeregisterSeconds int
}

// MaxPoolConns bounds the configured pool size. Not a tuning limit but a safety one: the value
// reaches the driver as an int32, and PostgreSQL's own max_connections is three digits on any
// ordinary installation. A four-digit pool per process is a typo, not a plan.
const MaxPoolConns = 1000

// DatabaseConfig is the connection pool. PostgreSQL is the only mandatory dependency (ADR-0003);
// everything else may fail without stopping the write path.
type DatabaseConfig struct {
	DSN secret.Secret
	// MaxConns is per process, not per cluster. Several roles in one image means several pools,
	// so the sum is what reaches PostgreSQL.
	MaxConns int
	MinConns int
	// MaxConnLifetime bounds how long a connection is reused, so that a failover reaches the
	// pool instead of pinning it to a former primary.
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
	// StatementTimeout is short on the interactive path and longer for background work
	// (engineering-guidelines.md §4) - the protection against a runaway query.
	StatementTimeout       time.Duration
	WorkerStatementTimeout time.Duration
}

// BackupConfig is the operator's say over where backups may go (backup-restore.md §2, E-03).
type BackupConfig struct {
	// LocalRoot is the volume a `local` target writes inside. A target's own path is relative to
	// it and cannot leave it, which is what keeps "write my backups to /etc" out of reach of
	// somebody who administers the instance but not the machine. Empty means this installation
	// serves no local targets.
	LocalRoot string
	// TenantTargets lets a tenant configure its own target in provider operation. Off by default
	// and named in backup-restore.md §2 as the switch it is: a backup target is an egress
	// channel, and one a tenant chose is an egress channel the operator did not.
	//
	// It has no meaning in single-tenant operation, where the tenant's owner *is* the instance
	// administrator and there is nobody else it could be protecting.
	TenantTargets bool
}

// EncryptionKey is one master key as the environment gives it: an identifier that is stored in
// every value sealed under it, and the material that is stored nowhere.
type EncryptionKey struct {
	ID       string
	Material secret.Secret
}

// EncryptionConfig is the installation's master keys, current first (E-02, security.md §3).
//
// A list rather than one key, because rotation without a readable predecessor is not rotation: a
// single key that could be changed would make every value sealed under the old one unreadable at
// the moment it changed. The order is the configuration's statement of which key is current, so
// there is no second setting for it to disagree with.
//
// It may be empty. An installation that encrypts nothing needs no key, and one that is asked to
// seal something without one refuses rather than writing a plaintext.
type EncryptionConfig struct {
	Keys []EncryptionKey
}

// ActiveKeyID is the key new values are sealed under, empty when none is configured.
func (c EncryptionConfig) ActiveKeyID() string {
	if len(c.Keys) == 0 {
		return ""
	}
	return c.Keys[0].ID
}

// RetentionConfig is the lifecycle context's operational surface: how a deletion run is paced, and
// how long the marker of a removal has to outlive it (ADR-0020, data-retention.md §5).
//
// Data rather than code, like the periods themselves - but unlike them, these are the operator's
// rather than the tenant's: a batch size is about the database this installation runs on, and the
// offline window is about the devices its people carry.
type RetentionConfig struct {
	// TombstoneWindow is the maximum offline window (offline-sync.md §7). Two things at once: how
	// long a removal's marker survives it, and the lower bound an automatic run observes before
	// removing at all. A device that has not checked in for longer than this has to resynchronise
	// from scratch, so the marker has done its work by then.
	TombstoneWindow time.Duration
	// BatchSize is how many rows one pass of a deletion reads. Batches so that a large deletion does
	// not hold one transaction open across the whole of it, and so that it can be stopped between
	// passes rather than only by killing it.
	BatchSize int
	// Interval is how long a tenant's sweep waits after a pass that reached the end of its trash.
	// It is what a quiet installation pays for having the machinery at all: nothing expires within
	// an hour of anything else, so an hour is often enough and cheap.
	Interval time.Duration
}

// MediaConfig is what the reconciliation of uploaded files runs on (C-06, data-protection.md §5).
//
// The operator's rather than the tenant's, exactly as RetentionConfig is: how long an abandoned
// upload is given before it counts as abandoned is about the lines this installation's people are
// on, not about what any one workspace keeps.
type MediaConfig struct {
	// StagingGrace is how long a staged upload may stay unconfirmed before the reconciliation
	// treats it as abandoned. It has to outlast the upload window comfortably - a client that got
	// its target and is still pushing 64 MiB up a slow line has not abandoned anything - and the
	// only cost of it being generous is a row and its bytes sitting unreferenced for that long.
	StagingGrace time.Duration
	// UnreferencedGrace is how long a confirmed object may point at nothing before the
	// reconciliation calls it an orphan. Never zero: an object is unreferenced between its
	// confirmation and the first thing that uses it, and again between a detachment and the next
	// attachment, and a pass landing in either window would mark a file somebody is in the middle
	// of using. Marking is where the loss begins rather than where it is decided - every read path
	// refuses a marked object, so nothing can attach it back.
	UnreferencedGrace time.Duration
	// OrphanGrace is how long a marked object waits before its bytes go. The window in which a
	// mistaken removal is still recoverable by hand: the row says what it was and the bytes are
	// still in the bucket.
	OrphanGrace time.Duration
	// BatchSize is how many orphans one pass reclaims. Batched for the reason a retention pass is:
	// each object costs a call to a bucket, and a pass that took them all would be a pass nobody
	// can stop.
	BatchSize int
	// Interval is how long a tenant's reconciliation waits after a pass that found nothing left to
	// do. What a quiet installation pays for having the machinery at all.
	Interval time.Duration
}

// QueueConfig is the background work of the worker and scheduler roles (ADR-0008).
//
// The defaults are the self-hosting ones: a single process running every role, where the queue
// should be quiet enough not to be noticed. A provider deployment with its own worker pods raises
// the batch and lowers the poll interval, which are the two knobs that matter.
type QueueConfig struct {
	// PollInterval is the wait after a round that found nothing. It bounds how long a job that
	// was scheduled without a wake-up has to wait, so it is also the floor under a reminder's
	// punctuality (SLO-5).
	PollInterval time.Duration
	// BatchSize is how many jobs one round claims.
	BatchSize int
	// JobTimeout bounds one job. Every handler inherits it as a context deadline - no call
	// without one (ADR-0016).
	JobTimeout time.Duration
	// MaxAttempts is how often a job is tried before it goes to the dead letter.
	MaxAttempts int
	// RetryBase and RetryMax are the exponential backoff between attempts, with full jitter.
	RetryBase time.Duration
	RetryMax  time.Duration
	// SchedulerTick is how often the scheduler leader looks at the clock, and therefore also how
	// quickly a standby notices that the leader is gone.
	SchedulerTick time.Duration
	// OutboxBatch is how many events one dispatch round delivers.
	OutboxBatch int
	// OutboxMinInterval and OutboxMaxInterval are the adaptive poll of the dispatcher: the first
	// after a round that delivered something, the second for a tenant that had nothing. The
	// maximum is the worst case for SLO-4, so it stays well under thirty seconds.
	OutboxMinInterval time.Duration
	OutboxMaxInterval time.Duration
	// TriggerPollLag is how far behind the present the polling trigger reads (G-04,
	// automation.md §3.2).
	//
	// The endpoint pages the outbox in `(occurred_at, id)` order, and `occurred_at` is stamped by
	// the writing transaction rather than by its commit. A transaction that began before one
	// already answered can therefore still commit a row sorting behind the cursor - and a poller
	// that had moved past it would step over the event and have no way to know. Rows younger than
	// this are withheld from the page and from the cursor together, so that by the time an event
	// is answered, nothing can still arrive in front of it.
	//
	// It has to exceed the longest transaction that appends an event. This installation bounds a
	// statement rather than a transaction, so the closest documented bound is the worker's
	// statement timeout, and the default is that. An operator who knows their writes are short can
	// lower it and get a fresher trigger; one who has raised the worker's budget should raise this
	// with it.
	TriggerPollLag time.Duration
}

// LeaseMargin is how much longer a claim holds than the job it covers.
//
// Derived rather than configured, because the two values only make sense together: a lease that
// expires while its job is still running is a job two workers are doing, and that is not a
// trade-off an operator should be able to make by setting one variable and not the other.
const LeaseMargin = 30 * time.Second

// Lease is how long a claimed job stays claimed.
func (q QueueConfig) Lease() time.Duration { return q.JobTimeout + LeaseMargin }

// StorageConfig is the object storage for media. Optional: without it only media is restricted,
// nothing else (ADR-0016).
type StorageConfig struct {
	Kind StorageKind
	// LocalPath is used for StorageLocal.
	LocalPath string
	// The remaining fields apply to StorageS3, including S3-compatible services (MinIO, Garage).
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    secret.Secret
	SecretKey    secret.Secret
	UsePathStyle bool
}

// MailConfig is outbound SMTP. Optional; without it notifications are only caught up later.
type MailConfig struct {
	Host     string
	Port     int
	Username string
	Password secret.Secret
	From     string
	Security MailSecurity
	Timeout  time.Duration
}

// RateLimitConfig holds the levels from security.md §9: per IP for anonymous traffic, per token,
// per tenant, and a stricter limit for the paths worth attacking.
type RateLimitConfig struct {
	AnonymousPerMinute int
	TokenPerMinute     int
	TenantPerMinute    int
	// AuthPerMinute covers login, password reset, and invitation - the endpoints where guessing
	// pays off.
	AuthPerMinute int
	// Burst is how much of the budget may be spent at once.
	Burst int
}

// RequestConfig bounds a single request (security.md §9, threat T-17).
type RequestConfig struct {
	MaxBodyBytes   int64
	MaxUploadBytes int64
	// MaxMailBytes is the limit for the mail intake, whose body is a whole message rather than a
	// document (G-11). Its own bound because it is its own shape: a mail with two attachments is
	// far past what any JSON endpoint here ever carries, and far below what an upload may be -
	// bounding it by either would make the route useless or make it a way to store files.
	MaxMailBytes int64
	// Timeout is the server-side deadline every handler inherits. No call without a deadline
	// (ADR-0016).
	Timeout time.Duration
}

// CORSConfig is the browser side of the API. Empty by default: a self-hosted installation serves
// its own frontend from its own origin and needs no cross-origin access at all, and an allowlist
// that starts empty is one nobody has to remember to close (security.md §9).
type CORSConfig struct {
	// AllowedOrigins are complete origins (scheme, host, optional port), compared exactly. A
	// single "*" allows every origin - permitted, because a read-only public API is a legitimate
	// self-hosting choice, and safe here because this API never answers with credentials.
	AllowedOrigins []string
	// MaxAge is how long a browser may cache the preflight answer.
	MaxAge time.Duration
}

// AllowsAnyOrigin reports the wildcard case.
func (c CORSConfig) AllowsAnyOrigin() bool {
	return len(c.AllowedOrigins) == 1 && c.AllowedOrigins[0] == CORSWildcard
}

// CORSWildcard is the one value that stands for every origin.
const CORSWildcard = "*"

// MaxOutboundRedirects bounds the configured redirect budget. A chain longer than this is not a
// site that moved, it is a chain being used to walk a request somewhere it may not go (T-07).
const MaxOutboundRedirects = 10

// OutboundConfig bounds every call Hubtask makes to the outside world: webhooks, automation
// actions, AI providers, OIDC discovery (security.md §T-07). The values reach
// infrastructure/httpclient.GuardedClient, which is the only way out of the process.
type OutboundConfig struct {
	// Timeout bounds one outbound call end to end, redirects included.
	Timeout time.Duration
	// ConnectTimeout bounds the connection attempt on its own, so that a target that accepts
	// nothing fails fast instead of consuming the whole budget.
	ConnectTimeout time.Duration
	// MaxResponseBytes caps what is read from a response. Without it, a hostile or broken
	// target can hand back a stream until the process runs out of memory (T-17).
	MaxResponseBytes int64
	// MaxRedirects is how many hops are followed. Every hop is checked again from scratch -
	// a redirect to a private address is the classic way around an allowlist.
	MaxRedirects int
	// AllowedHosts is the egress allowlist. Empty means "every public address"; in provider
	// operation an allowlist is mandatory (T-07), which is why an empty one raises a warning
	// there.
	AllowedHosts []string
	// AllowPrivateNetworks opens outbound calls to RFC 1918, loopback, and link-local
	// addresses. Off by default and off in provider operation: it is the switch that turns a
	// webhook into a port scanner of the host network. A self-hoster with an internal target
	// on the same LAN is the one legitimate reason to turn it on, and it warns when set.
	AllowPrivateNetworks bool
}

// MetricsConfig covers what a metric may say about a tenant.
type MetricsConfig struct {
	// TenantLabel adds tenant_id to the per-use-case series. Off by default: in provider
	// operation it multiplies every series by the number of tenants
	// (observability-reliability.md §3.2).
	TenantLabel bool
}

// TracingConfig is the trace exporter. Off by default - a self-hosted installation on a
// Raspberry Pi has no collector to send to, and traces that go nowhere still cost.
type TracingConfig struct {
	Enabled  bool
	Endpoint string
	// SampleRatio is the share of ordinary traces kept, between 0 and 1. Errors and slow
	// requests are kept regardless (§3.3).
	SampleRatio float64
}

// UIConfig is the embedded web interface (ADR-0028).
type UIConfig struct {
	// Enabled serves the built application at "/". Default true: a self-hoster who starts the
	// image expects to be able to open it.
	//
	// False is for an installation that is an API and nothing else - somebody else's frontend in
	// front of it, or a deployment that only serves integrations. Then "/" is 404 and the API is
	// untouched, which is the honest answer: there is nothing there.
	Enabled bool
}

// LocaleConfig is the installation-wide fallback, the last link in the resolution chain
// request → account → tenant → installation (i18n-l10n.md §2).
type LocaleConfig struct {
	// DefaultLocale is BCP 47: de, de-AT, pt-BR, zh-Hans.
	DefaultLocale string
	// DefaultTimeZone is an IANA name (Europe/Berlin), never a fixed UTC offset - an offset
	// cannot represent daylight saving.
	DefaultTimeZone string
}

func (c Config) HasRole(r Role) bool {
	for _, have := range c.Roles {
		if have == r {
			return true
		}
	}
	return false
}

// StatementTimeoutFor returns the query budget of a role. The interactive path gets the short
// one; anything running in the background gets the long one.
func (c Config) StatementTimeoutFor(r Role) time.Duration {
	if r == RoleAPI {
		return c.Database.StatementTimeout
	}
	return c.Database.WorkerStatementTimeout
}

// Port loads and validates the configuration.
type Port interface {
	Load() (Config, error)
	// Warnings reports states that are not errors but that the operator is missing -
	// a missing backup configuration, for example. They appear in /meta/health (ADR-0016).
	Warnings(Config) []Warning
}

// Warning is machine readable: a code instead of free text, so that clients can translate.
type Warning struct {
	Code     string
	Severity string // info | warn | critical
	Params   map[string]string
}
