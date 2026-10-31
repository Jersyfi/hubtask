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

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

type archiveStarterFake struct {
	refused error
	token   string
}

func (f *archiveStarterFake) Start(
	_ context.Context, _ appshared.ActorContext, cmd adminservice.DownloadWorkspaceArchiveCommand,
) (adminservice.WorkspaceDownload, error) {
	f.token = cmd.StepUpToken
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
