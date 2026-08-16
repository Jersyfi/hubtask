// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package observability holds the adapters for logging, metrics and tracing.
//
// This file is the logger. Structured JSON in operation, text for a developer's terminal, and a
// redaction layer that no caller has to remember (threat T-18): the log is written by many hands,
// including hands that pass an error along without reading it, and an error is where a connection
// string travels.
package observability

import (
	"context"
	"io"
	"log/slog"
	"regexp"
	"strings"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Redacted is what replaces a value that must not be readable. A fixed marker rather than an
// empty string, so that the log still shows the field was present.
const Redacted = "[REDACTED]"

// NewLogger builds the process logger from the configuration.
//
// Trace correlation (trace_id, span_id) is added in task A-04, once OpenTelemetry exists.
func NewLogger(cfg env.Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}

	var handler slog.Handler
	if strings.EqualFold(cfg.LogFormat, "text") {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(NewRedactingHandler(handler)).With(
		slog.String("service", "hubtask"),
		slog.String("version", cfg.Version),
	)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewRedactingHandler wraps a handler so that every attribute passes the redaction on its way
// through - including the attributes of With and WithGroup, which are applied once and then
// reused for the lifetime of a logger.
func NewRedactingHandler(inner slog.Handler) slog.Handler {
	return redactingHandler{inner: inner}
}

type redactingHandler struct {
	inner slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, scrub(record.Message), record.PC)
	record.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redact(a))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		clean = append(clean, redact(a))
	}
	return redactingHandler{inner: h.inner.WithAttrs(clean)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{inner: h.inner.WithGroup(name)}
}

// sensitiveKeyParts are matched as substrings of the lower-cased attribute key. "secret" also
// covers secret_key and client_secret; "key" alone is deliberately absent, because it would
// swallow idempotency_key and order_key without either being a secret.
var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie",
	"credential", "dsn", "api_key", "apikey", "access_key", "private_key",
	"signing_key", "encryption_key", "session_id", "bearer", "passphrase",
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func redact(a slog.Attr) slog.Attr {
	// Resolve first: a LogValuer decides for itself what it shows, which is how secret.Secret
	// masks itself. Without resolving, the value below would be the wrapper.
	a.Value = a.Value.Resolve()

	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		clean := make([]slog.Attr, 0, len(attrs))
		for _, inner := range attrs {
			clean = append(clean, redact(inner))
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(clean...)}
	}

	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}

	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, scrub(a.Value.String()))
	case slog.KindAny:
		// An error is the usual carrier: a driver error contains the connection string it
		// failed to dial, complete with the password.
		if err, ok := a.Value.Any().(error); ok && err != nil {
			return slog.String(a.Key, scrub(err.Error()))
		}
		if s, ok := a.Value.Any().(string); ok {
			return slog.String(a.Key, scrub(s))
		}
	}
	return a
}

var (
	// A URL carrying credentials: postgres://user:password@host/db. The password is what goes,
	// the rest stays readable - a DSN without its host is useless in a log.
	urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^\s:/@]+):([^\s@]+)@`)
	// Authorization header values, in the two forms that actually appear.
	authScheme = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._\-+/=]+`)
	// Personal access tokens carry their own prefix (api-guidelines.md §7), which makes them
	// recognisable even in a body someone logged.
	personalAccessToken = regexp.MustCompile(`hbt_pat_[A-Za-z0-9._\-]+`)
)

// scrub removes credentials from free text. It is the second line of defence: the first is that
// a secret is a secret.Secret and masks itself. This one catches what arrives as a string -
// error messages above all.
func scrub(s string) string {
	if s == "" {
		return s
	}
	s = urlCredentials.ReplaceAllString(s, "$1:"+Redacted+"@")
	s = authScheme.ReplaceAllString(s, "$1 "+Redacted)
	s = personalAccessToken.ReplaceAllString(s, "hbt_pat_"+Redacted)
	return s
}
