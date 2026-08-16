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
			Username: get("HUBTASK_SMTP_USERNAME", ""),
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
		Locale: env.LocaleConfig{
			DefaultLocale:   get("HUBTASK_DEFAULT_LOCALE", "en"),
			DefaultTimeZone: get("HUBTASK_DEFAULT_TIMEZONE", "UTC"),
		},
	}

	roles, err := parseRoles(get("HUBTASK_ROLES", "api,worker,scheduler,automation"))
	if err != nil {
		return cfg, err
	}
	cfg.Roles = roles

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
		w = append(w, env.Warning{Code: "config.smtp_missing", Severity: "info"})
	}
	if cfg.BaseURL == "" {
		w = append(w, env.Warning{Code: "config.base_url_missing", Severity: "warn"})
	}
	if cfg.Tenancy == env.TenancyMulti && os.Getenv("HUBTASK_OIDC_ISSUER") == "" {
		w = append(w, env.Warning{Code: "config.oidc_missing_in_multi_tenancy", Severity: "warn"})
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

	errs = append(errs, validateLogging(cfg)...)
	errs = append(errs, validateDatabase(cfg.Database)...)
	errs = append(errs, validateStorage(cfg.Storage)...)
	errs = append(errs, validateMail(cfg.Mail)...)
	errs = append(errs, validateRateLimit(cfg.RateLimit)...)
	errs = append(errs, validateRequest(cfg.Request)...)
	errs = append(errs, validateLocale(cfg.Locale)...)

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
	if db.MaxConns < 1 {
		errs = append(errs, configError("config.db_pool_invalid", "HUBTASK_DB_MAX_CONNS"))
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

func getBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
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
