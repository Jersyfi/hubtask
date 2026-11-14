// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// WopiService is the slice of the WOPI host this door calls (core/application/service/media.WopiHost).
type WopiService interface {
	StartEditing(ctx context.Context, actor appshared.ActorContext, mediaID shared.ID) (usecasemedia.EditingSession, error)
	ConvertToPDF(ctx context.Context, actor appshared.ActorContext, mediaID shared.ID) (io.ReadCloser, error)
	CheckFileInfo(ctx context.Context, token string, mediaID shared.ID) (usecasemedia.FileInfo, error)
	GetFile(ctx context.Context, token string, mediaID shared.ID) (storage.Object, error)
	Locks(ctx context.Context, token string, mediaID shared.ID, request usecasemedia.LockRequest) (string, error)
	PutFile(ctx context.Context, token string, mediaID shared.ID, lock string, content io.Reader, size int64) (string, error)
}

// WopiDoor serves the WOPI host's routes (UC-MED-06, ADR-0104) and the two routes the viewer
// uses beside them. The WOPI routes answer in the protocol's shape - its status codes and
// X-WOPI headers - because a document server reads nothing else; nothing here decides a
// permission, the access token is only handed inwards (rule 2). The token is never logged
// (rule 10): only the route template reaches a log.
type WopiDoor struct {
	Host WopiService
}

const (
	wopiOverride = "X-WOPI-Override"
	wopiLock     = "X-WOPI-Lock"
	wopiOldLock  = "X-WOPI-OldLock"
	wopiVersion  = "X-WOPI-ItemVersion"
	// wopiContentsSuffix is the byte route PutFile writes to; Bounded gives it the upload bound.
	wopiContentsSuffix = "/contents"
	wopiFilesPrefix    = APIBasePath + "/wopi/files/"
)

// StartMediaEditing answers POST /media/{mediaId}:edit.
func (c *WopiDoor) StartMediaEditing(w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId) {
	requestID := correlation.RequestIDFrom(r.Context())
	actor, _ := appshared.ActorFrom(r.Context())
	session, err := c.Host.StartEditing(r.Context(), actor, shared.ID(mediaID.String()))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	server := openapi.DocumentServer{Kind: openapi.DocumentServerKind(session.Server.Kind)}
	if session.Server.URL != "" {
		server.Url = &session.Server.URL
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSONAnswer(w, http.StatusOK, openapi.MediaEditingSession{
		AccessToken:    session.Token,
		AccessTokenTtl: session.ExpiresAt.UnixMilli(),
		WopiSrc:        session.WopiSrc,
		Server:         server,
	})
}

// ConvertMediaToPdf answers GET /media/{mediaId}:pdf: a download, never a rendering path.
func (c *WopiDoor) ConvertMediaToPdf(w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId) {
	requestID := correlation.RequestIDFrom(r.Context())
	actor, _ := appshared.ActorFrom(r.Context())
	pdf, err := c.Host.ConvertToPDF(r.Context(), actor, shared.ID(mediaID.String()))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	defer func() { _ = pdf.Close() }()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if _, err := io.Copy(w, pdf); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}

// CheckWopiFileInfo answers WOPI CheckFileInfo.
func (c *WopiDoor) CheckWopiFileInfo(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.CheckWopiFileInfoParams,
) {
	info, err := c.Host.CheckFileInfo(r.Context(), params.AccessToken, shared.ID(mediaID.String()))
	if err != nil {
		writeWopiError(w, r, err)
		return
	}
	name := info.Object.FileName
	if name == "" {
		name = info.Object.ID.String()
	}
	write := info.Write
	readOnly := !write
	yes := true
	writeJSONAnswer(w, http.StatusOK, openapi.WopiFileInfo{
		BaseFileName: name, Size: info.Object.ByteSize, Version: info.Version,
		OwnerId: info.Object.CreatedBy.String(), UserId: info.PersonID.String(),
		UserCanWrite: &write, ReadOnly: &readOnly, SupportsLocks: &yes, SupportsGetLock: &yes,
		SupportsUpdate: &yes, UserCanNotWriteRelative: &yes,
	})
}

// LockWopiFile answers the protocol's lock calls, chosen by X-WOPI-Override.
func (c *WopiDoor) LockWopiFile(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.LockWopiFileParams,
) {
	op := strings.ToUpper(strings.TrimSpace(r.Header.Get(wopiOverride)))
	request := usecasemedia.LockRequest{Op: op, Lock: r.Header.Get(wopiLock), Old: r.Header.Get(wopiOldLock)}
	switch op {
	case "LOCK":
		if request.Old != "" {
			request.Op = "UNLOCK_AND_RELOCK"
		}
	case "REFRESH_LOCK", "UNLOCK", "GET_LOCK":
	default:
		// PutRelativeFile, RenameFile and the rest are not offered (UserCanNotWriteRelative).
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	current, err := c.Host.Locks(r.Context(), params.AccessToken, shared.ID(mediaID.String()), request)
	if err != nil {
		writeWopiError(w, r, err)
		return
	}
	if op == "GET_LOCK" {
		w.Header().Set(wopiLock, current)
	}
	w.WriteHeader(http.StatusOK)
}

// GetWopiFile answers WOPI GetFile.
func (c *WopiDoor) GetWopiFile(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.GetWopiFileParams,
) {
	object, err := c.Host.GetFile(r.Context(), params.AccessToken, shared.ID(mediaID.String()))
	if err != nil {
		writeWopiError(w, r, err)
		return
	}
	defer func() { _ = object.Content.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if _, err := io.Copy(w, object.Content); err != nil {
		logStreamFailure(r.Context(), mediaID, err)
	}
}

// PutWopiFile answers WOPI PutFile: a new version of the file.
func (c *WopiDoor) PutWopiFile(
	w http.ResponseWriter, r *http.Request, mediaID openapi.MediaId, params openapi.PutWopiFileParams,
) {
	if r.ContentLength < 0 {
		w.WriteHeader(http.StatusLengthRequired)
		return
	}
	version, err := c.Host.PutFile(r.Context(), params.AccessToken, shared.ID(mediaID.String()),
		r.Header.Get(wopiLock), r.Body, r.ContentLength)
	if err != nil {
		writeWopiError(w, r, err)
		return
	}
	w.Header().Set(wopiVersion, version)
	w.WriteHeader(http.StatusOK)
}

// writeWopiError answers in the protocol's codes: 409 with the holding lock, 401 for a token that
// does not open the file, 404 for a file the person does not reach, 413 for too large, 500
// otherwise - with a log line that names the code, never the token or the file's name.
func writeWopiError(w http.ResponseWriter, r *http.Request, err error) {
	var conflict media.LockConflict
	if errors.As(err, &conflict) {
		w.Header().Set(wopiLock, conflict.Current)
		w.WriteHeader(http.StatusConflict)
		return
	}
	domainErr := shared.AsError(err)
	switch {
	case errors.Is(err, shared.ErrUnauthenticated), errors.Is(err, shared.ErrForbidden):
		w.WriteHeader(http.StatusUnauthorized)
	case errors.Is(err, shared.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case domainErr.DetailCode == "media.too_large":
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	case errors.Is(err, shared.ErrValidation), errors.Is(err, shared.ErrConflict):
		w.WriteHeader(http.StatusConflict)
	default:
		slog.WarnContext(r.Context(), "a WOPI call failed", slog.String("code", domainErr.DetailCode))
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// writeJSONAnswer writes a JSON answer as it is: CheckFileInfo's fields are the protocol's, and
// its answer carries no request-dependent envelope.
func writeJSONAnswer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
