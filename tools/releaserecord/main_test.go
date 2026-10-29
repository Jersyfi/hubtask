// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A release names its commit, image, pipeline, the gates it passed, the evidence by hash and the
// advisories it fixes (versioning-release.md §3).
func TestARecordNamesWhatTheReleaseIs(t *testing.T) {
	dir := t.TempDir()
	advisories := filepath.Join(dir, "advisories")
	if err := os.MkdirAll(advisories, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"a.yaml": "id: HUBTASK-2026-0001\nfixed: [\"1.0.2\"]\n",
		"b.yaml": "id: HUBTASK-2026-0002\nfixed: [\"1.1.0\"]\n",
	} {
		if err := os.WriteFile(filepath.Join(advisories, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sbom := filepath.Join(dir, "sbom.cdx.json")
	if err := os.WriteFile(sbom, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	record, err := build(Inputs{
		Version: "v1.0.2", Commit: "abc123", Image: "ghcr.io/jersyfi/hubtask:v1.0.2", Digest: "sha256:00",
		Run: "https://github.com/Jersyfi/hubtask/actions/runs/1", At: "2026-11-02T10:00:00Z", Go: "1.27.3",
		Advisories: advisories, Files: []string{sbom},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(record.Fixes, []string{"HUBTASK-2026-0001"}) {
		t.Fatalf("fixes %v", record.Fixes)
	}
	if record.Evidence["sbom.cdx.json"] != "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a" {
		t.Fatalf("evidence %v", record.Evidence)
	}
	if !slices.Contains(record.Gates, "make verify") || record.Schema != "hubtask-release-record/1" {
		t.Fatalf("record %+v", record)
	}
	if _, err := build(Inputs{Version: "v1.0.2"}); err == nil {
		t.Fatal("a record without its commit was written")
	}
}
