// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	requestMediaUploadUseCase = "RequestMediaUpload"
	confirmMediaUploadUseCase = "ConfirmMediaUpload"

	// mediaContentSuffix is the one route the contract declares as a byte stream rather than as
	// JSON. Named here because the request bound needs to recognise it (Bounded).
	mediaContentSuffix = ":content"
)

// MediaTokenValidator judges a content-route capability and answers which tenant it was minted in.
//
// An interface here rather than the issuer itself, because presentation may not import
// infrastructure - and because this is the whole of what this layer does with a media token:
// it verifies a signature, which is authentication, and decides nothing else
// (presentation/CLAUDE.md, ADR-0005).
type MediaTokenValidator interface {
	ValidateUpload(token string, mediaID shared.ID, now time.Time) (shared.ID, error)
	ValidateDownload(token string, mediaID shared.ID, now time.Time) (shared.ID, error)
}

// MediaContentService moves the bytes once the capability has been verified.
type MediaContentService interface {
	Receive(ctx context.Context, grant usecasemedia.Grant, content io.Reader) error
	Send(ctx context.Context, grant usecasemedia.Grant) (usecasemedia.Served, error)
}

// RequestMediaUpload answers POST /media.
//
// The answer carries the upload target, which is the point of the whole operation: on an
// object-storage installation it is a presigned bucket URL and the bytes never touch this server
// (arc42 §8.4); on a local one it is this server's own content route, with a token standing in for
// the signature.
func (c *RestController) RequestMediaUpload(
	w http.ResponseWriter, r *http.Request, _ openapi.RequestMediaUploadParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.MediaUploadRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"usage":        string(body.Usage),
		"size":         int(body.Size),
		"file_name":    optionalStringField(body.FileName),
		"content_type": optionalStringField(body.ContentType),
	}

	out, err := c.UseCases.Invoke(r.Context(), requestMediaUploadUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	object := mediaResponse(out)
	w.Header().Set("Location", APIBasePath+"/media/"+object.Id.String())
	writeJSON(w, r, http.StatusCreated, object)
}

// ConfirmMediaUpload answers POST /media/{mediaId}:confirm.
func (c *RestController) ConfirmMediaUpload(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, _ openapi.ConfirmMediaUploadParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := c.UseCases.Invoke(r.Context(), confirmMediaUploadUseCase, actor,
		usecase.Input{"media_id": mediaID.String()})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, mediaResponse(out))
}

