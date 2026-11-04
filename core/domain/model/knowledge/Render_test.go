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

// UC-DOC-09 check 4: a link whose scheme a file could run - javascript:, data:, vbscript: - is
// written as its words, never as an href; a page also arrives through the API and the importers.
func TestALinkOfAnUnsafeSchemeExportsAsItsWords(t *testing.T) {
	page := newPage(t, "Plan", "See [help](javascript:fetch(1)), [pic](data:text/html;base64,PHNjcmlwdD4=), "+
		"[old](VBScript:x), [mail](mailto:a@example.org), [plain](http://example.org) and [rel](/x).\n")
	out := HTMLFile(page)
	for _, banned := range []string{"javascript:", "data:", "VBScript:", `href="/x"`} {
		if strings.Contains(out, banned) {
			t.Errorf("the HTML carries %q:\n%s", banned, out)
		}
	}
	for _, want := range []string{"help", "pic", `<a href="mailto:a@example.org">mail</a>`, `<a href="http://example.org">plain</a>`, "rel"} {
		if !strings.Contains(out, want) {
			t.Errorf("the HTML lacks %q:\n%s", want, out)
		}
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

// A table as the editor writes it (packages/editor, markdown.ts) exports as a table: its first row
// the header, an escaped pipe a pipe in its cell, a cell's words escaped.
func TestATableExportsAsATable(t *testing.T) {
	body := "Before.\n\n| Name | Note |\n| --- | --- |\n| **Ada** | a \\| b <c> |\n| Bob |  |\n\nAfter."
	out := HTMLFile(newPage(t, "People", body))
	want := "<table>\n<thead>\n<tr><th>Name</th><th>Note</th></tr>\n</thead>\n<tbody>\n" +
		"<tr><td><strong>Ada</strong></td><td>a | b &lt;c&gt;</td></tr>\n<tr><td>Bob</td><td></td></tr>\n</tbody>\n</table>\n<p>After.</p>"
	if !strings.Contains(out, want) {
		t.Errorf("the HTML lacks the table:\n%s", out)
	}
	if strings.Contains(out, "| ---") {
		t.Errorf("the HTML carries the delimiter row:\n%s", out)
	}
	encoded, err := DOCXFile(newPage(t, "People", body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(encoded)
	archive, _ := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	var document string
	for _, f := range archive.File {
		if f.Name == "word/document.xml" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			document = string(b)
		}
	}
	if strings.Count(document, "<w:tr>") != 3 || strings.Count(document, "<w:tc>") != 6 || !strings.Contains(document, "a | b &lt;c&gt;") {
		t.Errorf("the document lacks the table's three rows of two cells:\n%s", document)
	}
	// A pipe line without a delimiter row under it is a paragraph, as before.
	if out := HTMLFile(newPage(t, "Pipes", "| not a table |")); !strings.Contains(out, "<p>| not a table |</p>") {
		t.Errorf("a lone pipe line became something else:\n%s", out)
	}
}
