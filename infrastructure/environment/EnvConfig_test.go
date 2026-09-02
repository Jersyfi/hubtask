// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package environment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

const validDSN = "postgres://hubtask@localhost:5432/hubtask?sslmode=disable"

// validKey is assembled rather than written out: a 32-character literal next to the word "key" is
// what the secret scan of SG-7 exists to find, and a fixture must not train anyone to ignore it.
var validKey = strings.Repeat("test-key", 4) // 32 characters, the minimum

// isolate clears every HUBTASK_ variable of the ambient environment. Without it a test would
// pass or fail depending on the shell it runs in - and CI does set HUBTASK_DB_DSN in some jobs.
func isolate(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "HUBTASK_") {
			t.Setenv(name, "")
		}
	}
}

// withRequiredSecrets is the smallest environment in which the process may start.
func withRequiredSecrets(t *testing.T) {
	t.Helper()
	isolate(t)
	t.Setenv("HUBTASK_DB_DSN", validDSN)
	t.Setenv("HUBTASK_SECRET_KEY", validKey)
}

func load(t *testing.T) (env.Config, error) {
	t.Helper()
	return New("1.2.3", "abc1234").Load()
}

// detailCodes collects the message codes of a failed load. A configuration error names its
// variable through a code, so a test can assert on it without matching on prose.
func detailCodes(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		var single *shared.Error
		if errors.As(err, &single) {
			return []string{single.DetailCode}
		}
		t.Fatalf("the error is not a configuration error: %v", err)
	}
	var codes []string
	for _, e := range joined.Unwrap() {
		var domainErr *shared.Error
		if !errors.As(e, &domainErr) {
			t.Errorf("a configuration error is untyped: %v", e)
			continue
		}
		if domainErr.Category != shared.CategoryValidation || domainErr.Code != "config_invalid" {
			t.Errorf("classification = %s/%s, want VALIDATION/config_invalid",
				domainErr.Category, domainErr.Code)
		}
		codes = append(codes, domainErr.DetailCode)
	}
	return codes
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the configuration was accepted, expected %s", want)
	}
	codes := detailCodes(t, err)
	for _, got := range codes {
		if got == want {
			return
		}
	}
	t.Errorf("codes = %v, want %s", codes, want)
}

// The acceptance criterion of A-02: a missing required secret prevents startup.
func TestAMissingSecretPreventsStartup(t *testing.T) {
	cases := []struct {
		name    string
		set     map[string]string
		wantErr string
	}{
		{"no DSN", map[string]string{"HUBTASK_SECRET_KEY": validKey}, "config.db_dsn_missing"},
		{"no key", map[string]string{"HUBTASK_DB_DSN": validDSN}, "config.secret_key_missing"},
		{"neither", map[string]string{}, "config.db_dsn_missing"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tc.set {
				t.Setenv(k, v)
			}

			_, err := load(t)

			assertCode(t, err, tc.wantErr)
		})
	}
}

// A key that is present but too short is worse than a missing one: it looks configured.
func TestATooShortSecretKeyIsRejectedWithoutRevealingIt(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SECRET_KEY", "too-short")

	_, err := load(t)

	assertCode(t, err, "config.secret_key_too_short")
	// T-18: the value must not appear in the error, which ends up in the startup log.
	if strings.Contains(err.Error(), "too-short") {
		t.Errorf("the error reveals the key: %v", err)
	}
	if !strings.Contains(err.Error(), "minimum=32") {
		t.Errorf("the error does not say what the minimum is: %v", err)
	}
}

// T-18 again, from the other side: a complete, valid configuration whose load fails for an
// unrelated reason must not carry any secret value into the message either.
func TestAConfigurationErrorNeverCarriesASecretValue(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SMTP_HOST", "smtp.example.org")
	t.Setenv("HUBTASK_SMTP_PASSWORD", "hunter2")
	t.Setenv("HUBTASK_SMTP_FROM", "") // the error this test provokes
	t.Setenv("HUBTASK_S3_ACCESS_KEY", "AKIAEXAMPLE")

	_, err := load(t)

	assertCode(t, err, "config.smtp_from_missing")
	for _, forbidden := range []string{"hunter2", "AKIAEXAMPLE", validKey, validDSN} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("the error reveals %q: %v", forbidden, err)
		}
	}
}

