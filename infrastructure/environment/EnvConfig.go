// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package environment is the adapter for the configuration port: it reads HUBTASK_* variables.
//
// A configuration error is a typed error with a message code, not a sentence (ADR-0011). The
// operator still gets something readable, because the code and its parameters name the variable -
// but a UI can translate it, and a test can assert on it.
package environment

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	// The time zone database travels in the binary, so the distroless image needs no system
	// tzdata and LoadLocation works there (i18n-l10n.md §2).
	_ "time/tzdata"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// minSecretKeyLength is the lower bound for the master key. Shorter than this and envelope
// encryption is not worth the name (security.md §8).
const minSecretKeyLength = 32

type EnvConfig struct {
	version string
	commit  string
}

func New(version, commit string) *EnvConfig { return &EnvConfig{version: version, commit: commit} }

func (e *EnvConfig) Load() (env.Config, error) {
	cfg := env.Config{
		Version:              e.version,
		Commit:               e.commit,
		HTTPAddr:             get("HUBTASK_HTTP_ADDR", ":8080"),
		OpsAddr:              get("HUBTASK_OPS_ADDR", ":9090"),
		BaseURL:              get("HUBTASK_BASE_URL", ""),
		LogFormat:            get("HUBTASK_LOG_FORMAT", "json"),
		LogLevel:             get("HUBTASK_LOG_LEVEL", "info"),
		Tenancy:              env.TenancyMode(get("HUBTASK_TENANCY_MODE", "single")),
		ShutdownGraceSeconds: getInt("HUBTASK_SHUTDOWN_GRACE_SECONDS", 30),
		// Longer than it usually needs to be, because the cost of being wrong is asymmetric: a
		// few seconds of a slower shutdown against requests refused during every rollout (RT-8,
		// docs/evidence/RT-8-2026-08-21.md).
		ShutdownDeregisterSeconds: getInt("HUBTASK_SHUTDOWN_DEREGISTER_SECONDS", 15),

		Database: env.DatabaseConfig{
			MaxConns:               getInt("HUBTASK_DB_MAX_CONNS", 10),
			MinConns:               getInt("HUBTASK_DB_MIN_CONNS", 2),
			MaxConnLifetime:        getDuration("HUBTASK_DB_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime:        getDuration("HUBTASK_DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
			ConnectTimeout:         getDuration("HUBTASK_DB_CONNECT_TIMEOUT", 5*time.Second),
			StatementTimeout:       getDuration("HUBTASK_DB_STATEMENT_TIMEOUT", 5*time.Second),
			WorkerStatementTimeout: getDuration("HUBTASK_DB_WORKER_STATEMENT_TIMEOUT", 60*time.Second),
		},
		Storage: env.StorageConfig{
			Kind:         env.StorageKind(strings.ToLower(get("HUBTASK_STORAGE_KIND", string(env.StorageLocal)))),
			LocalPath:    get("HUBTASK_STORAGE_LOCAL_PATH", "/var/lib/hubtask/media"),
			Endpoint:     get("HUBTASK_S3_ENDPOINT", ""),
			Region:       get("HUBTASK_S3_REGION", "us-east-1"),
			Bucket:       get("HUBTASK_S3_BUCKET", ""),
			UsePathStyle: getBool("HUBTASK_S3_USE_PATH_STYLE", true),
		},
		Mail: env.MailConfig{
			Host:     get("HUBTASK_SMTP_HOST", ""),
			Port:     getInt("HUBTASK_SMTP_PORT", 587),
			Username: get("HUBTASK_SMTP_USER", ""),
			From:     get("HUBTASK_SMTP_FROM", ""),
			Security: env.MailSecurity(strings.ToLower(get("HUBTASK_SMTP_SECURITY", string(env.MailSecurityStartTLS)))),
			Timeout:  getDuration("HUBTASK_SMTP_TIMEOUT", 10*time.Second),
		},
		RateLimit: env.RateLimitConfig{
			AnonymousPerMinute: getInt("HUBTASK_RATE_LIMIT_ANONYMOUS_PER_MINUTE", 60),
			TokenPerMinute:     getInt("HUBTASK_RATE_LIMIT_TOKEN_PER_MINUTE", 600),
			TenantPerMinute:    getInt("HUBTASK_RATE_LIMIT_TENANT_PER_MINUTE", 3000),
			AuthPerMinute:      getInt("HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE", 10),
			Burst:              getInt("HUBTASK_RATE_LIMIT_BURST", 20),
		},
		Request: env.RequestConfig{
			MaxBodyBytes:   int64(getInt("HUBTASK_MAX_BODY_BYTES", 1<<20)),   // 1 MiB
			MaxUploadBytes: int64(getInt("HUBTASK_MAX_UPLOAD_BYTES", 1<<26)), // 64 MiB
			Timeout:        getDuration("HUBTASK_REQUEST_TIMEOUT", 30*time.Second),
		},
		CORS: env.CORSConfig{
			AllowedOrigins: getList("HUBTASK_CORS_ALLOWED_ORIGINS"),
			MaxAge:         getDuration("HUBTASK_CORS_MAX_AGE", 10*time.Minute),
		},
		Outbound: env.OutboundConfig{
			Timeout:              getDuration("HUBTASK_HTTP_TIMEOUT", 10*time.Second),
			ConnectTimeout:       getDuration("HUBTASK_HTTP_CONNECT_TIMEOUT", 5*time.Second),
			MaxResponseBytes:     int64(getInt("HUBTASK_HTTP_MAX_RESPONSE_BYTES", 1<<20)), // 1 MiB
			MaxRedirects:         getInt("HUBTASK_HTTP_MAX_REDIRECTS", 3),
			AllowedHosts:         getList("HUBTASK_HTTP_ALLOWED_HOSTS"),
			AllowPrivateNetworks: getBool("HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS", false),
		},
		Queue: env.QueueConfig{
			PollInterval:      getDuration("HUBTASK_QUEUE_POLL_INTERVAL", 2*time.Second),
			BatchSize:         getInt("HUBTASK_QUEUE_BATCH_SIZE", 10),
			JobTimeout:        getDuration("HUBTASK_JOB_TIMEOUT", 60*time.Second),
			MaxAttempts:       getInt("HUBTASK_JOB_MAX_ATTEMPTS", 8),
			RetryBase:         getDuration("HUBTASK_JOB_RETRY_BASE", 5*time.Second),
			RetryMax:          getDuration("HUBTASK_JOB_RETRY_MAX", 15*time.Minute),
			SchedulerTick:     getDuration("HUBTASK_SCHEDULER_TICK_INTERVAL", 10*time.Second),
			OutboxBatch:       getInt("HUBTASK_OUTBOX_BATCH_SIZE", 100),
			OutboxMinInterval: getDuration("HUBTASK_OUTBOX_MIN_INTERVAL", time.Second),
			OutboxMaxInterval: getDuration("HUBTASK_OUTBOX_MAX_INTERVAL", 15*time.Second),
		},
		Retention: env.RetentionConfig{
			// 90 days: the maximum offline window offline-sync.md §7 documents, and therefore the
			// lower bound on a hard delete. A thousand rows a pass: data-retention.md §5's default.
			TombstoneWindow: getDuration("HUBTASK_TOMBSTONE_WINDOW", 90*24*time.Hour),
			BatchSize:       getInt("HUBTASK_RETENTION_BATCH_SIZE", 1000),
			Interval:        getDuration("HUBTASK_RETENTION_INTERVAL", time.Hour),
		},
		Media: env.MediaConfig{
			// A day for an abandoned staging: long enough that no upload over any line anybody
			// still uses is mistaken for one, short enough that a bucket does not fill with them.
			// An hour of grace before the bytes go, which is the window in which an object that
			// lost its last reference and gained a new one is recounted rather than removed.
			StagingGrace: getDuration("HUBTASK_MEDIA_STAGING_GRACE", 24*time.Hour),
			OrphanGrace:  getDuration("HUBTASK_MEDIA_ORPHAN_GRACE", time.Hour),
			BatchSize:    getInt("HUBTASK_MEDIA_RECONCILE_BATCH_SIZE", 100),
			Interval:     getDuration("HUBTASK_MEDIA_RECONCILE_INTERVAL", 6*time.Hour),
		},
		Metrics: env.MetricsConfig{
			TenantLabel: getBool("HUBTASK_METRICS_TENANT_LABEL", false),
		},
		Tracing: env.TracingConfig{
			Enabled:     getBool("HUBTASK_TRACING_ENABLED", false),
			Endpoint:    get("HUBTASK_TRACING_ENDPOINT", ""),
			SampleRatio: getFloat("HUBTASK_TRACING_SAMPLE_RATIO", 0.05),
		},
		Locale: env.LocaleConfig{
			DefaultLocale:   get("HUBTASK_DEFAULT_LOCALE", "en"),
			DefaultTimeZone: get("HUBTASK_DEFAULT_TIMEZONE", "UTC"),
		},
		UI: env.UIConfig{
			Enabled: getBool("HUBTASK_UI_ENABLED", true),
		},
	}

	roles, err := parseRoles(get("HUBTASK_ROLES", "api,worker,scheduler,automation"))
	if err != nil {
		return cfg, err
	}
	cfg.Roles = roles

	keys, err := parseEncryptionKeys(get("HUBTASK_ENCRYPTION_KEYS", ""))
	if err != nil {
		return cfg, err
	}
	cfg.Encryption = env.EncryptionConfig{Keys: keys}

	// Secrets: also available as *_FILE, so that Docker and Kubernetes secrets can be used
	// without the detour through environment variables.
	secrets := map[string]*secret.Secret{
		"HUBTASK_DB_DSN":        &cfg.Database.DSN,
		"HUBTASK_SECRET_KEY":    &cfg.SecretKey,
		"HUBTASK_S3_ACCESS_KEY": &cfg.Storage.AccessKey,
		"HUBTASK_S3_SECRET_KEY": &cfg.Storage.SecretKey,
		"HUBTASK_SMTP_PASSWORD": &cfg.Mail.Password,
	}
	for key, target := range secrets {
		value, err := getSecret(key)
		if err != nil {
			return cfg, err
		}
		*target = value
	}

	return cfg, validate(cfg)
}

// Warnings reports what the installation is missing without it being an error.
// These codes appear in /meta/health (ADR-0016) and are translatable on the client side.
func (e *EnvConfig) Warnings(cfg env.Config) []env.Warning {
	var w []env.Warning
	if os.Getenv("HUBTASK_BACKUP_LOCAL_PATH") == "" && os.Getenv("HUBTASK_BACKUP_TARGETS") == "" {
		w = append(w, env.Warning{Code: "config.backup_not_configured", Severity: "warn"})
	}
	if cfg.Mail.Host == "" {
		// Two different statements, and the roles decide which one it is. An installation that
		// serves the API and sends nothing has no mail server, which is worth knowing; one that
		// runs the worker or the scheduler fires reminders that have nowhere to go, which is the
		// promised warning of observability-reliability.md §7 - the record exists, the message
		// waits in the queue, and nobody is told until somebody configures SMTP (D-03).
		if cfg.HasRole(env.RoleWorker) || cfg.HasRole(env.RoleScheduler) {
			w = append(w, env.Warning{Code: "config.smtp_missing_with_reminders", Severity: "warn"})
		} else {
			w = append(w, env.Warning{Code: "config.smtp_missing", Severity: "info"})
		}
	}
	if cfg.BaseURL == "" {
		w = append(w, env.Warning{Code: "config.base_url_missing", Severity: "warn"})
	}
	if cfg.Tenancy == env.TenancyMulti && os.Getenv("HUBTASK_OIDC_ISSUER") == "" {
		w = append(w, env.Warning{Code: "config.oidc_missing_in_multi_tenancy", Severity: "warn"})
	}
	// In provider operation an egress allowlist is mandatory (security.md §T-07). It cannot be
	// an error - an installation that refuses to start because a list is empty is worse than
	// one that says so - but it belongs in /meta/health where the operator will see it.
	if cfg.Tenancy == env.TenancyMulti && len(cfg.Outbound.AllowedHosts) == 0 {
		w = append(w, env.Warning{Code: "config.egress_allowlist_missing", Severity: "warn"})
	}
	if cfg.Outbound.AllowPrivateNetworks {
		w = append(w, env.Warning{Code: "config.egress_private_networks_allowed", Severity: "warn"})
	}
	if cfg.Mail.Security == env.MailSecurityNone && cfg.Mail.Host != "" {
		w = append(w, env.Warning{
			Code:     "config.smtp_without_tls",
			Severity: "warn",
			Params:   map[string]string{"host": cfg.Mail.Host},
		})
	}
	return w
}

// configError builds a startup error: category VALIDATION, the stable code config_invalid, and
// a detail code naming the variable. The operator reads it in the log, a client can translate it.
func configError(detailCode, variable string) *shared.Error {
	return shared.New(shared.CategoryValidation, "config_invalid").
		WithDetail(detailCode).
		WithParams(map[string]string{"variable": variable})
}

func validate(cfg env.Config) error {
	var errs []error

	// Fail closed: no default for a secret. A generated key would be worse than a startup
	// error, because it renders all data unreadable after a restart (security.md §8).
	if cfg.Database.DSN.IsEmpty() {
		errs = append(errs, configError("config.db_dsn_missing", "HUBTASK_DB_DSN"))
	}
	if cfg.SecretKey.IsEmpty() {
		errs = append(errs, configError("config.secret_key_missing", "HUBTASK_SECRET_KEY"))
	} else if len(cfg.SecretKey.Reveal()) < minSecretKeyLength {
		errs = append(errs, configError("config.secret_key_too_short", "HUBTASK_SECRET_KEY").
			WithParams(map[string]string{
				"variable": "HUBTASK_SECRET_KEY",
				"minimum":  strconv.Itoa(minSecretKeyLength),
			}))
	}

	// The same floor as the installation secret, for the same reason: material an operator could
	// have typed from memory is not a key, and stretching it adds no entropy. Checked here as well
	// as in the keyring, because a process that starts and only discovers at the first backup that
	// its key is unusable has moved the failure to the worst possible moment.
	for _, key := range cfg.Encryption.Keys {
		variable := "HUBTASK_ENCRYPTION_KEY_" + strings.ToUpper(key.ID)
		if len(key.Material.Reveal()) < minSecretKeyLength {
			errs = append(errs, configError("config.encryption_key_too_short", variable).
				WithParams(map[string]string{
					"variable": variable,
					"key_id":   key.ID,
					"minimum":  strconv.Itoa(minSecretKeyLength),
				}))
		}
	}

	switch cfg.Tenancy {
	case env.TenancySingle, env.TenancyMulti:
	default:
		errs = append(errs, configError("config.tenancy_invalid", "HUBTASK_TENANCY_MODE"))
	}
	if len(cfg.Roles) == 0 {
		errs = append(errs, configError("config.roles_empty", "HUBTASK_ROLES"))
	}
	if cfg.ShutdownGraceSeconds <= 0 {
		errs = append(errs, configError("config.shutdown_grace_invalid", "HUBTASK_SHUTDOWN_GRACE_SECONDS"))
	}
	// Zero is allowed and negative is not: "do not wait" is a real answer for an installation
	// with nothing in front of it, and a negative wait is a typo.
	if cfg.ShutdownDeregisterSeconds < 0 {
		errs = append(errs, configError("config.shutdown_deregister_invalid", "HUBTASK_SHUTDOWN_DEREGISTER_SECONDS"))
	}

	errs = append(errs, validateLogging(cfg)...)
	errs = append(errs, validateDatabase(cfg.Database)...)
	errs = append(errs, validateStorage(cfg.Storage)...)
	errs = append(errs, validateMail(cfg.Mail)...)
	errs = append(errs, validateRateLimit(cfg.RateLimit)...)
	errs = append(errs, validateRequest(cfg.Request)...)
	errs = append(errs, validateCORS(cfg.CORS)...)
	errs = append(errs, validateOutbound(cfg.Outbound)...)
	errs = append(errs, validateLocale(cfg.Locale)...)
	errs = append(errs, validateTracing(cfg.Tracing)...)
	errs = append(errs, validateQueue(cfg.Queue)...)

	return errors.Join(errs...)
}

func validateLogging(cfg env.Config) []error {
	var errs []error
	switch cfg.LogFormat {
	case "json", "text":
	default:
		errs = append(errs, configError("config.log_format_invalid", "HUBTASK_LOG_FORMAT"))
	}
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, configError("config.log_level_invalid", "HUBTASK_LOG_LEVEL"))
	}
	return errs
}

