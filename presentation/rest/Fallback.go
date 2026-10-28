// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"log/slog"
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
	// Browsers names where a browser that opens a page on a host beside its workspace's main
	// address is sent (UC-INS-15 check 7). Nil sends nobody on.
	Browsers BrowserRedirect
}

// BrowserRedirect answers the main address's base for a host a browser should leave, or empty.
type BrowserRedirect interface {
	RedirectFor(ctx context.Context, host string) (string, error)
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
		if f.sendOn(w, r) {
			return
		}
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

// sendOn redirects a page load on a host beside the main address to the same path there. Only a
// page load: the API keeps answering on both hosts, and the assets a page loads come from wherever
// the page is, so asking for each would be a lookup per file. Temporary, because switching back
// reverses it; a lookup that fails serves the page where it was asked for.
func (f Fallback) sendOn(w http.ResponseWriter, r *http.Request) bool {
	if f.Browsers == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) || !isPageLoad(r) {
		return false
	}
	base, err := f.Browsers.RedirectFor(r.Context(), r.Host)
	if err != nil {
		slog.WarnContext(r.Context(), "where a browser is sent could not be read",
			slog.String("code", shared.AsError(err).DetailCode))
		return false
	}
	if base == "" {
		return false
	}
	WriteSecurityHeaders(w.Header(), contentSecurityPolicy)
	// The host is the workspace's main row, never the request's; the request gives only the path.
	http.Redirect(w, r, base+r.URL.RequestURI(), http.StatusFound) //nolint:gosec // G710: the target host is read from tenant_host
	return true
}

// isPageLoad is a navigation by its fetch metadata, or - for a browser that sends none - a request
// that accepts HTML.
func isPageLoad(r *http.Request) bool {
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
