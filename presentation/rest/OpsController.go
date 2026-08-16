// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package rest contains the inbound HTTP adapters.
//
// This file serves only the operations endpoints on the internal port. Business endpoints are
// generated from api/openapi.yaml (ADR-0004) and do not belong here.
package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

type OpsController struct {
	Health health.Registry
	// Metrics is the Prometheus endpoint. It lives here rather than on the public port
	// deliberately: the series say what runs, how much of it, and how slowly
	// (observability-reliability.md §3.2).
	Metrics http.Handler
}

func (c OpsController) Routes() http.Handler {
	mux := http.NewServeMux()

	// Liveness: the process only. No dependency check (ADR-0016).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		defer concurrency.Recover(r.Context(), "ops.healthz")
		if c.Health.Live() {
			writeText(w, http.StatusOK, "ok")
			return
		}
		writeText(w, http.StatusServiceUnavailable, "unavailable")
	})

	mux.HandleFunc("GET /startupz", func(w http.ResponseWriter, r *http.Request) {
		defer concurrency.Recover(r.Context(), "ops.startupz")
		if reg, ok := c.Health.(interface{ Started() bool }); ok && !reg.Started() {
			writeText(w, http.StatusServiceUnavailable, "starting")
			return
		}
		writeText(w, http.StatusOK, "ok")
	})

	// Readiness: mandatory dependencies and shutdown state. Responds with a status code and a
	// short reason only - details are available authenticated under /api/v1/meta/health, so
	// that this endpoint gives an attacker nothing to work with.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		defer concurrency.Recover(r.Context(), "ops.readyz")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if ok, reason := c.Health.Ready(ctx); ok {
			writeText(w, http.StatusOK, "ok")
		} else {
			writeText(w, http.StatusServiceUnavailable, reason)
		}
	})

	if c.Metrics != nil {
		mux.Handle("GET /metrics", c.Metrics)
	}

	// The deep self-diagnosis. The document puts it at /api/v1/meta/health behind an admin
	// scope; authentication and the generated router arrive with A-06, so until then it is
	// served here - on the internal port, which is not public either way.
	mux.HandleFunc("GET /meta/health", func(w http.ResponseWriter, r *http.Request) {
		defer concurrency.Recover(r.Context(), "ops.meta_health")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		report := c.Health.Report(ctx)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if report.Status == health.StatusDown {
			// A report that says "down" answers 503, so a status page needs to read no JSON to
			// know (ADR-0016).
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(healthReportJSON(report)); err != nil {
			slog.WarnContext(ctx, "writing the health report failed", slog.String("error", err.Error()))
		}
	})

	return mux
}

func writeText(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}