func validateDatabase(db env.DatabaseConfig) []error {
	var errs []error
	// The upper bound matters as much as the lower one: the value reaches the driver as an
	// int32, and an unbounded number parsed from the environment would wrap.
	if db.MaxConns < 1 || db.MaxConns > env.MaxPoolConns {
		errs = append(errs, configError("config.db_pool_invalid", "HUBTASK_DB_MAX_CONNS").
			WithParams(map[string]string{
				"variable": "HUBTASK_DB_MAX_CONNS",
				"maximum":  strconv.Itoa(env.MaxPoolConns),
			}))
	}
	if db.MinConns < 0 || db.MinConns > db.MaxConns {
		errs = append(errs, configError("config.db_pool_invalid", "HUBTASK_DB_MIN_CONNS"))
	}
	for variable, d := range map[string]time.Duration{
		"HUBTASK_DB_CONNECT_TIMEOUT":          db.ConnectTimeout,
		"HUBTASK_DB_STATEMENT_TIMEOUT":        db.StatementTimeout,
		"HUBTASK_DB_WORKER_STATEMENT_TIMEOUT": db.WorkerStatementTimeout,
		"HUBTASK_DB_MAX_CONN_LIFETIME":        db.MaxConnLifetime,
		"HUBTASK_DB_MAX_CONN_IDLE_TIME":       db.MaxConnIdleTime,
	} {
		if d <= 0 {
			errs = append(errs, configError("config.duration_invalid", variable))
		}
	}
	return errs
}

