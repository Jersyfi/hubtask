// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// SignInPictureReader is the application's answer to which workspace the address names and whether
// the picture is one its sign-in card draws (rule 2): the tenant it is read in, or the one 404.
type SignInPictureReader interface {
	Picture(ctx context.Context, host string, mediaID shared.ID) (shared.ID, error)
}

// workspacePictureGone is every refusal of the route, the same whatever the reason (T-34).
func workspacePictureGone() error {
	return shared.ErrNotFound.WithDetail("media.not_found")
}

// GetWorkspacePicture answers GET /auth/workspace-pictures/{mediaId}: a picture the sign-in card at
// this address draws, streamed as the raster image it was stored as - never a rendering path, never
// a redirect to the object store, which the page's img-src would refuse (T-11, T-34).
func (c *MediaDoor) GetWorkspacePicture(w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.SignIn == nil || c.Content == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	id := shared.ID(mediaID.String())
	tenant, err := c.SignIn.Picture(r.Context(), r.Host, id)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	served, err := c.Content.Send(r.Context(), usecasemedia.Grant{TenantID: tenant, MediaID: id})
	if errors.Is(err, shared.ErrNotFound) {
		err = workspacePictureGone()
	}
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	defer func() { _ = served.Content.Content.Close() }()
	header := w.Header()
	header.Set("Content-Type", served.Content.ContentType)
	header.Set("Content-Length", strconv.FormatInt(served.Content.Size, 10))
	header.Set("Content-Disposition", "inline")
	header.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	header.Set("X-Content-Type-Options", "nosniff")
	// The address is the picture's identifier, which a new picture replaces: a copy kept as long as
	// the address stands is never a stale one.
	header.Set("Cache-Control", "public, max-age=31536000, immutable")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, served.Content.Content); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}
