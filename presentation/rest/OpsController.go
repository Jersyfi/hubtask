// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package rest contains the inbound HTTP adapters.
//
// This file serves only the operations endpoints on the internal port. Business endpoints are
// generated from api/openapi.yaml (ADR-0004) and do not belong here.
package rest

import (
	"context"
	"net/http"
	"time"

	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

type OpsController struct {
	Health health.Registry
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

	return mux
}

func writeText(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(body))
}