func validateQueue(q env.QueueConfig) []error {
	var errs []error
	for variable, count := range map[string]int{
		"HUBTASK_QUEUE_BATCH_SIZE":  q.BatchSize,
		"HUBTASK_JOB_MAX_ATTEMPTS":  q.MaxAttempts,
		"HUBTASK_OUTBOX_BATCH_SIZE": q.OutboxBatch,
	} {
		if count < 1 {
			errs = append(errs, configError("config.limit_invalid", variable))
		}
	}
	for variable, d := range map[string]time.Duration{
		"HUBTASK_QUEUE_POLL_INTERVAL":     q.PollInterval,
		"HUBTASK_JOB_TIMEOUT":             q.JobTimeout,
		"HUBTASK_JOB_RETRY_BASE":          q.RetryBase,
		"HUBTASK_JOB_RETRY_MAX":           q.RetryMax,
		"HUBTASK_SCHEDULER_TICK_INTERVAL": q.SchedulerTick,
		"HUBTASK_OUTBOX_MIN_INTERVAL":     q.OutboxMinInterval,
		"HUBTASK_OUTBOX_MAX_INTERVAL":     q.OutboxMaxInterval,
	} {
		if d <= 0 {
			errs = append(errs, configError("config.duration_invalid", variable))
		}
	}
	// A maximum below the base would make the backoff shrink with every attempt, which is the
	// opposite of what a backoff is for.
	if q.RetryMax > 0 && q.RetryBase > 0 && q.RetryMax < q.RetryBase {
		errs = append(errs, configError("config.duration_invalid", "HUBTASK_JOB_RETRY_MAX"))
	}
	if q.OutboxMaxInterval > 0 && q.OutboxMinInterval > 0 && q.OutboxMaxInterval < q.OutboxMinInterval {
		errs = append(errs, configError("config.duration_invalid", "HUBTASK_OUTBOX_MAX_INTERVAL"))
	}
	return errs
}

