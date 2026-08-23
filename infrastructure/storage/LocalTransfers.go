// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// LocalTransfers issues the content-route URLs of a local-storage installation: the same
// three-step flow as a presigned bucket, with this server's /media/{id}:content standing in for
// the bucket and an HMAC token standing in for the signature (C-06).
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

func (l LocalTransfers) IssueUpload(_ string, mediaID shared.ID, expiresAt time.Time) (port.Transfer, error) {
	return port.Transfer{
		URL:       l.contentURL(mediaID, security.MediaTokenUpload, expiresAt),
		Method:    http.MethodPut,
		ExpiresAt: expiresAt,
	}, nil
}

func (l LocalTransfers) IssueDownload(
	_ string, mediaID shared.ID, _ string, expiresAt time.Time,
) (port.Transfer, error) {
	// The file name is not in the URL: the content route reads it off the record and sets the
	// disposition itself, so there is nothing a holder could tamper with.
	return port.Transfer{
		URL:       l.contentURL(mediaID, security.MediaTokenDownload, expiresAt),
		Method:    http.MethodGet,
		ExpiresAt: expiresAt,
	}, nil
}

func (l LocalTransfers) contentURL(
	mediaID shared.ID, purpose security.MediaTokenPurpose, expiresAt time.Time,
) string {
	return l.base + "/api/v1/media/" + mediaID.String() + ":content?token=" +
		l.tokens.Issue(purpose, mediaID, expiresAt)
}