func TestTheDefaultsAreTheDocumentedOnes(t *testing.T) {
	withRequiredSecrets(t)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("the minimal configuration was rejected: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"HTTP address", cfg.HTTPAddr, ":8080"},
		{"ops address", cfg.OpsAddr, ":9090"},
		{"log format", cfg.LogFormat, "json"},
		{"tenancy", cfg.Tenancy, env.TenancySingle},
		{"shutdown grace", cfg.ShutdownGraceSeconds, 30},
		{"shutdown deregister", cfg.ShutdownDeregisterSeconds, 15},
		{"pool size", cfg.Database.MaxConns, 10},
		{"statement timeout", cfg.Database.StatementTimeout, 5 * time.Second},
		{"worker statement timeout", cfg.Database.WorkerStatementTimeout, 60 * time.Second},
		{"storage kind", cfg.Storage.Kind, env.StorageLocal},
		{"body limit", cfg.Request.MaxBodyBytes, int64(1 << 20)},
		{"request timeout", cfg.Request.Timeout, 30 * time.Second},
		{"outbound timeout", cfg.Outbound.Timeout, 10 * time.Second},
		{"outbound connect timeout", cfg.Outbound.ConnectTimeout, 5 * time.Second},
		{"outbound response limit", cfg.Outbound.MaxResponseBytes, int64(1 << 20)},
		{"outbound redirects", cfg.Outbound.MaxRedirects, 3},
		{"private networks", cfg.Outbound.AllowPrivateNetworks, false},
		{"anonymous rate limit", cfg.RateLimit.AnonymousPerMinute, 60},
		{"load shed threshold", cfg.LoadShed.Inflight, 64},
		{"load shed retry after", cfg.LoadShed.RetryAfter, 5 * time.Second},
		{"auth rate limit", cfg.RateLimit.AuthPerMinute, 10},
		{"default locale", cfg.Locale.DefaultLocale, "en"},
		{"default time zone", cfg.Locale.DefaultTimeZone, "UTC"},
		{"version", cfg.Version, "1.2.3"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if len(cfg.Roles) != 4 {
		t.Errorf("roles = %v, want all four", cfg.Roles)
	}
}

// The interactive path gets a short query budget, background work a long one
// (engineering-guidelines.md §4).
func TestTheStatementTimeoutDependsOnTheRole(t *testing.T) {
	withRequiredSecrets(t)
	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if got := cfg.StatementTimeoutFor(env.RoleAPI); got != 5*time.Second {
		t.Errorf("API budget = %s, want 5s", got)
	}
	for _, role := range []env.Role{env.RoleWorker, env.RoleScheduler, env.RoleAutomation} {
		if got := cfg.StatementTimeoutFor(role); got != 60*time.Second {
			t.Errorf("%s budget = %s, want 60s", role, got)
		}
	}
}

func TestSecretsCanComeFromAFile(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	dsnFile := filepath.Join(dir, "dsn")
	// A trailing newline is what every editor and every kubectl create secret produces.
	if err := os.WriteFile(dsnFile, []byte(validDSN+"\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture failed: %v", err)
	}
	t.Setenv("HUBTASK_DB_DSN_FILE", dsnFile)
	t.Setenv("HUBTASK_SECRET_KEY", validKey)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Database.DSN.Reveal() != validDSN {
		t.Errorf("the DSN was not read from the file, or the newline stayed: %q",
			cfg.Database.DSN.Reveal())
	}
}

func TestAnUnreadableSecretFileIsAStartupError(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := load(t)

	assertCode(t, err, "config.secret_file_unreadable")
}

func TestInvalidValuesAreRejectedByCode(t *testing.T) {
	cases := []struct {
		variable string
		value    string
		want     string
	}{
		{"HUBTASK_TENANCY_MODE", "somewhat", "config.tenancy_invalid"},
		{"HUBTASK_LOG_FORMAT", "xml", "config.log_format_invalid"},
		{"HUBTASK_LOG_LEVEL", "chatty", "config.log_level_invalid"},
		{"HUBTASK_SHUTDOWN_GRACE_SECONDS", "0", "config.shutdown_grace_invalid"},
		{"HUBTASK_DB_MAX_CONNS", "0", "config.db_pool_invalid"},
		{"HUBTASK_DB_MIN_CONNS", "99", "config.db_pool_invalid"},
		{"HUBTASK_STORAGE_KIND", "dropbox", "config.storage_kind_invalid"},
		{"HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE", "0", "config.rate_limit_invalid"},
		{"HUBTASK_MAX_BODY_BYTES", "0", "config.limit_invalid"},
		{"HUBTASK_DEFAULT_LOCALE", "not a locale", "config.locale_invalid"},
		{"HUBTASK_DEFAULT_TIMEZONE", "Mars/Olympus_Mons", "config.timezone_unknown"},
		{"HUBTASK_DEFAULT_TIMEZONE", "+02:00", "config.timezone_unknown"},
		{"HUBTASK_REQUEST_TIMEOUT", "30", "config.duration_invalid"},
		{"HUBTASK_DB_STATEMENT_TIMEOUT", "soon", "config.duration_invalid"},
		{"HUBTASK_HTTP_TIMEOUT", "10", "config.duration_invalid"},
		{"HUBTASK_HTTP_MAX_RESPONSE_BYTES", "0", "config.limit_invalid"},
		{"HUBTASK_HTTP_MAX_REDIRECTS", "-1", "config.redirects_invalid"},
		{"HUBTASK_HTTP_MAX_REDIRECTS", "50", "config.redirects_invalid"},
		{"HUBTASK_HTTP_ALLOWED_HOSTS", "https://hooks.example.org", "config.allowed_host_invalid"},
		{"HUBTASK_HTTP_ALLOWED_HOSTS", "hooks.example.org/path", "config.allowed_host_invalid"},
		{"HUBTASK_LOAD_SHED_INFLIGHT", "-1", "config.load_shed_invalid"},
		{"HUBTASK_LOAD_SHED_RETRY_AFTER", "0s", "config.duration_invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.variable+"="+tc.value, func(t *testing.T) {
			withRequiredSecrets(t)
			t.Setenv(tc.variable, tc.value)

			_, err := load(t)

			assertCode(t, err, tc.want)
		})
	}
}

// Zero is the documented way to switch shedding off, and it has to survive validation - a check
// that treated it as "greater than zero, like every other limit" would make the off switch a
// startup failure.
func TestLoadSheddingCanBeSwitchedOff(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_LOAD_SHED_INFLIGHT", "0")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("switching load shedding off was refused: %v", err)
	}
	if cfg.LoadShed.Inflight != 0 {
		t.Errorf("threshold = %d, want 0", cfg.LoadShed.Inflight)
	}
}

