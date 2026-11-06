// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package dns is the port through which an own domain's records are looked up (ADR-0095 §4).
//
// Every lookup carries a deadline (rule 7): the adapter bounds each one, whatever the caller's
// context says, so a slow name server holds one check up for seconds, never a worker for minutes.
// Tests use a fake: no test asks the real network.
package dns

import "context"

// Resolver answers the two questions an own domain's check asks.
//
// A name that does not exist, or holds no record of the kind, is an answer - empty, without an
// error. An error is a lookup that did not get an answer (a timeout, a failing server) and wraps
// shared.ErrUnavailable: the check then moves nothing.
type Resolver interface {
	// TXT answers the texts at name.
	TXT(ctx context.Context, name string) ([]string, error)
	// CanonicalName answers where name's CNAME chain ends: name itself where it has no CNAME, empty
	// where it does not exist.
	CanonicalName(ctx context.Context, name string) (string, error)
}
