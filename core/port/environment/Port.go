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

	Database  DatabaseConfig
	Storage   StorageConfig
	Mail      MailConfig
	RateLimit RateLimitConfig
	Request   RequestConfig
	Outbound  OutboundConfig
	Locale    LocaleConfig
	Metrics   MetricsConfig
	Tracing   TracingConfig

	SecretKey secret.Secret

	// ShutdownGraceSeconds is the deadline for in-flight requests after SIGTERM.
	ShutdownGraceSeconds int
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
	// Timeout is the server-side deadline every handler inherits. No call without a deadline
	// (ADR-0016).
	Timeout time.Duration
}

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
