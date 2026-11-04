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

// Format is what a page is exported as (UC-DOC-09): Markdown with front matter, a standalone HTML
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
	kind    string // heading, paragraph, item, quote, code, rule, reference, open, close, toc, children, embed, table
	level   int    // a heading's level, an item's depth
	ordered bool
	number  int
	task    int // 0 no checkbox, 1 open, 2 done
	text    string
	variant string     // an opened container's tone, or details
	rows    [][]string // a table's cells, its first row the header
}

var (
	headingLine  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	bulletLine   = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	orderedLine  = regexp.MustCompile(`^(\s*)(\d{1,9})[.)]\s+(.*)$`)
	taskText     = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	fenceLine    = regexp.MustCompile("^(```|~~~)")
	ruleLine     = regexp.MustCompile(`^(?:-{3,}|\*{3,}|_{3,})\s*$`)
	blockRefLine = regexp.MustCompile(`^!\[[^\]]*\]\((hubtask:[a-z]+/[0-9a-fA-F-]{36}(?:#[A-Za-z0-9_-]{1,64})?)\)\s*$`)
	// The wiki blocks (packages/editor, markdown.ts): a callout or a toggle holds blocks up to a line
	// of as many colons; a contents, child-pages or embed block stands on a line of its own.
	containerLine = regexp.MustCompile(`^(:{3,})(info|warning|success|error|details)(?:\s+(.*?))?\s*$`)
	closeLine     = regexp.MustCompile(`^:{3,}\s*$`)
	leafLine      = regexp.MustCompile(`^::(toc|children|embed)(?:\s+(https://\S+))?\s*$`)
	// A table as GitHub's Markdown writes one, which is how the editor writes it: a row of pipes,
	// then a delimiter row, then a line per row.
	tableDelimiter = regexp.MustCompile(`^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?$`)
	dayChip        = regexp.MustCompile(`:date\[(\d{4}-\d{2}-\d{2})\]`)
	statusChip     = regexp.MustCompile(`:status\[([^\]\n]+)\]\{[a-z]+\}`)
	inlinePattern  = regexp.MustCompile("(!?)\\[((?:\\\\.|[^\\]\\\\])*)\\]\\(([^)\\s]+)\\)|\\*\\*([^*]+)\\*\\*|~~([^~]+)~~|\\*([^*]+)\\*|_([^_]+)_|`([^`]+)`|\\\\([\\\\`*_~\\[\\]#])")
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
		case containerLine.MatchString(line):
			m := containerLine.FindStringSubmatch(line)
			out = append(out, block{kind: "open", text: m[3], variant: m[2]})
		case closeLine.MatchString(line):
			out = append(out, block{kind: "close"})
		case leafLine.MatchString(line) && (leafLine.FindStringSubmatch(line)[1] != "embed" || leafLine.FindStringSubmatch(line)[2] != ""):
			m := leafLine.FindStringSubmatch(line)
			out = append(out, block{kind: m[1], text: m[2]})
		case strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && tableDelimiter.MatchString(strings.TrimSpace(lines[i+1])):
			rows := [][]string{tableCells(trimmed)}
			for i += 2; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				rows = append(rows, tableCells(strings.TrimSpace(lines[i])))
			}
			i--
			out = append(out, block{kind: "table", rows: rows})
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

// tableCells splits a table line at its pipes, an escaped pipe kept as a pipe in its cell.
func tableCells(line string) []string {
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	var cells []string
	var cell strings.Builder
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == '\\' && i+1 < len(line) && line[i+1] == '|':
			cell.WriteByte('|')
			i++
		case line[i] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(line[i])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}

func startsBlock(line string) bool {
	return headingLine.MatchString(line) || fenceLine.MatchString(strings.TrimSpace(line)) || ruleLine.MatchString(line) ||
		strings.HasPrefix(line, ">") || bulletLine.MatchString(line) || orderedLine.MatchString(line) ||
		containerLine.MatchString(line) || closeLine.MatchString(line) || leafLine.MatchString(line)
}

// span is a run of inline text with its marks.
type span struct {
	text                       string
	bold, italic, code, strike bool
	href                       string
	image                      bool // an image of href, which only a `hubtask:` address may be here
}

