// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const plaintext = "hunter2-super-secret"

func newTestLogger(t *testing.T, format string) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	cfg := env.Config{Version: "1.2.3", LogFormat: format, LogLevel: "debug"}
	return NewLogger(cfg, &buf), &buf
}

// The acceptance criterion of A-02, and threat T-18: the log contains no token.
func TestASecretValueNeverReachesTheLog(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			logger, buf := newTestLogger(t, format)

			logger.Info("connecting",
				slog.Any("value", secret.New(plaintext)),
				slog.Any("config", struct {
					DSN  secret.Secret
					Host string
				}{DSN: secret.New("postgres://u:" + plaintext + "@db:5432"), Host: "db"}),
			)

			assertNoPlaintext(t, buf.String())
			if !strings.Contains(buf.String(), "db") {
				t.Errorf("the surrounding, harmless information is gone too: %s", buf)
			}
		})
	}
}

func TestASensitiveKeyIsRedactedWhateverTheValue(t *testing.T) {
	keys := []string{
		"password", "user_password", "secret", "secret_key", "client_secret",
		"token", "refresh_token", "authorization", "Authorization", "cookie",
		"set-cookie", "credential", "db_dsn", "api_key", "apikey", "access_key",
		"private_key", "signing_key", "encryption_key", "session_id", "passphrase",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			logger, buf := newTestLogger(t, "json")

			logger.Info("event", slog.String(key, plaintext))

			assertNoPlaintext(t, buf.String())
			if !strings.Contains(buf.String(), Redacted) {
				t.Errorf("the value was dropped instead of marked: %s", buf)
			}
		})
	}
}

// Redaction that swallows harmless fields makes logs useless, and useless logs get switched off.
func TestOrdinaryKeysAreUntouched(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.Info("event",
		slog.String("tenant_id", "01J9TENANT"),
		slog.String("idempotency_key", "01J9IDEM"),
		slog.String("order_key", "0|hzzzzz:"),
		slog.String("request_id", "01J9REQ"),
		slog.Int("status", 200),
	)

	for _, want := range []string{"01J9TENANT", "01J9IDEM", "0|hzzzzz:", "01J9REQ", "200"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("%q was redacted although it is not a secret: %s", want, buf)
		}
	}
}

// The usual carrier: nobody logs a password on purpose, but plenty of code logs an error, and a
// driver error contains the connection string it failed to dial.
func TestCredentialsInsideAnErrorAreScrubbed(t *testing.T) {
	logger, buf := newTestLogger(t, "json")
	dialErr := fmt.Errorf("connecting: %w",
		errors.New(`dial postgres://hubtask:`+plaintext+`@db:5432/hubtask failed`))

	logger.Error("the database is unreachable", slog.Any("error", dialErr))

	assertNoPlaintext(t, buf.String())
	// The host stays: a scrubbed log still has to be diagnosable.
	if !strings.Contains(buf.String(), "db:5432") {
		t.Errorf("the scrubbing removed too much: %s", buf)
	}
}

func TestTokensInFreeTextAreScrubbed(t *testing.T) {
	cases := []struct {
		name  string
		value string
		keep  string
	}{
		{"bearer", "Authorization: Bearer eyJhbGciOi." + plaintext, "Bearer"},
		{"basic", "Authorization: Basic aHViOg==" + plaintext, "Basic"},
		{"personal access token", "hbt_pat_" + plaintext + " was rejected", "hbt_pat_"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, buf := newTestLogger(t, "json")

			// A neutral key, so that the scrubbing rather than the key match is under test.
			logger.Warn("rejected", slog.String("reason", tc.value))

			assertNoPlaintext(t, buf.String())
			if !strings.Contains(buf.String(), tc.keep) {
				t.Errorf("the scheme is gone, so the entry is unreadable: %s", buf)
			}
		})
	}
}

func TestTheMessageItselfIsScrubbed(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.Info("dialling postgres://hubtask:" + plaintext + "@db:5432")

	assertNoPlaintext(t, buf.String())
}

// With is applied once and reused for the life of the logger, so it takes its own path through
// the handler - and that path leaked in the first draft of this file.
func TestAttributesFromWithAreRedacted(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.With(slog.String("api_key", plaintext)).Info("event")

	assertNoPlaintext(t, buf.String())
}

func TestAttributesInsideAGroupAreRedacted(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.Info("event",
		slog.Group("smtp",
			slog.String("host", "smtp.example.org"),
			slog.String("password", plaintext),
			slog.Group("nested", slog.String("token", plaintext)),
		),
	)

	assertNoPlaintext(t, buf.String())
	if !strings.Contains(buf.String(), "smtp.example.org") {
		t.Errorf("the harmless field of the group is gone: %s", buf)
	}
}

func TestWithGroupRedactsToo(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.WithGroup("request").Info("event", slog.String("authorization", plaintext))

	assertNoPlaintext(t, buf.String())
}

// The whole configuration in one attribute is what an operator wants at startup, and the single
// most likely way to leak everything at once.
func TestLoggingTheWholeConfigurationRevealsNothing(t *testing.T) {
	cfg := env.Config{
		Version:   "1.2.3",
		LogFormat: "json",
		LogLevel:  "info",
		SecretKey: secret.New(plaintext),
		Database:  env.DatabaseConfig{DSN: secret.New("postgres://u:" + plaintext + "@db/hubtask")},
		Mail:      env.MailConfig{Host: "smtp.example.org", Password: secret.New(plaintext)},
		Storage: env.StorageConfig{
			Bucket:    "media",
			AccessKey: secret.New(plaintext),
			SecretKey: secret.New(plaintext),
		},
	}

	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			cfg.LogFormat = format
			NewLogger(cfg, &buf).Info("starting", slog.Any("config", cfg))

			assertNoPlaintext(t, buf.String())
		})
	}
}

func TestTheLevelComesFromTheConfiguration(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(env.Config{LogFormat: "json", LogLevel: "warn"}, &buf)

	logger.Info("not interesting")
	logger.Warn("interesting")

	if strings.Contains(buf.String(), "not interesting") {
		t.Errorf("the level was ignored: %s", &buf)
	}
	if !strings.Contains(buf.String(), "interesting") {
		t.Errorf("the entry above the level is missing: %s", &buf)
	}
}

func TestJSONOutputIsValidJSONWithTheServiceFields(t *testing.T) {
	logger, buf := newTestLogger(t, "json")

	logger.Info("started")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("the output is not valid JSON: %v (%s)", err, buf)
	}
	if entry["service"] != "hubtask" || entry["version"] != "1.2.3" {
		t.Errorf("the service fields are missing: %v", entry)
	}
	if entry["msg"] != "started" {
		t.Errorf("msg = %v", entry["msg"])
	}
}

func assertNoPlaintext(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, plaintext) {
		t.Errorf("the log contains the secret: %s", output)
	}
}