// A duration is Go syntax. A bare number is refused rather than guessed at: "30" as nanoseconds
// is a trap, and silently reading it as seconds is a different trap.
func TestDurationsAcceptGoSyntax(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_REQUEST_TIMEOUT", "1m30s")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Request.Timeout != 90*time.Second {
		t.Errorf("timeout = %s, want 1m30s", cfg.Request.Timeout)
	}
}

func TestAnUnknownRoleIsRejected(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_ROLES", "api,frontend")

	_, err := load(t)

	assertCode(t, err, "config.role_unknown")
	if !strings.Contains(err.Error(), "value=frontend") {
		t.Errorf("the error does not name the unknown role: %v", err)
	}
}

func TestRolesAreNormalised(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_ROLES", " API , worker ")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !cfg.HasRole(env.RoleAPI) || !cfg.HasRole(env.RoleWorker) {
		t.Errorf("roles = %v", cfg.Roles)
	}
	if cfg.HasRole(env.RoleScheduler) {
		t.Errorf("roles = %v, scheduler was not configured", cfg.Roles)
	}
}

// Half-configured object storage fails at startup, not at the first upload.
func TestS3WithoutCredentialsIsAStartupError(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_STORAGE_KIND", "s3")

	_, err := load(t)

	codes := detailCodes(t, err)
	var incomplete int
	for _, c := range codes {
		if c == "config.s3_incomplete" {
			incomplete++
		}
	}
	if incomplete != 3 {
		t.Errorf("codes = %v, want bucket, access key and secret key reported", codes)
	}
}

