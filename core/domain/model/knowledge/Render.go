// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Format is what a page is exported as (UC-DOC-12): Markdown with front matter, a standalone HTML
// document, or an Office Open XML text document (ISO/IEC 29500, DOCX).
type Format string

const (
	FormatMarkdown Format = "MARKDOWN"
	FormatHTML     Format = "HTML"
	FormatDOCX     Format = "DOCX"
)

// ParseFormat reads a requested format; empty is Markdown.
func ParseFormat(raw string) (Format, error) {
	switch Format(strings.ToUpper(strings.TrimSpace(raw))) {
	case "", FormatMarkdown:
		return FormatMarkdown, nil
	case FormatHTML:
		return FormatHTML, nil
	case FormatDOCX:
		return FormatDOCX, nil
	}
	return "", invalid("/format", "pages.export_format_unknown")
}

// Extension is the file name's ending of a format.
func (f Format) Extension() string {
	switch f {
	case FormatHTML:
		return ".html"
	case FormatDOCX:
		return ".docx"
	}
	return ".md"
}

// Binary reports whether the format's content travels as base64.
func (f Format) Binary() bool { return f == FormatDOCX }

// block is one block of a body, as the editor writes it (packages/editor, markdown.ts).
type block struct {
	kind    string // heading, paragraph, item, quote, code, rule, reference
	level   int    // a heading's level, an item's depth
	ordered bool
	number  int
	task    int // 0 no checkbox, 1 open, 2 done
	text    string
}

var (
	headingLine   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	bulletLine    = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	orderedLine   = regexp.MustCompile(`^(\s*)(\d{1,9})[.)]\s+(.*)$`)
	taskText      = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	fenceLine     = regexp.MustCompile("^(```|~~~)")
	ruleLine      = regexp.MustCompile(`^(?:-{3,}|\*{3,}|_{3,})\s*$`)
	blockRefLine  = regexp.MustCompile(`^!\[[^\]]*\]\((hubtask:[a-z]+/[0-9a-fA-F-]{36}(?:#[A-Za-z0-9_-]{1,64})?)\)\s*$`)
	inlinePattern = regexp.MustCompile("(!?)\\[((?:\\\\.|[^\\]\\\\])*)\\]\\(([^)\\s]+)\\)|\\*\\*([^*]+)\\*\\*|~~([^~]+)~~|\\*([^*]+)\\*|_([^_]+)_|`([^`]+)`|\\\\([\\\\`*_~\\[\\]#])")
)

// blocks reads a body into its blocks. What it does not know is a paragraph of its text.
func blocks(body string) []block {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var out []block
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case fenceLine.MatchString(trimmed):
			fence := trimmed[:3]
			var code []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != fence; i++ {
				code = append(code, lines[i])
			}
			out = append(out, block{kind: "code", text: strings.Join(code, "\n")})
		case headingLine.MatchString(line):
			m := headingLine.FindStringSubmatch(line)
			out = append(out, block{kind: "heading", level: len(m[1]), text: m[2]})
		case ruleLine.MatchString(line):
			out = append(out, block{kind: "rule"})
		case blockRefLine.MatchString(line):
			out = append(out, block{kind: "reference", text: blockRefLine.FindStringSubmatch(line)[1]})
		case strings.HasPrefix(line, ">"):
			out = append(out, block{kind: "quote", text: strings.TrimSpace(strings.TrimPrefix(line, ">"))})
		case orderedLine.MatchString(line):
			m := orderedLine.FindStringSubmatch(line)
			n, _ := strconv.Atoi(m[2])
			out = append(out, block{kind: "item", ordered: true, number: n, level: len(m[1]) / 2, text: m[3]})
		case bulletLine.MatchString(line):
			m := bulletLine.FindStringSubmatch(line)
			item := block{kind: "item", level: len(m[1]) / 2, text: m[2]}
			if t := taskText.FindStringSubmatch(item.text); t != nil {
				item.task, item.text = 1, t[2]
				if t[1] != " " {
					item.task = 2
				}
			}
			out = append(out, item)
		default:
			text := line
			for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && !startsBlock(lines[i+1]) {
				i++
				text += "\n" + lines[i]
			}
			out = append(out, block{kind: "paragraph", text: text})
		}
	}
	return out
}

func startsBlock(line string) bool {
	return headingLine.MatchString(line) || fenceLine.MatchString(strings.TrimSpace(line)) || ruleLine.MatchString(line) ||
		strings.HasPrefix(line, ">") || bulletLine.MatchString(line) || orderedLine.MatchString(line)
}

// span is a run of inline text with its marks.
type span struct {
	text                       string
	bold, italic, code, strike bool
	href                       string
}

