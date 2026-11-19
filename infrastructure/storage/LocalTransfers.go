// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// LocalTransfers issues the content-route URLs of a local-storage installation: the same
// three-step flow as a presigned bucket, with this server's /media/{id}:content standing in for
// the bucket and an HMAC token standing in for the signature.
type LocalTransfers struct {
	tokens security.MediaTokenIssuer
	// base is HUBTASK_BASE_URL, or empty - then the URL is served relative to the API origin,
	// which every first-party client resolves anyway.
	base string
}

func NewLocalTransfers(tokens security.MediaTokenIssuer, baseURL string) LocalTransfers {
	return LocalTransfers{tokens: tokens, base: strings.TrimSuffix(baseURL, "/")}
}

var _ port.TransferIssuer = LocalTransfers{}

func (l LocalTransfers) IssueUpload(
	ctx context.Context, object media.Object, expiresAt time.Time,
) (port.Transfer, error) {
	return port.Transfer{
		URL:       l.contentURL(ctx, object, security.MediaTokenUpload, expiresAt),
		Method:    http.MethodPut,
		ExpiresAt: expiresAt,
	}, nil
}

func (l LocalTransfers) IssueDownload(
	ctx context.Context, object media.Object, expiresAt time.Time,
) (port.Transfer, error) {
	// The file name is not in the URL: the content route reads it off the record and sets the
	// disposition itself, so there is nothing a holder could tamper with.
	return port.Transfer{
		URL:       l.contentURL(ctx, object, security.MediaTokenDownload, expiresAt),
		Method:    http.MethodGet,
		ExpiresAt: expiresAt,
	}, nil
}

func (l LocalTransfers) contentURL(
	ctx context.Context, object media.Object, purpose security.MediaTokenPurpose, expiresAt time.Time,
) string {
	return l.origin(ctx) + "/api/v1/media/" + object.ID.String() + ":content?token=" +
		l.tokens.Issue(purpose, object.TenantID, object.ID, expiresAt)
}

// origin is the base's scheme at the request's host: the page asking is on that host, and a
// target on another origin is refused by its connect-src and img-src 'self' (security.md §9). The
// scheme stays the installation's, because behind a proxy the request's own is not the browser's.
// A host that does not parse as one keeps the installation's address.
func (l LocalTransfers) origin(ctx context.Context) string {
	host := port.RequestHost(ctx)
	if host == "" || l.base == "" {
		return l.base
	}
	base, err := url.Parse(l.base)
	if err != nil || base.Scheme == "" {
		return l.base
	}
	parsed, err := url.Parse(base.Scheme + "://" + host)
	if err != nil || parsed.Host != host || parsed.User != nil {
		return l.base
	}
	return base.Scheme + "://" + host
}
