// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package wopi is the port for a document server (UC-MED-06, ADR-0104): the access tokens it
// presents to the WOPI host, and what Hubtask asks of it - a file converted to PDF for the viewer.
package wopi

import (
	"context"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Grant is what one access token says: this person, this file, reading or writing, until then.
type Grant struct {
	TenantID  shared.ID
	AccountID shared.ID
	MediaID   shared.ID
	Write     bool
	ExpiresAt time.Time
}

// Tokens signs and reads the access tokens. A token is never stored and never logged (rule 10).
type Tokens interface {
	Issue(grant Grant) string
	// Read verifies a token presented for a file; every forgery and an expired token are
	// ErrInvalidToken alike.
	Read(token string, mediaID shared.ID, now time.Time) (Grant, error)
}

// ErrInvalidToken is the one refusal of a token that does not open this file now.
func ErrInvalidToken() error {
	return shared.ErrUnauthenticated.WithDetail("wopi.token_invalid")
}

// Server is a document server as Hubtask asks it things, through GuardedClient with a deadline
// (rules 6, 7).
type Server interface {
	// ConvertToPDF sends the file and streams back its PDF. ErrUnsupported when the server's kind
	// converts nothing Hubtask can reach.
	ConvertToPDF(ctx context.Context, choice media.DocumentServerChoice, source Source) (io.ReadCloser, error)
	// Reach asks the server for its WOPI discovery, the administration's test of a choice before
	// it is stored. media.document_server_private_network when the address lies inside a
	// private network the installation does not call, media.document_server_unreachable when
	// nothing answers, media.document_server_no_discovery when the answer is no discovery.
	Reach(ctx context.Context, choice media.DocumentServerChoice) error
}

// Source is the file a conversion reads: its bytes, and where the server may fetch them when its
// kind fetches rather than receives (a WOPI GetFile address with a read token).
type Source struct {
	// Key names this version of the file for a server that caches conversions.
	Key     string
	Name    string
	Content io.Reader
	Size    int64
	FetchAt string
}

// ErrUnsupported answers a conversion the server's kind does not offer.
func ErrUnsupported() error {
	return shared.ErrConflict.WithDetail("media.conversion_unavailable")
}