func validateStorage(s env.StorageConfig) []error {
	var errs []error
	switch s.Kind {
	case env.StorageLocal:
		if s.LocalPath == "" {
			errs = append(errs, configError("config.storage_path_missing", "HUBTASK_STORAGE_LOCAL_PATH"))
		}
	case env.StorageS3:
		// Fail closed rather than start and discover at the first upload that the bucket is
		// unnamed: a half-configured storage is a broken storage.
		if s.Bucket == "" {
			errs = append(errs, configError("config.s3_incomplete", "HUBTASK_S3_BUCKET"))
		}
		if s.AccessKey.IsEmpty() {
			errs = append(errs, configError("config.s3_incomplete", "HUBTASK_S3_ACCESS_KEY"))
		}
		if s.SecretKey.IsEmpty() {
			errs = append(errs, configError("config.s3_incomplete", "HUBTASK_S3_SECRET_KEY"))
		}
	default:
		errs = append(errs, configError("config.storage_kind_invalid", "HUBTASK_STORAGE_KIND"))
	}
	return errs
}

func validateMail(m env.MailConfig) []error {
	var errs []error
	if m.Host == "" {
		// No SMTP at all is a warning, not an error: notifications degrade, nothing breaks.
		return nil
	}
	if m.Port < 1 || m.Port > 65535 {
		errs = append(errs, configError("config.smtp_port_invalid", "HUBTASK_SMTP_PORT"))
	}
	if m.From == "" {
		errs = append(errs, configError("config.smtp_from_missing", "HUBTASK_SMTP_FROM"))
	}
	switch m.Security {
	case env.MailSecurityStartTLS, env.MailSecurityTLS, env.MailSecurityNone:
	default:
		errs = append(errs, configError("config.smtp_security_invalid", "HUBTASK_SMTP_SECURITY"))
	}
	if m.Timeout <= 0 {
		errs = append(errs, configError("config.duration_invalid", "HUBTASK_SMTP_TIMEOUT"))
	}
	return errs
}

