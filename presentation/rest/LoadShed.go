// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// DeferrableRoutes are the operations that may be refused while the process is busy: the bulk,
// export and search shapes observability-reliability.md §6 names by class, plus the item query,
// which is the heaviest read the contract has (H-11).
//
// Everything absent from this list is interactive and is never shed. That direction is the safe
// one, and it is the reason the list is written out rather than derived: a person who cannot tick
// off a task retries by hand, which adds load instead of removing it, so a route wrongly listed
// here costs more than a route wrongly missing from it. What keeps it honest is the contract
// test - every entry has to be a route the router serves, or a typo would be a line that silently
// never matches.
var DeferrableRoutes = map[string]bool{
	// Five hundred operations in one call, by the contract's own bound (C-11).
	http.MethodPost + " " + APIBasePath + "/items:bulk": true,
	// The query DSL: filters and sorts the interactive list endpoints do not offer, over the
	// whole of a tenant's items (ADR-0026).
	http.MethodPost + " " + APIBasePath + "/items:query": true,
	http.MethodPost + " " + APIBasePath + "/search":      true,
	// The three exports. Each one reads far more than it answers with, and each one has a caller
	// that is a job rather than a person waiting.
	http.MethodPost + " " + APIBasePath + "/views/{viewId}:export":           true,
	http.MethodPost + " " + APIBasePath + "/audit:export":                    true,
	http.MethodPost + " " + APIBasePath + "/admin/tenants/{tenantId}:export": true,
}

// Shedder decides whether one call may run, and is what the composition root closes over.
//
// A function rather than the load shedder itself: the shedder is infrastructure and an inbound
// adapter does not reach across to the outbound side (project-structure.md §2). The contract is
// that release is never nil - including on a refusal, so that a caller may defer it
// unconditionally.
type Shedder func(deferrable bool) (release func(), err error)

// Shedding is admission control in front of the API: above a threshold on the requests in
// flight, deferrable work is refused with 503 and a Retry-After before latency tips over for
// everyone (observability-reliability.md §6, RT-6).
//
// It sits above authentication and above the rate limits, and below the body bound. Above,
// because a refusal must cost nothing - a shed request that had already spent a database lookup
// resolving its actor would be load rather than the absence of it. Below, because an oversized
// body is a cheaper refusal still and belongs first.
type Shedding struct {
	Next http.Handler
	// Routes resolves the route template, which is what decides the class. The template rather
	// than the path: the class is a property of the operation, and two items are the same
	// operation.
	Routes Router
	// Admit is nil when shedding is switched off, and then this middleware is a pass-through
	// rather than a threshold nobody can reach.
	Admit Shedder
	// Signals counts the refusals, under the same instrument the rate limits use and with the
	// class in the scope - `load_shed:deferrable`. One counter for every way a call is turned
	// away, because the question an operator asks under load is how much is being refused, and
	// an answer split across instruments is one nobody assembles. Optional; nil records nothing.
	Signals LimitSignals
}

func (s Shedding) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.Admit == nil {
		s.Next.ServeHTTP(w, r)
		return
	}

	_, route := s.Routes.Handler(r)
	release, err := s.Admit(DeferrableRoutes[route])
	if release != nil {
		defer release()
	}
	if err != nil {
		if s.Signals != nil {
			s.Signals.RateLimited(r.Context(), shedScope(DeferrableRoutes[route]))
		}
		if seconds := retryAfterOf(err); seconds != "" {
			w.Header().Set("Retry-After", seconds)
		}
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	s.Next.ServeHTTP(w, r)
}

// shedScope labels the counter. The class rather than the route: the route is already a label on
// the request metrics, and putting it here as well would multiply the series for an answer nobody
// needs twice.
func shedScope(deferrable bool) string {
	if deferrable {
		return "load_shed:deferrable"
	}
	return "load_shed:interactive"
}

// retryAfterOf reads the wait out of the refusal. The shedder puts it in a parameter rather than
// in a header, because the domain does not know what a header is (ADR-0011); turning it into one
// is this layer's job (api-guidelines.md §6).
func retryAfterOf(err error) string {
	var refusal *shared.Error
	if !errors.As(err, &refusal) {
		return ""
	}
	seconds, known := refusal.Params["retry_after_seconds"]
	if !known {
		return ""
	}
	// Parsed rather than passed through: the value ends up in a response header, and a header
	// assembled from a map deserves the same distrust as one assembled from a request.
	if _, convErr := strconv.Atoi(seconds); convErr != nil {
		return ""
	}
	return seconds
}