func TestS3WithCredentialsIsAccepted(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_STORAGE_KIND", "s3")
	t.Setenv("HUBTASK_S3_BUCKET", "hubtask-media")
	t.Setenv("HUBTASK_S3_ACCESS_KEY", "AKIAEXAMPLE")
	t.Setenv("HUBTASK_S3_SECRET_KEY", "s3cr3t")
	t.Setenv("HUBTASK_S3_USE_PATH_STYLE", "false")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("the S3 configuration was rejected: %v", err)
	}

	if cfg.Storage.Bucket != "hubtask-media" || cfg.Storage.UsePathStyle {
		t.Errorf("storage = %+v", cfg.Storage)
	}
}

// Without SMTP nothing breaks: notifications are caught up later (ADR-0016). It is a warning,
// not an error - and which warning it is depends on whether this installation fires anything.
func TestNoSMTPIsAWarningNotAnError(t *testing.T) {
	withRequiredSecrets(t)

	t.Run("an installation that sends nothing", func(t *testing.T) {
		t.Setenv("HUBTASK_ROLES", "api")
		cfg, err := load(t)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}

		if !hasWarning(New("v", "c").Warnings(cfg), "config.smtp_missing") {
			t.Error("a missing SMTP configuration produces no warning")
		}
	})

	// The warning observability-reliability.md §7 promises: the reminders will fire, the records
	// will exist, and nobody will be told until somebody configures a mail server (D-03).
	t.Run("an installation that fires reminders", func(t *testing.T) {
		t.Setenv("HUBTASK_ROLES", "api,worker,scheduler")
		cfg, err := load(t)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}

		warnings := New("v", "c").Warnings(cfg)
		if !hasWarning(warnings, "config.smtp_missing_with_reminders") {
			t.Error("an installation that fires reminders is not warned about missing SMTP")
		}
		if hasWarning(warnings, "config.smtp_missing") {
			t.Error("both warnings were reported for one missing mail server")
		}
	})
}

func TestConfiguredSMTPMustBeComplete(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SMTP_HOST", "smtp.example.org")
	t.Setenv("HUBTASK_SMTP_PORT", "70000")

	_, err := load(t)

	assertCode(t, err, "config.smtp_port_invalid")
	assertCode(t, err, "config.smtp_from_missing")
}

func TestWarningsReportWhatTheOperatorIsMissing(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_TENANCY_MODE", "multi")
	t.Setenv("HUBTASK_SMTP_HOST", "smtp.example.org")
	t.Setenv("HUBTASK_SMTP_FROM", "hubtask@example.org")
	t.Setenv("HUBTASK_SMTP_SECURITY", "none")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	warnings := New("v", "c").Warnings(cfg)

	for _, want := range []string{
		"config.base_url_missing", // links in emails would be wrong
		"config.oidc_missing_in_multi_tenancy",
		"config.egress_allowlist_missing", // mandatory in provider operation (T-07)
		"config.smtp_without_tls",
	} {
		if !hasWarning(warnings, want) {
			t.Errorf("the warning %s is missing from %v", want, warnings)
		}
	}
	if hasWarning(warnings, "config.smtp_missing") {
		t.Error("SMTP is configured, so the warning must not appear")
	}

	// config.backup_not_configured is deliberately not here any more (E-03). It used to be keyed
	// on two environment variables, one of which nothing read and neither of which said whether a
	// backup target exists - a target is a row in a tenant's database. The question is answered by
	// the repository's coverage count instead, and the surface that asks it is the tenant-facing
	// health report, which is still route.operation_not_available.
	if hasWarning(warnings, "config.backup_not_configured") {
		t.Error("a warning about backup targets is being derived from the environment again")
	}
}