func validateRateLimit(r env.RateLimitConfig) []error {
	var errs []error
	for variable, v := range map[string]int{
		"HUBTASK_RATE_LIMIT_ANONYMOUS_PER_MINUTE": r.AnonymousPerMinute,
		"HUBTASK_RATE_LIMIT_TOKEN_PER_MINUTE":     r.TokenPerMinute,
		"HUBTASK_RATE_LIMIT_TENANT_PER_MINUTE":    r.TenantPerMinute,
		"HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE":      r.AuthPerMinute,
		"HUBTASK_RATE_LIMIT_BURST":                r.Burst,
	} {
		if v < 1 {
			// Zero would mean "everything blocked", which nobody configures on purpose - and
			// a rate limit that is off by accident is exactly what T-17 is about.
			errs = append(errs, configError("config.rate_limit_invalid", variable))
		}
	}
	return errs
}

func validateRequest(r env.RequestConfig) []error {
	var errs []error
	if r.MaxBodyBytes < 1 {
		errs = append(errs, configError("config.limit_invalid", "HUBTASK_MAX_BODY_BYTES"))
	}
	if r.MaxUploadBytes < 1 {
		errs = append(errs, configError("config.limit_invalid", "HUBTASK_MAX_UPLOAD_BYTES"))
	}
	if r.Timeout <= 0 {
		errs = append(errs, configError("config.duration_invalid", "HUBTASK_REQUEST_TIMEOUT"))
	}
	return errs
}

