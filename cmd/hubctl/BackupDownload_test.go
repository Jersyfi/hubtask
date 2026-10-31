// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipOf(t *testing.T, name, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	member, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = member.Write([]byte(content))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// UC-BAK-07: the owner's download is written to the named file with the proof the server asked
// for, and only once the whole archive arrived.
func TestTheWorkspaceArchiveIsWrittenToItsFileAfterAFreshProof(t *testing.T) {
	archive := zipOf(t, "manifest.json", `{"format":1}`)
	var proofs []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/backups:download-workspace") {
			t.Errorf("called %s", r.URL.Path)
		}
		proofs = append(proofs, r.Header.Get(stepUpHeader))
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive)
	})
	env := signedIn(stub)
	env[envStepUp] = "proof-1"
	file := filepath.Join(t.TempDir(), "workspace.zip")

	code, out, errOut := invokeAgainst(t, stub, env, "", "backup", "download", "--out", file)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("the archive went to standard output: %q", out)
	}
	written, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(written, archive) {
		t.Fatalf("the file holds %d bytes (%v), want the archive's %d", len(written), err, len(archive))
	}
	if len(proofs) != 1 || proofs[0] != "proof-1" {
		t.Errorf("the proofs sent were %q", proofs)
	}
	if _, err := os.Stat(file + ".part"); !os.IsNotExist(err) {
		t.Errorf("the partial file was left behind: %v", err)
	}
}

// A transfer cut short is not a file that looks like an archive: nothing is left at --out, and
// the command fails. A refusal is the problem document it is.
func TestACutArchiveLeavesNoFileAndARefusalIsReported(t *testing.T) {
	archive := zipOf(t, "manifest.json", `{"format":1}`)
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive[:len(archive)-10])
	})
	file := filepath.Join(t.TempDir(), "workspace.zip")
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "backup", "download", "--out", file)
	if code == exitOK || !strings.Contains(errOut, "incomplete") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, name := range []string{file, file + ".part"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Errorf("%s exists after a cut transfer", name)
		}
	}

	stub = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"too many","status":429,"code":"rate_limited","detail_code":"backup.download_window"}`))
	})
	code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "backup", "download", "--out", file)
	if code == exitOK || errOut == "" {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("a refusal left a file")
	}

	code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "backup", "download")
	if code != exitUsage || !strings.Contains(errOut, "--out") {
		t.Fatalf("without --out: exit %d: %s", code, errOut)
	}
}
