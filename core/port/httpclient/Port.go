// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package httpclient is the port for calls that leave the process: webhooks, automation
// actions, AI providers, OIDC discovery.
//
// It exists as a port and not as a bare *http.Client for two reasons. The core must not know a
// transport technology (ADR-0001), and every outbound call has to pass the SSRF guard of
// infrastructure/httpclient.GuardedClient (rule 6, security.md §T-07) - a port that takes a URL
// and returns bytes leaves no room for an adapter to reach for http.DefaultClient instead.
package httpclient

import "context"

// Request is one outbound call. The body is a byte slice rather than a stream: everything
// Hubtask sends outwards is a small, already-rendered payload, and a caller holding a stream
// open across a retry is a resource leak waiting to be found.
type Request struct {
	Method string
	// URL is absolute and http or https. The guard rejects everything else.
	URL string
	// Header is set by the caller. Credentials belong here, never in the URL.
	Header map[string][]string
	Body   []byte
	// TargetClass is what the call is, not who it goes to: webhook, ai, oidc. It becomes the
	// label of hubtask_outbound_http_duration_seconds, so it is a small fixed set - a label
	// per target host would grow a series per customer integration (rule 10).
	TargetClass string
}

// Response is what came back. Status is the HTTP status: an outbound call that reaches its
// target and is answered with a 500 is a successful call with an unhappy answer, and only the
// caller knows which of the two matters to it.
type Response struct {
	Status int
	Header map[string][]string
	Body   []byte
}

// Port makes one call. Every implementation bounds the call with a deadline, refuses private
// and reserved addresses, limits redirects, and caps the response size.
type Port interface {
	Do(ctx context.Context, req Request) (Response, error)
}