func validateOutbound(o env.OutboundConfig) []error {
	var errs []error
	for variable, d := range map[string]time.Duration{
		"HUBTASK_HTTP_TIMEOUT":         o.Timeout,
		"HUBTASK_HTTP_CONNECT_TIMEOUT": o.ConnectTimeout,
	} {
		if d <= 0 {
			errs = append(errs, configError("config.duration_invalid", variable))
		}
	}
	if o.MaxResponseBytes < 1 {
		errs = append(errs, configError("config.limit_invalid", "HUBTASK_HTTP_MAX_RESPONSE_BYTES"))
	}
	// Zero is a valid answer here - "follow nothing" is the strictest setting, not a mistake.
	// The upper bound is the interesting one: a long chain is not a site that moved.
	if o.MaxRedirects < 0 || o.MaxRedirects > env.MaxOutboundRedirects {
		errs = append(errs, configError("config.redirects_invalid", "HUBTASK_HTTP_MAX_REDIRECTS").
			WithParams(map[string]string{
				"variable": "HUBTASK_HTTP_MAX_REDIRECTS",
				"maximum":  strconv.Itoa(env.MaxOutboundRedirects),
			}))
	}
	for _, host := range o.AllowedHosts {
		// A scheme or a path in the allowlist means somebody expected URL matching. Silently
		// ignoring it would leave the operator believing a target is allowed when it is not.
		if strings.ContainsAny(host, "/:") {
			errs = append(errs, configError("config.allowed_host_invalid", "HUBTASK_HTTP_ALLOWED_HOSTS").
				WithParams(map[string]string{
					"variable": "HUBTASK_HTTP_ALLOWED_HOSTS",
					"value":    host,
				}))
		}
	}
	return errs
}

