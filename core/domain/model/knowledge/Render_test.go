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

// UC-DOC-12 check 3: a page exports as a standalone HTML document, every word escaped, no script.
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

// UC-DOC-12 check 3: a page exports as a DOCX document whose text is the page's.
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
