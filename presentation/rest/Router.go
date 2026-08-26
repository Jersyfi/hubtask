// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// APIBasePath is the one major path of the API (api-guidelines.md §2). The specification's paths
// are relative to it, so it is prepended once when the generated routes are registered rather
// than repeated in every path of the contract.
const APIBasePath = "/api/v1"

// Mux is the router the generated code registers its routes on.
//
// It exists for one reason: the specification writes an action as a suffix of the resource
// (`POST /items/{itemId}:complete`, Google AIP style, api-guidelines.md §2), and net/http's
// router requires a wildcard to span a whole path segment - `{itemId}:complete` makes it panic
// at registration. So the colon gets a segment of its own on the way in, and requests are
// rewritten the same way on the way out. The contract keeps its form, the router gets one it can
// parse, and no third-party router enters the dependency list for a one-character difference.
//
// It also answers the two conditions the router itself decides - no such route, and not this
// method - as RFC 9457 problems rather than as net/http's plain text (security.md §9).
type Mux struct {
	mux *http.ServeMux
	// templates maps a registered pattern back to the one the specification declares. It is what
	// the metric label and the span name are built from: the rewritten form is an implementation
	// detail and must not leak into a dashboard (observability-reliability.md §3.2).
	templates map[string]string
	// order keeps the registration sequence, so that the contract test can compare the router
	// against the specification without depending on map iteration.
	order []string
}

func NewMux() *Mux {
	return &Mux{mux: http.NewServeMux(), templates: map[string]string{}}
}

// HandleFunc implements the ServeMux interface of the generated code, which is the seam that lets
// this rewrite happen without touching a generated line (CLAUDE.md rule 11).
func (m *Mux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	routable := routablePattern(pattern)
	if _, exists := m.templates[routable]; !exists {
		m.order = append(m.order, pattern)
	}
	m.templates[routable] = pattern
	m.mux.HandleFunc(routable, handler)
}

// Routes returns the route templates as the specification writes them, in registration order.
func (m *Mux) Routes() []string { return append([]string(nil), m.order...) }

// Handler resolves a request to its handler and to the specification's route template, without
// serving. It is what the observability middleware asks before dispatch.
func (m *Mux) Handler(r *http.Request) (http.Handler, string) {
	handler, pattern := m.mux.Handler(routableRequest(r))
	return handler, m.templates[pattern]
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	routed := routableRequest(r)

	// Asked first, served second - and served through the ServeMux rather than through the
	// handler it just returned. Handler() resolves without binding, so a handler invoked
	// directly would find every r.PathValue empty.
	if _, pattern := m.mux.Handler(routed); pattern == "" {
		// Unmatched: net/http has an answer, but not one in this API's format.
		m.serveUnrouted(w, routed)
		return
	}
	m.mux.ServeHTTP(w, routed)
}

// serveUnrouted converts net/http's own answer into a problem document.
//
// The built-in handler is asked rather than second-guessed, because it knows the two things worth
// keeping: whether the path matched nothing at all or only the wrong method, and which methods
// the route does serve. Its body is discarded - "404 page not found" is display text, and the
// backend produces none (ADR-0011).
func (m *Mux) serveUnrouted(w http.ResponseWriter, r *http.Request) {
	probe := &headerProbe{headers: http.Header{}, status: http.StatusNotFound}
	m.mux.ServeHTTP(probe, r)

	requestID := correlation.RequestIDFrom(r.Context())
	if probe.status == http.StatusMethodNotAllowed {
		// Allow is what makes a 405 useful to a client, and it is the only header worth carrying
		// over: everything else the probe collected is net/http describing a body we discarded.
		if allow := probe.headers.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		WriteMethodNotAllowed(w, requestID)
		return
	}
	WriteProblem(w, shared.ErrNotFound.WithDetail("route.unknown"), requestID)
}

// headerProbe is a response writer that keeps the status and the headers and throws the body
// away.
type headerProbe struct {
	headers http.Header
	status  int
}

func (p *headerProbe) Header() http.Header         { return p.headers }
func (p *headerProbe) WriteHeader(status int)      { p.status = status }
func (p *headerProbe) Write(b []byte) (int, error) { return len(b), nil }

