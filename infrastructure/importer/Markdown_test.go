// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func markdownZip(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range members {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// UC-IMP-05 check 6: a ZIP of Markdown files becomes the hub's pages in their folders' tree, a
// link between them a page reference and a picture in the export the page's file.
func TestAFolderOfMarkdownBecomesTheHubsPages(t *testing.T) {
	result, err := Markdown{}.Convert(context.Background(), trackerSource(t, markdownZip(t, map[string]string{
		"Handbook 0123456789abcdef0123456789abcdef.md":           "Welcome. See [tools](Handbook%200123456789abcdef0123456789abcdef/Tools.md).",
		"Handbook 0123456789abcdef0123456789abcdef/Tools.md":     "# Our tools\n\nWe use `git`. ![flow](img/flow.png) and [the web](https://example.org).",
		"Handbook 0123456789abcdef0123456789abcdef/img/flow.png": "\x89PNG\r\n\x1a\npng bytes",
		"notes/index.md":  "# Notes\n",
		"notes/a/deep.md": "under notes, the folder a has no page",
		"notes/README.md": "a second page of the folder notes",
		"__MACOSX/._x.md": "resource fork",
	})))
	if err != nil {
		t.Fatal(err)
	}
	pages := result.Records["pages"]
	if len(pages) != 5 {
		t.Fatalf("%d pages, want 5", len(pages))
	}
	handbook, tools, notes := titled(pages, "Handbook"), titled(pages, "Our tools"), titled(pages, "Notes")
	if handbook == nil || tools == nil || notes == nil {
		t.Fatalf("titles: %v", pages)
	}
	if handbook["parent_id"] != nil || tools["parent_id"] != handbook["id"] {
		t.Errorf("tree: handbook %v, tools under %v", handbook["parent_id"], tools["parent_id"])
	}
	if deep := titled(pages, "deep"); deep == nil || deep["parent_id"] != notes["id"] {
		t.Errorf("a page in a folder without one hangs under %v", deep)
	}
	if body := handbook["body"].(string); body != "Welcome. See [tools](hubtask:page/"+tools["id"].(string)+")." {
		t.Errorf("the link became %q", body)
	}
	body := tools["body"].(string)
	if !strings.HasPrefix(body, "We use `git`. ![flow](hubtask:media/") || !strings.Contains(body, "[the web](https://example.org)") {
		t.Errorf("the tools page is %q", body)
	}
	if len(result.Records["page_media"]) != 1 {
		t.Errorf("files held: %d", len(result.Records["page_media"]))
	}

	again, err := Markdown{}.Convert(context.Background(), trackerSource(t, markdownZip(t, map[string]string{
		"Handbook 0123456789abcdef0123456789abcdef.md": "Welcome.",
	})))
	if err != nil || titled(again.Records["pages"], "Handbook")["id"] != handbook["id"] {
		t.Errorf("a fresh export lands elsewhere (%v)", err)
	}
}

func TestWhatIsNoMarkdownExportIsRefused(t *testing.T) {
	for name, content := range map[string][]byte{
		"not a zip":        []byte("# Title"),
		"a zip without md": markdownZip(t, map[string]string{"a.txt": "x"}),
	} {
		_, err := Markdown{}.Convert(context.Background(), trackerSource(t, content))
		if !errors.Is(err, shared.ErrValidation) || !strings.Contains(err.Error(), domain.CodeFileNotKind) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
