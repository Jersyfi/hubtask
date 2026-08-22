// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"slices"
	"strconv"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The response headers of security.md §9. They are constants rather than configuration: an
// installation that can weaken them is one that will, and none of them costs anything on a JSON
// API.
const (
	// hstsValue is sent unconditionally. A browser ignores it on a plain-HTTP origin, so it
	// cannot lock out a self-hoster on http://localhost, and it takes effect the moment a proxy
	// terminates TLS in front of the process - which is the deployment the header is for.
	hstsValue = "max-age=31536000; includeSubDomains"

	// contentSecurityPolicy is written for an API that answers JSON and nothing else. If a body
	// ever did reach a browser as HTML, this would leave it with no way to load or send anything.
	contentSecurityPolicy = "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

	// permissionsPolicy switches off the device APIs. Minimal by the same argument as the CSP:
	// this origin serves data, so every one of them is a mistake if it is ever asked for.
	permissionsPolicy = "accelerometer=(), camera=(), geolocation=(), gyroscope=(), " +
		"magnetometer=(), microphone=(), payment=(), usb=()"

	// serverName carries no version. The version is available under /meta, authenticated
	// (security.md §9, "version disclosure").
	serverName = "Hubtask"
)

// corsAllowedHeaders is what a browser may send. Every entry is a header this API reads;
// anything else is refused by the preflight rather than silently ignored.
var corsAllowedHeaders = "Authorization, Content-Type, Accept, Accept-Language, " +
	"Idempotency-Key, If-Match, If-None-Match, " + RequestIDHeader + ", " + TenantHeader

// corsExposedHeaders is what a browser may read back. Without this list a cross-origin client
// cannot see its own request ID, its rate limit budget, or an ETag it is expected to send back.
var corsExposedHeaders = RequestIDHeader + ", ETag, Retry-After, " +
	"RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset, Deprecation, Sunset"

// TenantHeader is the third source of tenant resolution (multi-tenancy.md §3). It never wins over
// the token: a contradiction between the two is refused, not resolved.
const TenantHeader = "X-Hubtask-Tenant"

// Secured adds the response headers of security.md §9 and answers CORS preflights.
//
// It is the outermost middleware after the observability wrapper, because the headers have to be
// on every answer - including the ones no handler produced, such as a 404 from the router or a
// 429 from the rate limiter.
type Secured struct {
	Next http.Handler
	CORS env.CORSConfig
}

// WriteSecurityHeaders writes the response header set of security.md §9, with the caller's
// content security policy.
//
// The policy is a parameter because there are two origins in this process and they need different
// ones: the API answers JSON and gets `default-src 'none'`, the embedded UI answers a document
// and would be unable to load its own script under that (ADR-0028). Everything else is identical
// and stays defined once, here, so that adding a header adds it to both.
func WriteSecurityHeaders(header http.Header, contentSecurityPolicy string) {
	header.Set("Strict-Transport-Security", hstsValue)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cross-Origin-Resource-Policy", "same-site")
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	header.Set("Permissions-Policy", permissionsPolicy)
	header.Set("Server", serverName)
}

func (s Secured) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	WriteSecurityHeaders(header, contentSecurityPolicy)

	// Vary goes on before the allowlist is consulted, not after. An answer that differs by origin
	// differs whether or not this particular origin was allowed, and a cache told only about the
	// allowed case would happily serve a refusal to somebody on the list.
	if len(s.CORS.AllowedOrigins) > 0 && !s.CORS.AllowsAnyOrigin() {
		header.Add("Vary", "Origin")
	}

	origin := r.Header.Get("Origin")
	allowed := origin != "" && s.originAllowed(origin)
	if allowed {
		s.writeCORSHeaders(header, origin)
	}

	// OPTIONS exists for CORS and for nothing else (security.md §9). It is answered here rather
	// than routed, because a preflight is a question about the route, not a request to it - and
	// because the router would otherwise answer 405 to every browser.
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		if !allowed {
			// No body and no headers: an origin that is not on the list learns only that it is
			// not on the list.
			w.WriteHeader(http.StatusForbidden)
			return
		}
		header.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
		header.Set("Access-Control-Max-Age", strconv.Itoa(int(s.CORS.MaxAge.Seconds())))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.Next.ServeHTTP(w, r)
}

func (s Secured) writeCORSHeaders(header http.Header, origin string) {
	if s.CORS.AllowsAnyOrigin() {
		header.Set("Access-Control-Allow-Origin", env.CORSWildcard)
	} else {
		// The origin is echoed rather than the list returned, which is what the specification
		// requires for more than one allowed origin - and it is why Vary matters: a cache that
		// ignored it would hand one origin's answer to another. It is set by the caller, before
		// the allowlist decides anything.
		header.Set("Access-Control-Allow-Origin", origin)
	}
	header.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
	header.Set("Access-Control-Expose-Headers", corsExposedHeaders)

	// Access-Control-Allow-Credentials is deliberately absent. This API authenticates with a
	// bearer token, never with a cookie, so a browser has no credential to send - and its absence
	// is what makes the wildcard above safe (security.md §9, "never * in combination with
	// credentials").
}

func (s Secured) originAllowed(origin string) bool {
	if s.CORS.AllowsAnyOrigin() {
		return true
	}
	// Exact comparison, not a suffix match: `https://evil-example.com` ends with
	// `example.com`, and that is the whole class of bug this avoids.
	return slices.Contains(s.CORS.AllowedOrigins, origin)
}