// validateCORS insists on complete origins. A browser sends `Origin: https://app.example.com`
// and compares byte for byte; a host name without a scheme, or one with a trailing slash, matches
// nothing - and an allowlist that silently matches nothing is worse than none, because the
// operator believes the frontend is allowed.
func validateCORS(c env.CORSConfig) []error {
	var errs []error

	if c.MaxAge < 0 {
		errs = append(errs, configError("config.duration_invalid", "HUBTASK_CORS_MAX_AGE"))
	}
	if c.AllowsAnyOrigin() {
		return errs
	}
	for _, origin := range c.AllowedOrigins {
		if origin == env.CORSWildcard {
			// "*" alongside named origins is a contradiction: either every origin is allowed or
			// a list of them is.
			errs = append(errs, corsOriginError(origin))
			continue
		}
		scheme, rest, found := strings.Cut(origin, "://")
		if !found || (scheme != "http" && scheme != "https") ||
			rest == "" || strings.ContainsAny(rest, "/?#") {
			errs = append(errs, corsOriginError(origin))
		}
	}
	return errs
}

func corsOriginError(origin string) error {
	return configError("config.cors_origin_invalid", "HUBTASK_CORS_ALLOWED_ORIGINS").
		WithParams(map[string]string{
			"variable": "HUBTASK_CORS_ALLOWED_ORIGINS",
			"value":    origin,
		})
}

func validateTracing(t env.TracingConfig) []error {
	var errs []error
	if t.SampleRatio < 0 || t.SampleRatio > 1 {
		errs = append(errs, configError("config.sample_ratio_invalid", "HUBTASK_TRACING_SAMPLE_RATIO"))
	}
	// Enabled without a destination is the mistake that looks like it works: spans are built,
	// cost time, and go nowhere.
	if t.Enabled && t.Endpoint == "" {
		errs = append(errs, configError("config.tracing_endpoint_missing", "HUBTASK_TRACING_ENDPOINT"))
	}
	return errs
}

func validateLocale(l env.LocaleConfig) []error {
	var errs []error
	if !looksLikeBCP47(l.DefaultLocale) {
		errs = append(errs, configError("config.locale_invalid", "HUBTASK_DEFAULT_LOCALE"))
	}
	// An IANA name, never a fixed offset - an offset cannot represent daylight saving
	// (i18n-l10n.md §2). LoadLocation reads the embedded tzdata, so this works in the
	// distroless image too.
	if _, err := time.LoadLocation(l.DefaultTimeZone); err != nil {
		errs = append(errs, configError("config.timezone_unknown", "HUBTASK_DEFAULT_TIMEZONE").
			WithParams(map[string]string{
				"variable": "HUBTASK_DEFAULT_TIMEZONE",
				"value":    l.DefaultTimeZone,
			}))
	}
	return errs
}

// looksLikeBCP47 is a syntax check, not a registry lookup: subtags of two to eight letters or
// digits, separated by hyphens. Real negotiation against the catalogue happens in the i18n
// adapter (i18n-l10n.md §2), which is where a mistyped locale would otherwise surface far too
// late.
func looksLikeBCP47(tag string) bool {
	if tag == "" {
		return false
	}
	for i, part := range strings.Split(tag, "-") {
		if len(part) < 1 || len(part) > 8 {
			return false
		}
		if i == 0 && (len(part) < 2 || !isAlpha(part)) {
			return false
		}
		if !isAlphanumeric(part) {
			return false
		}
	}
	return true
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

func isAlphanumeric(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func parseRoles(raw string) ([]env.Role, error) {
	var out []env.Role
	for _, part := range strings.Split(raw, ",") {
		r := env.Role(strings.TrimSpace(strings.ToLower(part)))
		if r == "" {
			continue
		}
		switch r {
		case env.RoleAPI, env.RoleWorker, env.RoleScheduler, env.RoleAutomation:
			out = append(out, r)
		default:
			return nil, configError("config.role_unknown", "HUBTASK_ROLES").
				WithParams(map[string]string{"variable": "HUBTASK_ROLES", "value": string(r)})
		}
	}
	return out, nil
}

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// getList reads a comma-separated list. Entries are trimmed and lower-cased, because a host name
// is case-insensitive and an allowlist that misses "API.Example.COM" is an allowlist that fails
// open at the worst moment. Empty entries are dropped rather than kept as "": a trailing comma
// is a typo, and an empty host would match nothing anyway.
func getList(key string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.ToLower(strings.TrimSpace(part)); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return -1 // invalid, and validate turns that into a startup error
	}
	return fallback
}

// getDuration accepts Go duration syntax (30s, 5m, 1h30m). A plain number is rejected rather
// than guessed at: "30" as nanoseconds is a trap nobody means.
func getDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		return -1 // invalid, and validate turns that into a startup error
	}
	return fallback
}

