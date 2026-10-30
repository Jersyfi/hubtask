// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// wikiPageHTML is one exported page; attached are the links of its attachments' section.
func wikiPageHTML(title, crumbs, content, attached string) string {
	return `<html><head><title>Team Handbook : ` + title + `</title></head><body>
<div id="breadcrumb-section"><ol id="breadcrumbs"><li class="first"><span><a href="index.html">Team Handbook</a></span></li>` + crumbs + `</ol></div>
<h1 id="title-heading" class="pagetitle"><span id="title-text"> Team Handbook : ` + title + ` </span></h1>
<div class="page-metadata">Created by <span class="author"> Ada</span>, last modified on Mar 03, 2026</div>
<div id="main-content" class="wiki-content group">` + content + `</div>
<div class="pageSection group"><h2 id="attachments">Attachments:</h2>` + attached + `</div>
</body></html>`
}

// wikiZip is a space's HTML export: an index, three pages two deep, a picture drawn and a file
// listed on one of them, and a file of a page the export left out.
func wikiZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"TEAM/index.html": `<html><head><title>Team Handbook</title></head><body><div id="main-content"></div></body></html>`,
		"TEAM/Onboarding_100.html": wikiPageHTML("Onboarding", "",
			`<h2>First day</h2><p>Read the <a href="Tools_200.html">tools page</a> and the <a href="https://example.org/policy">policy</a>.</p><ul><li>Laptop<ul><li>Charger</li></ul></li><li><strong>Badge</strong></li></ul>`, ""),
		"TEAM/Tools_200.html": wikiPageHTML("Tools", `<li><span><a href="Onboarding_100.html">Onboarding</a></span></li>`,
			`<p>We use <code>git</code>.</p><table><tr><th>Tool</th><th>Owner</th></tr><tr><td>Mail</td><td>IT</td></tr></table><p>The flow: <img src="attachments/200/301.png?version=1" alt="diagram"/> and an <img src="https://example.org/logo.png"/></p>`,
			`<a href="attachments/200/301.png">diagram.png</a><a href="attachments/200/302.pdf">brief.pdf</a>`),
		"TEAM/Editors_300.html": wikiPageHTML("Editors", `<li><span><a href="Onboarding_100.html">Onboarding</a></span></li><li><span><a href="Tools_200.html">Tools</a></span></li>`,
			`<pre>vim --version</pre>`, ""),
		"TEAM/attachments/200/301.png": "\x89PNG\r\n\x1a\npng bytes",
		"TEAM/attachments/200/302.pdf": "%PDF-1.4 the brief",
		"TEAM/attachments/999/401.txt": "a page not exported",
		"TEAM/styles/site.css":         "body{}",
	} {
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

// UC-IMP-05 check 1: a space's export becomes the hub's pages in their tree, each with its
// title, its date and a first version.
func TestASpaceBecomesTheHubsPagesInTheirTree(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	pages := result.Records["pages"]
	if len(pages) != 3 || len(result.Records["page_versions"]) != 3 {
		t.Fatalf("pages = %d, versions = %d", len(pages), len(result.Records["page_versions"]))
	}
	onboarding := titled(pages, "Onboarding")
	tools := titled(pages, "Tools")
	editors := titled(pages, "Editors")
	if onboarding["parent_id"] != nil || tools["parent_id"] != onboarding["id"] || editors["parent_id"] != tools["id"] {
		t.Errorf("tree = %v / %v / %v", onboarding["parent_id"], tools["parent_id"], editors["parent_id"])
	}
	if onboarding["hub_id"] != hub.String() || onboarding["updated_at"] != "2026-03-03T00:00:00Z" {
		t.Errorf("onboarding = %v", onboarding)
	}
	if len(result.Records["containers"]) != 0 || len(result.Records["work_items"]) != 0 {
		t.Error("a wiki lands pages and nothing else")
	}
}

// UC-IMP-05 check 1, the bound: a branch deeper than pages allow hangs its deeper pages at the
// deepest level the tree has, under the last page that may still have children.
func TestATreeDeeperThanPagesAllowHangsAtTheDeepestLevel(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	crumbs := ""
	count := knowledge.MaxDepth + 2
	for i := 1; i <= count; i++ {
		title := fmt.Sprintf("Level %d", i)
		file := fmt.Sprintf("Level_%d.html", i)
		member, err := writer.Create("TEAM/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(wikiPageHTML(title, crumbs, "<p>"+title+"</p>", ""))); err != nil {
			t.Fatal(err)
		}
		crumbs += `<li><span><a href="` + file + `">` + title + `</a></span></li>`
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, buffer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	pages := result.Records["pages"]
	if len(pages) != count {
		t.Fatalf("pages = %d, want %d", len(pages), count)
	}
	parentOf := map[any]any{}
	for _, page := range pages {
		parentOf[page.Data["id"]] = page.Data["parent_id"]
	}
	for _, page := range pages {
		level := 1
		for parent := page.Data["parent_id"]; parent != nil; parent = parentOf[parent] {
			level++
		}
		if level > knowledge.MaxDepth {
			t.Errorf("%v sits at level %d, deeper than %d", page.Data["title"], level, knowledge.MaxDepth)
		}
	}
	lastParent := titled(pages, fmt.Sprintf("Level %d", knowledge.MaxDepth-1))["id"]
	for i := knowledge.MaxDepth; i <= count; i++ {
		if got := titled(pages, fmt.Sprintf("Level %d", i))["parent_id"]; got != lastParent {
			t.Errorf("Level %d hangs under %v, want the deepest page that may have children", i, got)
		}
	}
}

// UC-IMP-05 check 2: the content becomes Markdown, and a link to a page of the space a page
// reference; what no page holds is counted.
func TestAPagesContentBecomesMarkdownWithPageReferences(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	pages := result.Records["pages"]
	onboarding := titled(pages, "Onboarding")["body"].(string)
	tools := titled(pages, "Tools")
	want := "## First day\n\nRead the [tools page](hubtask:page/" + tools["id"].(string) + ") and the [policy](https://example.org/policy).\n\n- Laptop\n  - Charger\n- **Badge**"
	if onboarding != want {
		t.Errorf("body =\n%q\nwant\n%q", onboarding, want)
	}
	if body := tools["body"].(string); !strings.Contains(body, "We use `git`.") || !strings.Contains(body, "| Tool | Owner |\n| --- | --- |\n| Mail | IT |") {
		t.Errorf("tools = %q", body)
	}
	if body := titled(pages, "Editors")["body"].(string); body != "```\nvim --version\n```" {
		t.Errorf("editors = %q", body)
	}
	if result.Unmapped["attachments"] != 1 || result.Unmapped["pictures"] != 1 {
		t.Errorf("unmapped = %v, want the file of the page not exported and the outside picture", result.Unmapped)
	}
}

// UC-IMP-05 check 5: a page's attachments become its files - a picture drawn where it stood, a
// file it does not draw after its content -, each with its bytes and its name from the export.
func TestAPagesAttachmentsAndPicturesBecomeItsFiles(t *testing.T) {
	result, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]map[string]any{}
	for _, record := range result.Records["media_objects"] {
		objects[record.Data["file_name"].(string)] = record.Data
	}
	picture, brief := objects["diagram.png"], objects["brief.pdf"]
	if len(objects) != 2 || picture == nil || brief == nil {
		t.Fatalf("objects = %v", objects)
	}
	if picture["mime_type"] != "image/png" || brief["mime_type"] != "application/pdf" || picture["ref_count"] != 1 {
		t.Errorf("picture %v, brief %v", picture, brief)
	}
	tools := titled(result.Records["pages"], "Tools")
	want := "The flow: and an\n\n![](hubtask:media/" + picture["id"].(string) + ")\n\n![](hubtask:media/" + brief["id"].(string) + ")"
	if body := tools["body"].(string); !strings.HasSuffix(body, want) {
		t.Errorf("body =\n%q\nwant it to end with\n%q", body, want)
	}
	held := result.Records["page_media"]
	if len(held) != 2 {
		t.Fatalf("page media = %v", held)
	}
	for _, record := range held {
		if record.Data["page_id"] != tools["id"] {
			t.Errorf("held by %v, want Tools", record.Data["page_id"])
		}
	}
	if len(result.Media) != 2 {
		t.Errorf("bytes handed over = %d, want 2", len(result.Media))
	}
}

// UC-IMP-05 check 3: the same space twice derives the same pages; a file that is no export is
// refused.
func TestTheSameSpaceTwiceIsTheSamePagesAndAForeignFileIsRefused(t *testing.T) {
	first, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Wiki{}.Convert(context.Background(), trackerSource(t, wikiZip(t)))
	if err != nil {
		t.Fatal(err)
	}
	if titled(first.Records["pages"], "Tools")["id"] != titled(again.Records["pages"], "Tools")["id"] {
		t.Error("the same space derives other pages")
	}
	for _, content := range [][]byte{[]byte(`{"issues": []}`), emptyZip(t)} {
		_, err := Wiki{}.Convert(context.Background(), trackerSource(t, content))
		if shared.AsError(err).DetailCode != domain.CodeFileNotKind {
			t.Errorf("err = %v", err)
		}
	}
}

func emptyZip(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	member, _ := writer.Create("readme.txt")
	_, _ = member.Write([]byte("nothing"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