// routablePattern rewrites "POST /api/v1/items/{itemId}:complete" into a pattern net/http can
// parse.
func routablePattern(pattern string) string {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		return routablePath(pattern)
	}
	return method + " " + routablePath(path)
}

// routablePath applies both rewrites. They never meet on one path - an action is a colon suffix
// and an extension is a file one - but applying both in one place means a route only has to be
// registered once, whichever it carries.
func routablePath(path string) string { return extensionSegment(actionSegment(path)) }

// routableRequest applies the same rewrite to an incoming request. A request that carries no
// action is passed through untouched, so the rewrite can only ever affect an action call.
func routableRequest(r *http.Request) *http.Request {
	rewritten := routablePath(r.URL.Path)
	if rewritten == r.URL.Path {
		return r
	}

	url := *r.URL
	url.Path = rewritten
	// RawPath is dropped deliberately: the escaped form is derived from Path again, and the only
	// character this rewrite inserts is a separator. The effect is that a client which
	// percent-encoded the colon reaches the same route as one that did not.
	url.RawPath = ""

	clone := *r
	clone.URL = &url
	return &clone
}

// actionSegment gives an action suffix a path segment of its own: `/items/{itemId}:complete`
// becomes `/items/{itemId}/:complete`.
//
// Only the last segment is considered, because that is where the contract puts an action. The
// colon is kept rather than replaced, and a segment beginning with one cannot be produced by any
// identifier - so an action can never collide with a resource path.
func actionSegment(path string) string {
	slash := strings.LastIndexByte(path, '/')
	last := path[slash+1:]

	colon := strings.IndexByte(last, ':')
	if colon <= 0 {
		// No action, or a path already in the rewritten form.
		return path
	}
	return path[:slash+1] + last[:colon] + "/" + last[colon:]
}

// icsExtension is the one file extension the contract uses. A calendar client stores a URL and
// decides from its ending what it is looking at, which is why /calendar/{token}.ics is written
// that way in api-guidelines.md §2 rather than as a plain resource.
const icsExtension = ".ics"

// extensionSegment gives that extension a path segment of its own: `/calendar/{token}.ics`
// becomes `/calendar/{token}/.ics`, for the reason actionSegment exists - net/http's router
// requires a wildcard to span a whole segment, and `{token}.ics` makes it panic at registration.
//
// Only this one extension, and only at the end: a segment beginning with a dot cannot be produced
// by any identifier the API mints, so the rewritten form can never collide with a real path, and
// a rewrite that fired on any dot would catch tokens and identifiers that legitimately contain
// one.
func extensionSegment(path string) string {
	if !strings.HasSuffix(path, icsExtension) {
		return path
	}
	slash := strings.LastIndexByte(path, '/')
	last := path[slash+1:]
	if last == icsExtension {
		// Already rewritten, or a request for the extension alone - which matches nothing.
		return path
	}
	return path[:slash+1] + strings.TrimSuffix(last, icsExtension) + "/" + icsExtension
}

// Mounted serves one path that the specification does not describe, and leaves everything else to
// the router that it does.
//
// There is exactly one such path today: /mcp. MCP is not REST - it is JSON-RPC over one endpoint
// (ai-first.md §1.1) - so it has no place in api/openapi.yaml, and registering it on the Mux
// would put a route into the contract test that the contract does not contain. Mounting it here
// keeps both true: the specification still describes every REST route, and the MCP endpoint still
// travels through the same middleware, so it is authenticated, rate limited and observed like
// everything else.
type Mounted struct {
	// Router is the specification's routes.
	Router Router
	// Path is the mounted path, matched exactly.
	Path string
	// Mount answers it.
	Mount http.Handler
}

var _ Router = Mounted{}

// Handler resolves the request, returning the mounted path as its own route template so that a
// metric label and a span name exist for it too (observability-reliability.md §3.2).
func (m Mounted) Handler(r *http.Request) (http.Handler, string) {
	if r.URL.Path == m.Path {
		return m.Mount, m.Path
	}
	return m.Router.Handler(r)
}

func (m Mounted) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == m.Path {
		m.Mount.ServeHTTP(w, r)
		return
	}
	m.Router.ServeHTTP(w, r)
}
