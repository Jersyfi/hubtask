// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package connector is the port a connector's source is pulled through (research itsm-cmdb-ai
// C1): one page of changes since a cursor, already in the reconciliation's payload shape, so that
// every source writes through the one reconciliation (R1). The adapters per program live in
// infrastructure/connector and leave the process only through the guarded client (rule 6).
package connector

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The refusals a pull answers, each a shared.ErrUnavailable with this detail code, so that a run
// records why it failed by a code and never by the source's words (rule 10).
const (
	CodeUnreachable    = "connector.unreachable"
	CodeUnauthorised   = "connector.unauthorised"
	CodeRateLimited    = "connector.rate_limited"
	CodeMalformed      = "connector.answer_malformed"
	CodeHostNotAllowed = "connector.host_not_allowed"
	CodeUnsupported    = "connector.kind_unsupported"
	CodeNoCredential   = "connector.credential_missing"
)

// Pull is one request for a page of a source's changes.
type Pull struct {
	// Connector names the dependency, for a circuit breaker of its own.
	Connector shared.ID
	Kind      cmdb.ConnectorKind
	BaseURL   string
	Settings  map[string]string
	// Hosts are the only hosts the pull may reach (research itsm-cmdb-ai C6).
	Hosts      []string
	Credential secret.Secret
	// Cursor is where the last page ended; empty starts from the beginning.
	Cursor string
}

// Page is what one pull fetched.
type Page struct {
	Payload cmdb.Payload
	// Cursor is where the next pull starts.
	Cursor string
	// More says the source has more changes now; the run pulls again from Cursor.
	More bool
}

// Puller pulls a page from a connector's source.
type Puller interface {
	Pull(ctx context.Context, pull Pull) (Page, error)
}

// RetryAfter answers how long a rate-limited source asked to be left alone, from a refusal's
// `retry_after` parameter in seconds; zero when it named none.
func RetryAfter(err error) time.Duration {
	var e *shared.Error
	if !errors.As(err, &e) || e.DetailCode != CodeRateLimited {
		return 0
	}
	seconds := 0
	for _, c := range e.Params["retry_after"] {
		if c < '0' || c > '9' {
			return 0
		}
		seconds = seconds*10 + int(c-'0')
		if seconds > 86400 {
			return 24 * time.Hour
		}
	}
	return time.Duration(seconds) * time.Second
}