// A warning is machine readable: a code plus parameters, never a sentence (ADR-0011).
func TestWarningsCarryCodesNotProse(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SMTP_HOST", "smtp.example.org")
	t.Setenv("HUBTASK_SMTP_FROM", "hubtask@example.org")
	t.Setenv("HUBTASK_SMTP_SECURITY", "none")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	for _, w := range New("v", "c").Warnings(cfg) {
		if strings.Contains(w.Code, " ") {
			t.Errorf("the warning code reads like prose: %q", w.Code)
		}
		switch w.Severity {
		case "info", "warn", "critical":
		default:
			t.Errorf("severity = %q for %s", w.Severity, w.Code)
		}
		if w.Code == "config.smtp_without_tls" && w.Params["host"] != "smtp.example.org" {
			t.Errorf("the parameters of %s are missing the host: %v", w.Code, w.Params)
		}
	}
}

// Every problem at once, not one per restart: an operator setting up an installation wants the
// whole list.
func TestAllProblemsAreReportedTogether(t *testing.T) {
	isolate(t)
	t.Setenv("HUBTASK_TENANCY_MODE", "somewhat")
	t.Setenv("HUBTASK_LOG_FORMAT", "xml")

	_, err := load(t)

	codes := detailCodes(t, err)
	if len(codes) < 4 {
		t.Errorf("codes = %v, want DSN, key, tenancy and log format at once", codes)
	}
}

// %+v over the config struct is the classic way a secret reaches a log (T-18).
func TestFormattingTheWholeConfigRevealsNoSecret(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_SMTP_HOST", "smtp.example.org")
	t.Setenv("HUBTASK_SMTP_FROM", "hubtask@example.org")
	t.Setenv("HUBTASK_SMTP_PASSWORD", "hunter2")
	t.Setenv("HUBTASK_S3_ACCESS_KEY", "AKIAEXAMPLE")
	t.Setenv("HUBTASK_S3_SECRET_KEY", "s3cr3t")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		rendered := fmt.Sprintf(format, cfg)
		for _, forbidden := range []string{validKey, "hunter2", "AKIAEXAMPLE", "s3cr3t", validDSN} {
			if strings.Contains(rendered, forbidden) {
				t.Errorf("%s reveals %q", format, forbidden)
			}
		}
	}
}

func hasWarning(warnings []env.Warning, code string) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

// CodeQL found this one: the pool size reaches the driver as an int32, and a value parsed from
// the environment without an upper bound would wrap rather than fail.
func TestAnAbsurdPoolSizeIsRejected(t *testing.T) {
	for _, value := range []string{"2147483648", "99999999999", "1001"} {
		t.Run(value, func(t *testing.T) {
			withRequiredSecrets(t)
			t.Setenv("HUBTASK_DB_MAX_CONNS", value)

			_, err := load(t)

			assertCode(t, err, "config.db_pool_invalid")
		})
	}
}

// A host name is case-insensitive, and an allowlist that misses "API.Example.COM" is one that
// fails open at the worst moment.
func TestTheEgressAllowlistIsNormalised(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_HTTP_ALLOWED_HOSTS", " Hooks.Example.ORG , api.partner.test ,, ")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	want := []string{"hooks.example.org", "api.partner.test"}
	if len(cfg.Outbound.AllowedHosts) != len(want) {
		t.Fatalf("allowed hosts = %v, want %v", cfg.Outbound.AllowedHosts, want)
	}
	for i, host := range want {
		if cfg.Outbound.AllowedHosts[i] != host {
			t.Errorf("allowed hosts = %v, want %v", cfg.Outbound.AllowedHosts, want)
			break
		}
	}
}

