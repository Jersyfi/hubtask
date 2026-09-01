// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package correlation carries the request-scoped identifiers that every signal is joined on:
// the request ID and the tenant (observability-reliability.md §3.1).
//
// It lives in core/shared rather than in the observability adapter because both ends need it and
// they are not allowed to know each other: the REST middleware writes the values, the logger and
// the metrics read them, and presentation must not import infrastructure
// (project-structure.md §2).
//
// Only identifiers belong here. The actor and the permissions get their own typed wrapper with
// A-06; a context value is not a place to keep an authorisation decision (ADR-0005).
package correlation

import "context"

type contextKey int

const (
	requestIDKey contextKey = iota
	tenantIDKey
	apiClientKey
)

// ContextWithRequestID carries the request ID through the call chain. It is what a user quotes
// in a support request: the one handle that connects a response to a log entry
// (api-guidelines.md §6).
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom returns the request ID, or the empty string outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// ContextWithTenant carries the tenant for the signals. tenant_id is a mandatory log field
// (§3.1) and the one metric label an operator may switch on (§3.2) - an identifier of an
// installation, not user content.
//
// This is for observability only. The tenant boundary itself is enforced by the transaction
// wrapper against the database (ADR-0010), never by reading this value.
func ContextWithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// TenantFrom returns the tenant, or the empty string outside a tenant scope.
func TenantFrom(ctx context.Context) string {
	id, _ := ctx.Value(tenantIDKey).(string)
	return id
}

// ContextWithAPIClient carries the OAuth client a request acts under (H-05): an identifier of a
// registered app, which is what "audited with the client as a first-class actor attribute"
// means. An identifier like the other two, never content - which is why it belongs here.
func ContextWithAPIClient(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, apiClientKey, clientID)
}

// APIClientFrom returns the client, or the empty string for a request no app is behind.
func APIClientFrom(ctx context.Context) string {
	id, _ := ctx.Value(apiClientKey).(string)
	return id
}
