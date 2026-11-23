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
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// SignInPictureReader is the application's answer to which workspace the address names and whether
// the picture is one its sign-in card draws (rule 2): the tenant it is read in, or the one 404.
type SignInPictureReader interface {
	Picture(ctx context.Context, host string, mediaID shared.ID) (shared.ID, error)
}

// workspacePictureCache keeps a picture short and revalidated, not for the life of its address: a
// removed, replaced, switched-off or infected picture is shown from a cache for five minutes at most
// (T-34).
const workspacePictureCache = "public, max-age=300"

// workspacePictureGone is every refusal of the route, the same whatever the reason (T-34).
func workspacePictureGone() error {
	return shared.ErrNotFound.WithDetail("media.not_found")
}

// GetWorkspacePicture answers GET /auth/workspace-pictures/{mediaId}: a picture the sign-in card at
// this address draws, streamed as the raster image it was stored as - never a rendering path, never
// a redirect to the object store, which the page's img-src would refuse (T-11, T-34).
func (c *MediaDoor) GetWorkspacePicture(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.GetWorkspacePictureParams,
) {
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
	// A media object's bytes never change under its identifier, so the identifier is the tag. Asked
	// only after the application admitted the picture again: a removed one answers 404, not 304.
	tag := `"` + id.String() + `"`
	header := w.Header()
	if params.IfNoneMatch != nil && noneMatchHolds(*params.IfNoneMatch, tag) {
		header.Set("ETag", tag)
		header.Set("Cache-Control", workspacePictureCache)
		w.WriteHeader(http.StatusNotModified)
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
	if !media.IsWorkspacePictureType(served.Content.ContentType) {
		// The type the bytes are sent as, judged once more where they leave (T-34).
		WriteProblem(w, workspacePictureGone(), requestID)
		return
	}
	header.Set("Content-Type", served.Content.ContentType)
	header.Set("Content-Length", strconv.FormatInt(served.Content.Size, 10))
	header.Set("Content-Disposition", "inline")
	header.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("ETag", tag)
	header.Set("Cache-Control", workspacePictureCache)
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, served.Content.Content); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}