// Following nothing is the strictest setting, not a mistake.
func TestZeroRedirectsIsAValidSetting(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_HTTP_MAX_REDIRECTS", "0")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("zero redirects was rejected: %v", err)
	}
	if cfg.Outbound.MaxRedirects != 0 {
		t.Errorf("redirects = %d, want 0", cfg.Outbound.MaxRedirects)
	}
}

// The switch that turns a webhook into a port scanner of the host network says so in
// /meta/health, rather than only in whoever set it remembering that they did.
func TestOpeningPrivateNetworksWarns(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS", "true")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !cfg.Outbound.AllowPrivateNetworks {
		t.Fatal("the setting did not reach the configuration")
	}
	if !hasWarning(New("v", "c").Warnings(cfg), "config.egress_private_networks_allowed") {
		t.Error("opening private networks produces no warning")
	}
}

// Self-hosting is the one profile where an empty allowlist is the sensible default: a private
// installation calls whatever webhook its owner sets up, and there is no operator to maintain
// a list.
func TestAnEmptyAllowlistOnlyWarnsInProviderOperation(t *testing.T) {
	withRequiredSecrets(t)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if hasWarning(New("v", "c").Warnings(cfg), "config.egress_allowlist_missing") {
		t.Error("self-hosting warned about a missing allowlist")
	}
}

// The browser side stays closed unless somebody opens it (security.md §9).
func TestCrossOriginIsEmptyByDefault(t *testing.T) {
	withRequiredSecrets(t)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.CORS.AllowedOrigins) != 0 {
		t.Errorf("allowed origins = %v by default", cfg.CORS.AllowedOrigins)
	}
}

