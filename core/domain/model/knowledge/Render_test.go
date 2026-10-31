// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

const renderBody = "# Start\n\nRead **this** and *that*, see [the site](https://example.org) and [](hubtask:entry/00000000-0000-0000-0000-0000000000e1).\n\n" +
	"- one\n- [x] done\n\n1. first\n\n> quoted <b>\n\n```\na < b\n```\n\n---\n\n![](hubtask:view/00000000-0000-0000-0000-0000000000e2)\n"

// UC-DOC-09 check 4: a page exports as a standalone HTML document, every word escaped, no script.
func TestAPageExportsAsHTML(t *testing.T) {
	page := newPage(t, "Plan <1>", renderBody)
	out := HTMLFile(page)
	for _, want := range []string{
		"<title>Plan &lt;1&gt;</title>", "<h2>Start</h2>", "<strong>this</strong>", "<em>that</em>",
		`<a href="https://example.org">the site</a>`, `<a href="hubtask:entry/00000000-0000-0000-0000-0000000000e1">[entry]</a>`,
		"<li><input type=\"checkbox\" checked disabled> done</li>", "<ol>\n<li>first</li>", "<blockquote><p>quoted &lt;b&gt;</p></blockquote>",
		"<pre><code>a &lt; b</code></pre>", "<hr>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the HTML lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<script") || strings.Contains(out, "<b>") {
		t.Errorf("the HTML carries markup of the body:\n%s", out)
	}
}

// UC-DOC-09 check 4: a page exports as a DOCX document whose text is the page's.
func TestAPageExportsAsADOCXDocument(t *testing.T) {
	page := newPage(t, "Plan & more", renderBody)
	encoded, err := DOCXFile(page)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var document string
	names := map[string]bool{}
	for _, f := range archive.File {
		names[f.Name] = true
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			document = string(b)
		}
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml"} {
		if !names[name] {
			t.Errorf("the package lacks %s", name)
		}
	}
	for _, want := range []string{"Plan &amp; more", `w:val="Heading1"`, "<w:b/>", "☑ ", "a &lt; b", "the site (https://example.org)"} {
		if !strings.Contains(document, want) {
			t.Errorf("the document lacks %q", want)
		}
	}
}

func TestAnExportFormatIsOneOfThree(t *testing.T) {
	for raw, want := range map[string]Format{"": FormatMarkdown, "html": FormatHTML, "DOCX": FormatDOCX} {
		if got, err := ParseFormat(raw); err != nil || got != want {
			t.Errorf("%q read as %q, %v", raw, got, err)
		}
	}
	if _, err := ParseFormat("pdf"); detail(err) != "pages.export_format_unknown" {
		t.Errorf("an unknown format answered %v", err)
	}
	files, err := TreeIn([]Page{newPage(t, "Plan", "x")}, newPage(t, "Plan", "x").ID.String(), FormatHTML)
	if err != nil || len(files) != 1 || !strings.HasSuffix(files[0].Path, ".html") {
		t.Errorf("a tree in HTML %+v %v", files, err)
	}
}

// UC-DOC-09 check 4, UC-DOC-01 check 10: the wiki blocks export as what they are - a callout as a
// note, a toggle as a details element with its summary, the contents as the page's headings, an
// embed as its link, a date and a status as their words - and a container left open is closed.
func TestTheWikiBlocksExportAsWhatTheyAre(t *testing.T) {
	body := "::toc\n\n# Rollback\n\n:::warning\nStop **first**.\n:::\n\n::::details Why <now>?\n:::info\nSince :date[2026-10-01], :status[Approved]{success}.\n:::\n::::\n\n" +
		"::children\n\n::embed https://videos.example.org/v/7\n\n::embed http://plain.example.org/x\n\n:::error\nnever closed\n"
	out := HTMLFile(newPage(t, "Runbook", body))
	for _, want := range []string{
		"<nav><ol>\n<li>Rollback</li>\n</ol></nav>", "<aside role=\"note\" data-callout=\"warning\">\n<p>Stop <strong>first</strong>.</p>\n</aside>",
		"<details open><summary>Why &lt;now&gt;?</summary>\n<aside role=\"note\" data-callout=\"info\">\n<p>Since 2026-10-01, Approved.</p>\n</aside>\n</details>",
		`<p><a href="https://videos.example.org/v/7">https://videos.example.org/v/7</a></p>`,
		"<aside role=\"note\" data-callout=\"error\">\n<p>never closed</p>\n</aside>\n</body>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the HTML lacks %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{":::", "::children", "::toc", `href="http://plain`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the HTML carries %q:\n%s", unwanted, out)
		}
	}
	if _, err := DOCXFile(newPage(t, "Runbook", body)); err != nil {
		t.Fatal(err)
	}
}