// encryptionKeyIDPattern is what an identifier may be, and it is narrow for three reasons at
// once: the value is stored in every row sealed under the key, it is printed in log lines an
// operator reads, and it becomes part of an environment variable name. No case, no punctuation,
// nothing that would need quoting in a shell.
var encryptionKeyIDPattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// parseEncryptionKeys reads the keyring: HUBTASK_ENCRYPTION_KEYS names the identifiers, current
// first, and each key's material comes from HUBTASK_ENCRYPTION_KEY_<ID> - or its _FILE form, so
// that every key can be its own Docker or Kubernetes secret.
//
// Two variables rather than one list of "id:material" pairs, and deliberately: a single list needs
// a delimiter, and a delimiter inside a generated secret is a key that silently loads as two
// halves of nothing. Splitting it also means no key material ever appears in the variable that
// says which keys exist, which is the one of the two an operator pastes into a support ticket.
func parseEncryptionKeys(list string) ([]env.EncryptionKey, error) {
	if strings.TrimSpace(list) == "" {
		return nil, nil
	}

	var keys []env.EncryptionKey
	seen := map[string]bool{}

	for _, raw := range strings.Split(list, ",") {
		id := strings.ToLower(strings.TrimSpace(raw))
		if !encryptionKeyIDPattern.MatchString(id) {
			return nil, configError("config.encryption_key_id_invalid", "HUBTASK_ENCRYPTION_KEYS").
				WithParams(map[string]string{
					"variable": "HUBTASK_ENCRYPTION_KEYS", "key_id": id,
				})
		}
		if seen[id] {
			return nil, configError("config.encryption_key_id_duplicate", "HUBTASK_ENCRYPTION_KEYS").
				WithParams(map[string]string{
					"variable": "HUBTASK_ENCRYPTION_KEYS", "key_id": id,
				})
		}
		seen[id] = true

		variable := "HUBTASK_ENCRYPTION_KEY_" + strings.ToUpper(id)
		material, err := getSecret(variable)
		if err != nil {
			return nil, err
		}
		if material.IsEmpty() {
			// Named and not supplied. A startup error rather than a ring quietly missing a key:
			// the value that would fail is one nobody notices until an old archive will not open.
			return nil, configError("config.encryption_key_missing", variable).
				WithParams(map[string]string{"variable": variable, "key_id": id})
		}
		keys = append(keys, env.EncryptionKey{ID: id, Material: material})
	}
	return keys, nil
}

// getSecret supports KEY and KEY_FILE (Docker and Kubernetes secrets).
func getSecret(key string) (secret.Secret, error) {
	if path, ok := os.LookupEnv(key + "_FILE"); ok && path != "" {
		// The path comes from the process environment, which only the operator controls -
		// that is the whole point of the _FILE convention (Docker and Kubernetes secrets).
		b, err := os.ReadFile(path) //nolint:gosec // G304: the operator names the secret file
		if err != nil {
			return secret.Secret{}, configError("config.secret_file_unreadable", key+"_FILE").
				WithParams(map[string]string{"variable": key + "_FILE"}).
				WithCause(fmt.Errorf("%s_FILE is not readable: %w", key, err))
		}
		return secret.New(strings.TrimSpace(string(b))), nil
	}
	return secret.New(os.Getenv(key)), nil
}