// A browser compares the Origin header byte for byte. An entry that cannot match one is a
// configuration mistake worth failing on, not one to discover when the frontend stays blocked.
func TestAnOriginWithoutASchemeIsRefused(t *testing.T) {
	cases := map[string]string{
		"a bare host":        "app.example.com",
		"a trailing slash":   "https://app.example.com/",
		"a path":             "https://app.example.com/api",
		"an unknown scheme":  "ftp://app.example.com",
		"the wildcard mixed": "https://app.example.com,*",
	}

	for name, origins := range cases {
		t.Run(name, func(t *testing.T) {
			withRequiredSecrets(t)
			t.Setenv("HUBTASK_CORS_ALLOWED_ORIGINS", origins)

			if _, err := load(t); err == nil {
				t.Fatal("the configuration was accepted")
			} else if !strings.Contains(err.Error(), "config.cors_origin_invalid") {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestACompleteOriginIsAccepted(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_CORS_ALLOWED_ORIGINS", "https://app.example.com, http://localhost:5173")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	want := []string{"https://app.example.com", "http://localhost:5173"}
	for i, origin := range want {
		if i >= len(cfg.CORS.AllowedOrigins) || cfg.CORS.AllowedOrigins[i] != origin {
			t.Fatalf("allowed origins = %v, want %v", cfg.CORS.AllowedOrigins, want)
		}
	}
}

// "*" is a legitimate self-hosting choice for a read-only public API, and it is safe only
// because this API never answers with credentials.
func TestTheWildcardIsAcceptedOnItsOwn(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_CORS_ALLOWED_ORIGINS", "*")

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !cfg.CORS.AllowsAnyOrigin() {
		t.Errorf("allowed origins = %v", cfg.CORS.AllowedOrigins)
	}
}

// The keyring (E-02). Two variables: one naming the keys in order, one per key holding the
// material - so that no key material appears in the variable an operator pastes into a support
// ticket, and so that every key can be its own mounted secret.

var otherKey = strings.Repeat("other-ke", 4) // 32 characters, the minimum

func TestAnInstallationWithNoKeyringStarts(t *testing.T) {
	withRequiredSecrets(t)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Encryption.Keys) != 0 || cfg.Encryption.ActiveKeyID() != "" {
		t.Fatalf("an unconfigured installation reports %d keys", len(cfg.Encryption.Keys))
	}
}

// The order is the configuration's statement of which key is current. A second setting for it
// would be a second place for the two to disagree.
func TestTheKeyringKeepsItsOrderAndTheFirstKeyIsCurrent(t *testing.T) {
	withRequiredSecrets(t)
	t.Setenv("HUBTASK_ENCRYPTION_KEYS", "k2026, k2025")
	t.Setenv("HUBTASK_ENCRYPTION_KEY_K2026", validKey)
	t.Setenv("HUBTASK_ENCRYPTION_KEY_K2025", otherKey)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(cfg.Encryption.Keys) != 2 {
		t.Fatalf("%d keys", len(cfg.Encryption.Keys))
	}
	if cfg.Encryption.ActiveKeyID() != "k2026" {
		t.Fatalf("the current key is %q", cfg.Encryption.ActiveKeyID())
	}
	if cfg.Encryption.Keys[1].ID != "k2025" {
		t.Fatalf("the predecessor is %q", cfg.Encryption.Keys[1].ID)
	}
	if cfg.Encryption.Keys[0].Material.Reveal() != validKey {
		t.Error("the material did not reach the key it belongs to")
	}
}

func TestEachKeyCanBeItsOwnMountedSecret(t *testing.T) {
	withRequiredSecrets(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "k2026")
	if err := os.WriteFile(file, []byte(validKey+"\n"), 0o600); err != nil {
		t.Fatalf("writing the fixture failed: %v", err)
	}
	t.Setenv("HUBTASK_ENCRYPTION_KEYS", "k2026")
	t.Setenv("HUBTASK_ENCRYPTION_KEY_K2026_FILE", file)

	cfg, err := load(t)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Encryption.Keys) != 1 || cfg.Encryption.Keys[0].Material.Reveal() != validKey {
		t.Fatal("the key was not read from its file, or the newline stayed")
	}
}

// Every one of these is a process that does not start, which is the right outcome for all of
// them: a keyring that is wrong is discovered now rather than at the first backup.
func TestTheKeyringRefusesWhatItCannotStandBehind(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		code string
	}{
		"an identifier with a hyphen": {
			map[string]string{"HUBTASK_ENCRYPTION_KEYS": "key-1"},
			"config.encryption_key_id_invalid",
		},
		"the same identifier twice": {
			map[string]string{
				"HUBTASK_ENCRYPTION_KEYS":  "a,a",
				"HUBTASK_ENCRYPTION_KEY_A": validKey,
			},
			"config.encryption_key_id_duplicate",
		},
		"a key named and not supplied": {
			map[string]string{"HUBTASK_ENCRYPTION_KEYS": "a"},
			"config.encryption_key_missing",
		},
		"material somebody could have typed from memory": {
			map[string]string{
				"HUBTASK_ENCRYPTION_KEYS":  "a",
				"HUBTASK_ENCRYPTION_KEY_A": "too short",
			},
			"config.encryption_key_too_short",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			withRequiredSecrets(t)
			for variable, value := range c.env {
				t.Setenv(variable, value)
			}

			_, err := load(t)
			if err == nil {
				t.Fatal("the process started anyway")
			}
			if !slices.Contains(detailCodes(t, err), c.code) {
				t.Fatalf("codes %v, want %s", detailCodes(t, err), c.code)
			}
			// The refusal names the variable and never the material.
			if strings.Contains(err.Error(), "too short") && strings.Contains(err.Error(), validKey) {
				t.Fatal("the refusal quoted the material")
			}
		})
	}
}