// spans reads the inline grammar of one block. A reference is drawn as its kind in brackets: the
// export resolves no title, so it shows no name a reader of the file might not be allowed to see.
func spans(text string) []span {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\\\n", "\n"), "  \n", "\n")
	// A date and a status are their words in a file: the tone is the editor's drawing.
	text = statusChip.ReplaceAllString(dayChip.ReplaceAllString(text, "$1"), "$1")
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
				out = append(out, span{text: "[" + kind + "]", href: href, image: group(1) == "!"})
			} else if group(1) == "" && linkable(href) {
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
func HTMLFile(page Page) string { return htmlFile(page, nil) }

func htmlFile(page Page, view *PublicView) string {
	var out strings.Builder
	lang := page.ContentLanguage
	if lang == "" {
		lang = "und"
	}
	out.WriteString("<!doctype html>\n<html lang=\"" + html.EscapeString(lang) + "\">\n<head>\n<meta charset=\"utf-8\">\n")
	if view == nil {
		out.WriteString("<meta name=\"hubtask:id\" content=\"" + page.ID.String() + "\">\n")
	}
	if view == nil && !page.ParentID.IsZero() {
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
	parsed := blocks(page.Body)
	var open []string
	for _, b := range parsed {
		if b.kind != "item" {
			closeList()
		}
		switch b.kind {
		case "open":
			if b.variant == "details" {
				out.WriteString("<details open><summary>" + view.spans(spans(b.text)) + "</summary>\n")
				open = append(open, "</details>\n")
			} else {
				out.WriteString("<aside role=\"note\" data-callout=\"" + b.variant + "\">\n")
				open = append(open, "</aside>\n")
			}
		case "close":
			if len(open) > 0 {
				out.WriteString(open[len(open)-1])
				open = open[:len(open)-1]
			}
		case "toc":
			out.WriteString("<nav><ol>\n")
			for _, heading := range parsed {
				if heading.kind == "heading" {
					out.WriteString("<li>" + view.spans(spans(heading.text)) + "</li>\n")
				}
			}
			out.WriteString("</ol></nav>\n")
		case "embed":
			out.WriteString("<p><a href=\"" + html.EscapeString(b.text) + "\">" + html.EscapeString(b.text) + "</a></p>\n")
		case "heading":
			level := strconv.Itoa(min(6, b.level+1))
			out.WriteString("<h" + level + ">" + view.spans(spans(b.text)) + "</h" + level + ">\n")
		case "paragraph":
			out.WriteString("<p>" + view.spans(spans(b.text)) + "</p>\n")
		case "quote":
			out.WriteString("<blockquote><p>" + view.spans(spans(b.text)) + "</p></blockquote>\n")
		case "code":
			out.WriteString("<pre><code>" + html.EscapeString(b.text) + "</code></pre>\n")
		case "rule":
			out.WriteString("<hr>\n")
		case "table":
			out.WriteString("<table>\n")
			for at, row := range b.rows {
				cell := "td"
				switch at {
				case 0:
					cell = "th"
					out.WriteString("<thead>\n")
				case 1:
					out.WriteString("<tbody>\n")
				}
				out.WriteString("<tr>")
				for _, text := range row {
					out.WriteString("<" + cell + ">" + view.spans(spans(text)) + "</" + cell + ">")
				}
				out.WriteString("</tr>\n")
				if at == 0 {
					out.WriteString("</thead>\n")
				}
			}
			if len(b.rows) > 1 {
				out.WriteString("</tbody>\n")
			}
			out.WriteString("</table>\n")
		case "reference":
			if view == nil {
				out.WriteString("<p><a href=\"" + html.EscapeString(b.text) + "\">" + html.EscapeString(b.text) + "</a></p>\n")
			} else if shown := view.reference(b.text, true); shown != "" {
				out.WriteString("<p>" + shown + "</p>\n")
			}
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
			out.WriteString("<li>" + box + view.spans(spans(b.text)) + "</li>\n")
		}
	}
	closeList()
	for i := len(open) - 1; i >= 0; i-- {
		out.WriteString(open[i])
	}
	out.WriteString("</body>\n</html>\n")
	return out.String()
}

func boldened(runs []span) []span {
	for i := range runs {
		runs[i].bold = true
	}
	return runs
}

// linkable reports whether an export may write href as a link. An allow-list, not a deny-list: a
// file opened locally has no CSP, so any scheme a browser runs (javascript:, data:, vbscript:)
// would run there; a page also arrives through the API and the importers, not only the editor.
func linkable(href string) bool {
	lower := strings.ToLower(href)
	for _, scheme := range []string{"https://", "http://", "mailto:"} {
		if strings.HasPrefix(lower, scheme) {
			return true
		}
	}
	return false
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
	parsed := blocks(page.Body)
	for _, b := range parsed {
		switch b.kind {
		case "open":
			if b.variant == "details" {
				paragraph("", "", append([]span{{text: "▸ "}}, boldened(spans(b.text))...), 0)
			}
		case "toc":
			for _, heading := range parsed {
				if heading.kind == "heading" {
					paragraph("", "", spans(heading.text), heading.level)
				}
			}
		case "embed":
			paragraph("", "", []span{{text: b.text, href: b.text}}, 0)
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
		case "table":
			// Borders of its own: the package's styles define no table style to lean on.
			body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>`)
			for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
				body.WriteString(`<w:` + edge + ` w:val="single" w:sz="4" w:space="0" w:color="auto"/>`)
			}
			body.WriteString(`</w:tblBorders></w:tblPr><w:tblGrid>`)
			width := 0
			for _, row := range b.rows {
				width = max(width, len(row))
			}
			body.WriteString(strings.Repeat(`<w:gridCol/>`, width) + `</w:tblGrid>`)
			for at, row := range b.rows {
				body.WriteString("<w:tr>")
				for _, text := range row {
					runs := spans(text)
					if at == 0 {
						runs = boldened(runs)
					}
					body.WriteString("<w:tc>")
					paragraph("", "", runs, 0)
					body.WriteString("</w:tc>")
				}
				body.WriteString("</w:tr>")
			}
			body.WriteString("</w:tbl>")
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