// spans reads the inline grammar of one block. A reference is drawn as its kind in brackets: the
// export resolves no title, so it shows no name a reader of the file might not be allowed to see.
func spans(text string) []span {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\\\n", "\n"), "  \n", "\n")
	var out []span
	at := 0
	for _, m := range inlinePattern.FindAllStringSubmatchIndex(text, -1) {
		if m[0] > at {
			out = append(out, span{text: strings.ReplaceAll(text[at:m[0]], "\n", " ")})
		}
		group := func(n int) string {
			if m[2*n] < 0 {
				return ""
			}
			return text[m[2*n]:m[2*n+1]]
		}
		switch {
		case m[6] >= 0:
			href := group(3)
			if strings.HasPrefix(href, "hubtask:") {
				kind := strings.SplitN(strings.TrimPrefix(href, "hubtask:"), "/", 2)[0]
				out = append(out, span{text: "[" + kind + "]", href: href})
			} else if group(1) == "" {
				out = append(out, span{text: group(2), href: href})
			} else {
				out = append(out, span{text: group(2)})
			}
		case m[8] >= 0:
			out = append(out, span{text: group(4), bold: true})
		case m[10] >= 0:
			out = append(out, span{text: group(5), strike: true})
		case m[12] >= 0:
			out = append(out, span{text: group(6), italic: true})
		case m[14] >= 0:
			out = append(out, span{text: group(7), italic: true})
		case m[16] >= 0:
			out = append(out, span{text: group(8), code: true})
		case m[18] >= 0:
			out = append(out, span{text: group(9)})
		}
		at = m[1]
	}
	if at < len(text) {
		out = append(out, span{text: strings.ReplaceAll(text[at:], "\n", " ")})
	}
	return out
}

// HTMLFile writes a page as a standalone HTML document: no script, no style sheet from elsewhere,
// every word escaped. A reference is a link to its `hubtask:` address.
func HTMLFile(page Page) string {
	var out strings.Builder
	lang := page.ContentLanguage
	if lang == "" {
		lang = "und"
	}
	out.WriteString("<!doctype html>\n<html lang=\"" + html.EscapeString(lang) + "\">\n<head>\n<meta charset=\"utf-8\">\n")
	out.WriteString("<meta name=\"hubtask:id\" content=\"" + page.ID.String() + "\">\n")
	if !page.ParentID.IsZero() {
		out.WriteString("<meta name=\"hubtask:parent\" content=\"" + page.ParentID.String() + "\">\n")
	}
	out.WriteString("<title>" + html.EscapeString(page.Title) + "</title>\n</head>\n<body>\n")
	out.WriteString("<h1>" + html.EscapeString(page.Title) + "</h1>\n")
	list := ""
	closeList := func() {
		if list != "" {
			out.WriteString("</" + list + ">\n")
			list = ""
		}
	}
	for _, b := range blocks(page.Body) {
		if b.kind != "item" {
			closeList()
		}
		switch b.kind {
		case "heading":
			level := strconv.Itoa(min(6, b.level+1))
			out.WriteString("<h" + level + ">" + htmlSpans(spans(b.text)) + "</h" + level + ">\n")
		case "paragraph":
			out.WriteString("<p>" + htmlSpans(spans(b.text)) + "</p>\n")
		case "quote":
			out.WriteString("<blockquote><p>" + htmlSpans(spans(b.text)) + "</p></blockquote>\n")
		case "code":
			out.WriteString("<pre><code>" + html.EscapeString(b.text) + "</code></pre>\n")
		case "rule":
			out.WriteString("<hr>\n")
		case "reference":
			out.WriteString("<p><a href=\"" + html.EscapeString(b.text) + "\">" + html.EscapeString(b.text) + "</a></p>\n")
		case "item":
			want := "ul"
			if b.ordered {
				want = "ol"
			}
			if list != want {
				closeList()
				out.WriteString("<" + want + ">\n")
				list = want
			}
			box := ""
			switch b.task {
			case 1:
				box = "<input type=\"checkbox\" disabled> "
			case 2:
				box = "<input type=\"checkbox\" checked disabled> "
			}
			out.WriteString("<li>" + box + htmlSpans(spans(b.text)) + "</li>\n")
		}
	}
	closeList()
	out.WriteString("</body>\n</html>\n")
	return out.String()
}

