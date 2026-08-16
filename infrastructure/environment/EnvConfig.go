// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package environment is the adapter for the configuration port: it reads HUBTASK_* variables.
package environment

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

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
	}

	roles, err := parseRoles(get("HUBTASK_ROLES", "api,worker,scheduler,automation"))
	if err != nil {
		return cfg, err
	}
	cfg.Roles = roles

	// Secrets: also available as *_FILE, so that Docker and Kubernetes secrets can be used
	// without the detour through environment variables.
	dsn, err := getSecret("HUBTASK_DB_DSN")
	if err != nil {
		return cfg, err
	}
	key, err := getSecret("HUBTASK_SECRET_KEY")
	if err != nil {
		return cfg, err
	}
	cfg.DatabaseDSN, cfg.SecretKey = dsn, key

	return cfg, validate(cfg)
}

// Warnings reports what the installation is missing without it being an error.
// These codes appear in /meta/health (ADR-0016) and are translatable on the client side.
func (e *EnvConfig) Warnings(cfg env.Config) []env.Warning {
	var w []env.Warning
	if os.Getenv("HUBTASK_BACKUP_LOCAL_PATH") == "" && os.Getenv("HUBTASK_BACKUP_TARGETS") == "" {
		w = append(w, env.Warning{Code: "config.backup_not_configured", Severity: "warn"})
	}
	if os.Getenv("HUBTASK_SMTP_HOST") == "" {
		w = append(w, env.Warning{Code: "config.smtp_missing", Severity: "info"})
	}
	if cfg.BaseURL == "" {
		w = append(w, env.Warning{Code: "config.base_url_missing", Severity: "warn"})
	}
	if cfg.Tenancy == env.TenancyMulti && os.Getenv("HUBTASK_OIDC_ISSUER") == "" {
		w = append(w, env.Warning{Code: "config.oidc_missing_in_multi_tenancy", Severity: "warn"})
	}
	return w
}

func validate(cfg env.Config) error {
	var errs []error
	if cfg.DatabaseDSN.IsEmpty() {
		errs = append(errs, errors.New("HUBTASK_DB_DSN is missing"))
	}
	// Fail closed: no default for the key - a generated random value would be worse than a
	// startup error, because it would render all data unreadable after a restart.
	if cfg.SecretKey.IsEmpty() {
		errs = append(errs, errors.New("HUBTASK_SECRET_KEY is missing"))
	}
	if len(cfg.SecretKey.Reveal()) > 0 && len(cfg.SecretKey.Reveal()) < 32 {
		errs = append(errs, errors.New("HUBTASK_SECRET_KEY must be at least 32 characters long"))
	}
	switch cfg.Tenancy {
	case env.TenancySingle, env.TenancyMulti:
	default:
		errs = append(errs, fmt.Errorf("HUBTASK_TENANCY_MODE is invalid: %q", cfg.Tenancy))
	}
	if len(cfg.Roles) == 0 {
		errs = append(errs, errors.New("HUBTASK_ROLES is empty"))
	}
	return errors.Join(errs...)
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
			return nil, fmt.Errorf("unknown role in HUBTASK_ROLES: %q", r)
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

// getSecret supports KEY and KEY_FILE (Docker and Kubernetes secrets).
func getSecret(key string) (secret.Secret, error) {
	if path, ok := os.LookupEnv(key + "_FILE"); ok && path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return secret.Secret{}, fmt.Errorf("%s_FILE is not readable: %w", key, err)
		}
		return secret.New(strings.TrimSpace(string(b))), nil
	}
	return secret.New(os.Getenv(key)), nil
}
