// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// UIRoute is the route template every user interface request is recorded under.
//
// One label for all of them, on purpose. A single-page application owns its own routes, so the
// paths a browser asks for are invented by the client and unbounded - using them would make the
// metric's cardinality a property of somebody else's router (observability-reliability.md §3.2).
// What is worth knowing here is how many requests the UI serves and how fast, not which of its
// screens a person opened.
const UIRoute = "GET /*"

// Fallback decides, for every request, whether the API owns the path or the user interface does.
//
// The rule is one sentence and the order in it is the whole point: the API keeps every path it
// owns, and the user interface gets what is left (ADR-0028). A route the specification declares
// can therefore never be shadowed by a file that happens to have the same name, and a request
// under /api/ that matches no route still gets the API's own problem document rather than an HTML
// page a client cannot parse.
//
// It sits directly under the observability wrapper and above everything else, which is what keeps
// the user interface out of the API's middleware: a static file needs no actor, no tenant and no
// idempotency key, and a page load that spent six requests of the anonymous rate limit budget
// would make the first visit the last one.
type Fallback struct {
	// API resolves the route template of a request the API owns. It is the router, not the chain.
	API Router
	// Reserved are the paths the API owns. An entry ending in "/" is a prefix; anything else is
	// matched exactly.
	Reserved []string
	// Serve is the API's middleware chain - authentication, limits, idempotency, and the router
	// at the end of it.
	Serve http.Handler
	// UI serves the embedded bundle. Nil when the interface is switched off, and then every path
	// the API does not own is answered 404 here.
	UI http.Handler
}

var _ Router = Fallback{}

func (f Fallback) Handler(r *http.Request) (http.Handler, string) {
	if f.apiOwns(r.URL.Path) || f.UI == nil {
		return f.API.Handler(r)
	}
	return f.UI, UIRoute
}

func (f Fallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.apiOwns(r.URL.Path) {
		f.Serve.ServeHTTP(w, r)
		return
	}
	if f.UI != nil {
		f.UI.ServeHTTP(w, r)
		return
	}

	// The interface is switched off. The answer is 404 and it is written here rather than passed
	// down the chain, because the chain would authenticate first and answer 401 to an anonymous
	// request for "/" - which would tell a visitor that there is something there to log in to.
	// HUBTASK_UI_ENABLED=false means there is not.
	WriteSecurityHeaders(w.Header(), contentSecurityPolicy)
	WriteProblem(w, shared.ErrNotFound.WithDetail("route.unknown"), correlation.RequestIDFrom(r.Context()))
}

func (f Fallback) apiOwns(path string) bool {
	for _, reserved := range f.Reserved {
		if strings.HasSuffix(reserved, "/") {
			// A prefix entry also owns the path without its trailing slash, so that "/api/v1"
			// cannot be answered with a document while "/api/v1/" is an API 404.
			if strings.HasPrefix(path, reserved) || path == strings.TrimSuffix(reserved, "/") {
				return true
			}
			continue
		}
		if path == reserved {
			return true
		}
	}
	return false
}