func htmlSpans(runs []span) string {
	var out strings.Builder
	for _, r := range runs {
		text := html.EscapeString(r.text)
		if r.code {
			text = "<code>" + text + "</code>"
		}
		if r.bold {
			text = "<strong>" + text + "</strong>"
		}
		if r.italic {
			text = "<em>" + text + "</em>"
		}
		if r.strike {
			text = "<s>" + text + "</s>"
		}
		if r.href != "" {
			text = "<a href=\"" + html.EscapeString(r.href) + "\">" + text + "</a>"
		}
		out.WriteString(text)
	}
	return out.String()
}

// DOCXFile writes a page as an Office Open XML text document, base64 for the answer's text: the title, headings in
// the built-in heading styles, paragraphs, lists as indented paragraphs with their mark, code in a
// fixed-width font. What a word processor would add - numbering definitions, comments - is left out.
func DOCXFile(page Page) (string, error) {
	var body strings.Builder
	paragraph := func(style, prefix string, runs []span, indent int) {
		body.WriteString("<w:p><w:pPr>")
		if style != "" {
			body.WriteString(`<w:pStyle w:val="` + style + `"/>`)
		}
		if indent > 0 {
			body.WriteString(`<w:ind w:left="` + strconv.Itoa(indent*360) + `"/>`)
		}
		body.WriteString("</w:pPr>")
		if prefix != "" {
			body.WriteString(docxRun(span{text: prefix}))
		}
		for _, r := range runs {
			body.WriteString(docxRun(r))
		}
		body.WriteString("</w:p>")
	}
	paragraph("Title", "", []span{{text: page.Title}}, 0)
	for _, b := range blocks(page.Body) {
		switch b.kind {
		case "heading":
			paragraph("Heading"+strconv.Itoa(min(6, b.level)), "", spans(b.text), 0)
		case "paragraph":
			paragraph("", "", spans(b.text), 0)
		case "quote":
			paragraph("Quote", "", spans(b.text), 1)
		case "code":
			for _, line := range strings.Split(b.text, "\n") {
				paragraph("", "", []span{{text: line, code: true}}, 1)
			}
		case "rule":
			paragraph("", "", []span{{text: "———"}}, 0)
		case "reference":
			paragraph("", "", []span{{text: b.text}}, 0)
		case "item":
			mark := "• "
			if b.ordered {
				mark = strconv.Itoa(b.number) + ". "
			}
			switch b.task {
			case 1:
				mark = "☐ "
			case 2:
				mark = "☑ "
			}
			paragraph("", mark, spans(b.text), b.level+1)
		}
	}
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		body.String() + `<w:sectPr/></w:body></w:document>`

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`},
		{"word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`</Relationships>`},
		{"word/styles.xml", docxStyles()},
		{"word/document.xml", document},
	}
	for _, part := range parts {
		w, err := archive.Create(part.name)
		if err != nil {
			return "", err
		}
		if _, err := w.Write([]byte(part.content)); err != nil {
			return "", err
		}
	}
	if err := archive.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func docxRun(r span) string {
	var props strings.Builder
	if r.bold {
		props.WriteString("<w:b/>")
	}
	if r.italic {
		props.WriteString("<w:i/>")
	}
	if r.strike {
		props.WriteString("<w:strike/>")
	}
	if r.code {
		props.WriteString(`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/>`)
	}
	text := r.text
	if r.href != "" && !strings.HasPrefix(r.href, "hubtask:") {
		text += " (" + r.href + ")"
	}
	out := "<w:r>"
	if props.Len() > 0 {
		out += "<w:rPr>" + props.String() + "</w:rPr>"
	}
	return out + `<w:t xml:space="preserve">` + xmlEscape(text) + "</w:t></w:r>"
}

func xmlEscape(text string) string {
	var out bytes.Buffer
	for _, r := range text {
		// XML 1.0 refuses most control characters; a body never needs them.
		if r < 0x20 && r != '\t' {
			continue
		}
		switch r {
		case '<':
			out.WriteString("&lt;")
		case '>':
			out.WriteString("&gt;")
		case '&':
			out.WriteString("&amp;")
		case '"':
			out.WriteString("&quot;")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func docxStyles() string {
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:rPr><w:b/><w:sz w:val="48"/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:rPr><w:i/></w:rPr></w:style>`)
	for level := 1; level <= 6; level++ {
		n := strconv.Itoa(level)
		size := strconv.Itoa(36 - 4*level)
		out.WriteString(`<w:style w:type="paragraph" w:styleId="Heading` + n + `"><w:name w:val="heading ` + n + `"/>` +
			`<w:basedOn w:val="Normal"/><w:pPr><w:outlineLvl w:val="` + strconv.Itoa(level-1) + `"/></w:pPr>` +
			`<w:rPr><w:b/><w:sz w:val="` + size + `"/></w:rPr></w:style>`)
	}
	out.WriteString(`</w:styles>`)
	return out.String()
}
