// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

type archiveStarterFake struct {
	refused    error
	token      string
	passphrase string
}

func (f *archiveStarterFake) Start(
	_ context.Context, _ appshared.ActorContext, cmd adminservice.DownloadWorkspaceArchiveCommand,
) (adminservice.WorkspaceDownload, error) {
	f.token = cmd.StepUpToken
	f.passphrase = cmd.Passphrase.Reveal()
	if f.refused != nil {
		return adminservice.WorkspaceDownload{}, f.refused
	}
	return adminservice.NewWorkspaceDownload("hubtask-export-x.zip", func(ctx context.Context, store backupstorage.Store) error {
		for key, body := range map[string]string{"x/manifest.json": `{"format":"hubtask"}`, "x/data/items.jsonl": "{}\n"} {
			if _, err := store.Put(ctx, key, strings.NewReader(body)); err != nil {
				return err
			}
		}
		return nil
	}), nil
}

// UC-BAK-07 check 7: the archive arrives as one ZIP file whose members are the archive's files,
// the proof passed through, never cached.
func TestTheWorkspaceArchiveStreamsAsOneZip(t *testing.T) {
	starter := &archiveStarterFake{}
	door := &WorkspaceArchiveDoor{Downloads: starter}
	proof := "proof"
	recorder := httptest.NewRecorder()
	door.DownloadWorkspaceArchive(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/backups:download-workspace", nil),
		openapi.DownloadWorkspaceArchiveParams{XHubtaskStepUp: &proof})

	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/zip" ||
		recorder.Header().Get("Cache-Control") != "no-store" || starter.token != "proof" {
		t.Fatalf("answered %d %v (proof %q)", recorder.Code, recorder.Header(), starter.token)
	}
	body := recorder.Body.Bytes()
	unpacked, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("the answer is no ZIP file: %v", err)
	}
	members := map[string]string{}
	for _, file := range unpacked.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(opened)
		members[file.Name] = string(content)
	}
	if members["x/manifest.json"] != `{"format":"hubtask"}` || members["x/data/items.jsonl"] != "{}\n" {
		t.Errorf("members %v", members)
	}
}

// A refusal is a problem document, and the download window's names the seconds left.
func TestARefusedDownloadSaysWhenToComeBack(t *testing.T) {
	door := &WorkspaceArchiveDoor{Downloads: &archiveStarterFake{refused: shared.ErrRateLimited.
		WithDetail("backup.download_window").WithParams(map[string]string{"retry_after": "3600"})}}
	recorder := httptest.NewRecorder()
	door.DownloadWorkspaceArchive(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/backups:download-workspace", nil),
		openapi.DownloadWorkspaceArchiveParams{})
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "3600" ||
		!strings.Contains(recorder.Body.String(), "backup.download_window") {
		t.Errorf("answered %d %v %s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

// UC-BAK-07 check 7: the passphrase travels in the body to the download, and a body with a field
// the contract does not name is refused before anything is started.
func TestTheDownloadsPassphraseTravelsInTheBody(t *testing.T) {
	starter := &archiveStarterFake{}
	door := &WorkspaceArchiveDoor{Downloads: starter}
	recorder := httptest.NewRecorder()
	door.DownloadWorkspaceArchive(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/backups:download-workspace",
			strings.NewReader(`{"passphrase":"correct horse battery staple"}`)),
		openapi.DownloadWorkspaceArchiveParams{})
	if recorder.Code != http.StatusOK || starter.passphrase != "correct horse battery staple" {
		t.Fatalf("answered %d, passphrase %q", recorder.Code, starter.passphrase)
	}

	refused := &archiveStarterFake{}
	recorder = httptest.NewRecorder()
	(&WorkspaceArchiveDoor{Downloads: refused}).DownloadWorkspaceArchive(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/backups:download-workspace",
			strings.NewReader(`{"pass_phrase":"x"}`)),
		openapi.DownloadWorkspaceArchiveParams{})
	if recorder.Code != http.StatusUnprocessableEntity && recorder.Code != http.StatusBadRequest {
		t.Errorf("an unknown field answered %d", recorder.Code)
	}
}

// A client that stops reading ends the transfer after the stall bound rather than holding the
// archive's snapshot and its pool connection for the whole transfer budget.
func TestADownloadNobodyReadsEndsAfterItsStall(t *testing.T) {
	ended := make(chan error, 1)
	starter := archiveStarterFunc(func(ctx context.Context, store backupstorage.Store) error {
		// Far more than the sockets' buffers hold, so the writes block on the silent client.
		_, err := store.Put(ctx, "x/media/big", io.LimitReader(zeroes{}, 256<<20))
		ended <- err
		return err
	})
	door := &WorkspaceArchiveDoor{Downloads: starter, Stall: 300 * time.Millisecond}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		door.DownloadWorkspaceArchive(w, r, openapi.DownloadWorkspaceArchiveParams{})
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	// The client reads nothing more.
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("the archive was written to a client that read nothing")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a download nobody reads still holds its archive after 20 s")
	}
}

type archiveStarterFunc func(ctx context.Context, store backupstorage.Store) error

func (f archiveStarterFunc) Start(
	context.Context, appshared.ActorContext, adminservice.DownloadWorkspaceArchiveCommand,
) (adminservice.WorkspaceDownload, error) {
	return adminservice.NewWorkspaceDownload("hubtask-export-x.zip", f), nil
}

type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
