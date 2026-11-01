// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// WorkspaceArchiveStarter decides an owner's download before a byte is written.
type WorkspaceArchiveStarter interface {
	Start(ctx context.Context, actor appshared.ActorContext, cmd adminservice.DownloadWorkspaceArchiveCommand) (adminservice.WorkspaceDownload, error)
}

// WorkspaceArchiveDoor streams the workspace's archive as one ZIP file into the answer
// (UC-BAK-07 check 7). Delegated, because the archive is a stream the generic door would buffer.
type WorkspaceArchiveDoor struct {
	Downloads WorkspaceArchiveStarter
	// Timeout bounds the whole transfer (rule 7); zero is six hours.
	Timeout time.Duration
	// Stall bounds one write: a client that reads nothing for this long ends the transfer, so a
	// stalled reader never holds the archive's snapshot and its pool connection for the whole
	// Timeout. Zero is two minutes.
	Stall time.Duration
}

// DownloadWorkspaceArchive answers POST /backups:download-workspace.
func (d *WorkspaceArchiveDoor) DownloadWorkspaceArchive(
	w http.ResponseWriter, r *http.Request, params openapi.DownloadWorkspaceArchiveParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	actor, _ := appshared.ActorFrom(r.Context())
	cmd := adminservice.DownloadWorkspaceArchiveCommand{}
	if params.XHubtaskStepUp != nil {
		cmd.StepUpToken = *params.XHubtaskStepUp
	}
	// The body is optional: without one the archive is the unencrypted format.
	if r.ContentLength != 0 {
		var body openapi.WorkspaceArchiveDownload
		err := decodeFrom(http.MaxBytesReader(w, r.Body, 8<<10), &body)
		if typed := shared.AsError(err); err != nil && (typed == nil || typed.DetailCode != "request.body_missing") {
			WriteProblem(w, err, requestID)
			return
		}
		if body.Passphrase != nil {
			cmd.Passphrase = secret.New(*body.Passphrase)
		}
	}
	download, err := d.Downloads.Start(r.Context(), actor, cmd)
	if err != nil {
		if typed := shared.AsError(err); typed != nil && errors.Is(err, shared.ErrRateLimited) {
			if wait, convErr := strconv.Atoi(typed.Params["retry_after"]); convErr == nil && wait > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(wait))
			}
		}
		WriteProblem(w, err, requestID)
		return
	}

	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 6 * time.Hour
	}
	stall := d.Stall
	if stall <= 0 {
		stall = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	// A whole workspace takes longer than the server's write timeout allows a response; each write
	// gets the stall bound instead, renewed while the client reads.
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(stall))

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+download.Name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	archive := NewZipStream(&stallWriter{w: w, controller: controller, stall: stall, end: time.Now().Add(timeout), cancel: cancel})
	if err := download.WriteTo(ctx, archive); err != nil {
		// The answer has begun: the ZIP stays without its end, which no unpacker accepts.
		slog.WarnContext(ctx, "the workspace archive download ended early", "request_id", requestID, "error", err)
		return
	}
	if err := archive.Close(); err != nil {
		slog.WarnContext(ctx, "the workspace archive download could not be finished", "request_id", requestID, "error", err)
	}
}

// stallWriter renews the response's write deadline before every write, never past the transfer's
// end, and cancels the archive once a write fails: the snapshot it reads under ends with it.
type stallWriter struct {
	w          io.Writer
	controller *http.ResponseController
	stall      time.Duration
	end        time.Time
	cancel     context.CancelFunc
}

func (s *stallWriter) Write(p []byte) (int, error) {
	deadline := time.Now().Add(s.stall)
	if deadline.After(s.end) {
		deadline = s.end
	}
	_ = s.controller.SetWriteDeadline(deadline)
	n, err := s.w.Write(p)
	if err != nil {
		s.cancel()
	}
	return n, err
}

// ZipStream is a backupstorage.Store that writes every object as the next member of one ZIP
// stream and keeps nothing: the archive writer only writes, so reading answers ErrUnavailable.
// Members are stored, not deflated - the archive's files are compressed already.
type ZipStream struct {
	mu     sync.Mutex
	writer *zip.Writer
	sizes  map[string]int64
}

var _ backupstorage.Store = (*ZipStream)(nil)

// NewZipStream writes onto w.
func NewZipStream(w io.Writer) *ZipStream {
	return &ZipStream{writer: zip.NewWriter(w), sizes: map[string]int64{}}
}

// Put writes one member.
func (z *ZipStream) Put(_ context.Context, key string, content io.Reader) (int64, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	member, err := z.writer.CreateHeader(&zip.FileHeader{Name: key, Method: zip.Store, Modified: time.Now().UTC()})
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(member, content)
	if err != nil {
		return written, err
	}
	z.sizes[key] = written
	return written, nil
}

// Get is never asked of a stream.
func (z *ZipStream) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, shared.ErrUnavailable.WithDetail("backup.stream_write_only")
}

// List answers nothing: a stream holds no earlier archive.
func (z *ZipStream) List(context.Context, string) ([]backupstorage.Entry, error) { return nil, nil }

// Stat answers a member already written.
func (z *ZipStream) Stat(_ context.Context, key string) (backupstorage.Entry, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	size, ok := z.sizes[key]
	if !ok {
		return backupstorage.Entry{}, shared.ErrNotFound.WithDetail("backup.object_not_found")
	}
	return backupstorage.Entry{Key: key, Size: size}, nil
}

// Delete is never asked of a stream.
func (z *ZipStream) Delete(context.Context, string) error {
	return shared.ErrUnavailable.WithDetail("backup.stream_write_only")
}

// Close writes the ZIP's central directory: without it the file is incomplete.
func (z *ZipStream) Close() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.writer.Close()
}