// UploadMediaContent answers PUT /media/{mediaId}:content.
//
// No bearer credential and no actor: the URL is the capability, exactly as a presigned bucket URL
// is, and the token in it is what this handler verifies. Everything after that verification is the
// application layer's - which tenant the transaction runs as comes out of the token, and what may
// be done with the object is decided there (ADR-0005).
func (c *RestController) UploadMediaContent(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.UploadMediaContentParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	grant, err := c.grantFor(r, mediaID, params.Token, uploading)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	if err := c.MediaContent.Receive(r.Context(), grant, r.Body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadMediaContent answers GET /media/{mediaId}:content.
//
// A download and never a rendering path: the disposition is attachment, the type is the sniffed
// one rather than anything the uploader claimed, and the sandboxing policy is set on the answer
// itself - so a browser that follows this URL saves a file rather than executing one (T-11).
func (c *RestController) DownloadMediaContent(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.DownloadMediaContentParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	grant, err := c.grantFor(r, mediaID, params.Token, downloading)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	served, err := c.MediaContent.Send(r.Context(), grant)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	defer func() { _ = served.Content.Content.Close() }()

	writeDownloadHeaders(w, served)
	// Streamed rather than buffered: an object read into memory to be written out again is an OOM
	// kill waiting for a large file (T-17). A copy that fails halfway can no longer become a
	// problem document - the status is already on the wire - so it is logged and the connection
	// ends short, which is what a client sees as a truncated download.
	if _, err := io.Copy(w, served.Content.Content); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}

// logStreamFailure records a download that ended short. Not a problem document: the status and the
// headers are already on the wire, so there is nothing left to tell the client - and a truncated
// answer is worth a log line, because it is the shape a disk or a bucket failing takes here.
func logStreamFailure(ctx context.Context, mediaID openapi.MediaId, err error) {
	slog.WarnContext(ctx, "streaming a media object failed",
		slog.String("media_id", mediaID.String()),
		slog.String("error", err.Error()))
}

// direction is which half of the byte movement the capability is for. Not a boolean: an upload
// token must never open a download, and a call site reading `true` says nothing about which.
type direction bool

const (
	uploading   direction = true
	downloading direction = false
)

// grantFor verifies the capability and turns it into what the application layer takes.
func (c *RestController) grantFor(
	r *http.Request, mediaID openapi.MediaId, token string, want direction,
) (usecasemedia.Grant, error) {
	if c.MediaContent == nil || c.MediaTokens == nil || c.Clock == nil {
		return usecasemedia.Grant{}, errNotWired
	}

	id := shared.ID(mediaID.String())
	now := c.Clock.Now()

	var tenantID shared.ID
	var err error
	if want == uploading {
		tenantID, err = c.MediaTokens.ValidateUpload(token, id, now)
	} else {
		tenantID, err = c.MediaTokens.ValidateDownload(token, id, now)
	}
	if err != nil {
		return usecasemedia.Grant{}, err
	}
	return usecasemedia.Grant{TenantID: tenantID, MediaID: id}, nil
}

// writeDownloadHeaders makes the answer a download and nothing else.
func writeDownloadHeaders(w http.ResponseWriter, served usecasemedia.Served) {
	w.Header().Set("Content-Type", served.Content.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(served.Content.Size, 10))
	w.Header().Set("Content-Disposition", dispositionFor(served.FileName))
	// Nothing about an uploaded file may execute in a browser, whatever it turned out to be. The
	// sandbox directive is what makes that true for the formats sniffing cannot fully judge, and
	// nosniff is what stops a browser from overriding the type this server decided (T-11).
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
}

// dispositionFor keeps the quoted-string form of RFC 6266 intact. The domain already refused path
// separators and control characters in a file name; quotes and backslashes would end the string
// early, so they go.
func dispositionFor(fileName string) string {
	if fileName == "" {
		return "attachment"
	}

	quoted := make([]rune, 0, len(fileName))
	for _, r := range fileName {
		if r == '"' || r == '\\' {
			continue
		}
		quoted = append(quoted, r)
	}
	return `attachment; filename="` + string(quoted) + `"`
}

// mediaResponse maps the catalogue's projection onto the contract's schema.
func mediaResponse(out usecase.Output) openapi.MediaObject {
	createdBy := uuidValue(out.String("created_by"))
	createdAt := timeValue(out["created_at"])

	object := openapi.MediaObject{
		Id:          uuidValue(out.String("id")),
		ContentType: out.String("content_type"),
		Size:        int64(out.Int("size")),
		Usage:       openapi.MediaObjectUsage(out.String("usage")),
		Status:      openapi.MediaObjectStatus(out.String("status")),
		RefCount:    out.Int("ref_count"),
		CreatedBy:   createdBy,
		CreatedAt:   createdAt,
	}
	if name := out.String("file_name"); name != "" {
		object.FileName = &name
	}
	if checksum := out.String("checksum"); checksum != "" {
		object.Checksum = &checksum
	}
	object.Upload = transferResponse(out["upload"])
	object.Download = transferResponse(out["download"])
	return object
}

// transferResponse maps one side of the byte movement, or nothing when the projection carries
// none - a PENDING object has no download and a READY one has no upload.
func transferResponse(value any) *openapi.MediaTransfer {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	url, _ := fields["url"].(string)
	method, _ := fields["method"].(string)
	if url == "" {
		return nil
	}
	return &openapi.MediaTransfer{
		Url:       url,
		Method:    openapi.MediaTransferMethod(method),
		ExpiresAt: timeValue(fields["expires_at"]),
	}
}
