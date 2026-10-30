// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Wiki converts a wiki space's HTML export - a ZIP with one HTML file per page, each naming its
// ancestors in a breadcrumb - into the hub's pages in their tree (D-10). A page's content becomes
// its Markdown body, a link to another page of the space a page reference, the page's dates its
// own. What the export carries and pages have no place for - attachments and pictures - is counted.
//
// Identities derive from the space's name and each page's file, so that a fresh export of the same
// space into the same hub lands on the pages the first one made.
type Wiki struct{}

var _ service.Converter = Wiki{}

func (Wiki) Kind() domain.Kind { return domain.KindWiki }

type wikiPage struct {
	file      string
	title     string
	parent    string
	body      *html.Node
	created   *time.Time
	updated   *time.Time
	ancestors []string
}

var pageMetadataDate = regexp.MustCompile(`(?:on|am)\s+([A-Z][a-z]{2} \d{1,2}, \d{4})`)

func (c Wiki) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	if !bytes.HasPrefix(raw, []byte("PK\x03\x04")) {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
	}
	members := append([]*zip.File(nil), reader.File...)
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })

	pages := map[string]*wikiPage{}
	var files []string
	space := ""
	unmapped := map[string]int{}
	var total int64
	for _, member := range members {
		name := path.Clean(strings.ReplaceAll(member.Name, "\\", "/"))
		if member.FileInfo().IsDir() {
			continue
		}
		if indexOf(strings.Split(name, "/"), "attachments") >= 0 {
			unmapped["attachments"]++
			continue
		}
		if !strings.EqualFold(path.Ext(name), ".html") {
			continue
		}
		content, err := readMember(member, &total)
		if err != nil {
			return service.Result{}, err
		}
		document, err := html.Parse(bytes.NewReader(content))
		if err != nil {
			continue
		}
		base := path.Base(name)
		if strings.EqualFold(base, "index.html") {
			if title := spaceTitle(document); title != "" {
				space = title
			}
			continue
		}
		page := readWikiPage(document)
		if page == nil {
			continue
		}
		page.file = base
		pages[base] = page
		files = append(files, base)
	}
	if len(pages) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	b := newBuilder(source, "wiki:"+space)
	ids := map[string]shared.ID{}
	for _, file := range files {
		ids[file] = b.id("pages", "page:"+file)
	}
	// Parents first: a page's depth is its breadcrumb's, cut at the tree's bound.
	sort.SliceStable(files, func(i, j int) bool {
		return len(pages[files[i]].ancestors) < len(pages[files[j]].ancestors)
	})
	var refused []refusal
	for row, file := range files {
		page := pages[file]
		parent := ""
		for i := len(page.ancestors) - 1; i >= 0; i-- {
			if _, known := pages[page.ancestors[i]]; known {
				parent = page.ancestors[i]
				break
			}
		}
		for depth := depthOf(pages, parent); depth >= knowledge.MaxDepth && parent != ""; depth-- {
			parent = pages[parent].parent
		}
		page.parent = parent
		pictures := 0
		body := strings.TrimSpace(markdownOf(page.body, func(href string) (string, bool) {
			target := path.Base(strings.SplitN(href, "#", 2)[0])
			if id, ok := ids[target]; ok && !strings.Contains(href, "://") {
				return "hubtask:page/" + id.String(), true
			}
			return "", false
		}, &pictures))
		unmapped["pictures"] += pictures
		parentKey := ""
		if parent != "" {
			parentKey = "page:" + parent
		}
		if _, ok := b.page(Page{Key: "page:" + file, ParentKey: parentKey, Title: page.title, Body: body, CreatedAt: page.created, UpdatedAt: page.updated}); !ok {
			refused = append(refused, refusal{row: row + 1, code: domain.CodeRowTitleMissing})
		}
	}
	if len(b.records["pages"]) > knowledge.MaxPagesPerHub {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	result := b.result(refused)
	result.Unmapped = unmapped
	return result, nil
}

func depthOf(pages map[string]*wikiPage, file string) int {
	depth := 0
	for file != "" && depth <= knowledge.MaxDepth {
		page, ok := pages[file]
		if !ok {
			break
		}
		file = page.parent
		depth++
	}
	return depth
}

// spaceTitle is the space's name from the index's title.
func spaceTitle(document *html.Node) string {
	if title := find(document, func(n *html.Node) bool { return n.DataAtom == atom.Title }); title != nil {
		return strings.TrimSpace(textOf(title))
	}
	return ""
}

// readWikiPage reads one exported page: its title without the space's prefix, its breadcrumb's
// pages, its main content and its dates. A file without a main content is no page.
func readWikiPage(document *html.Node) *wikiPage {
	content := find(document, func(n *html.Node) bool { return attr(n, "id") == "main-content" })
	if content == nil {
		return nil
	}
	page := &wikiPage{body: content}
	title := find(document, func(n *html.Node) bool { return attr(n, "id") == "title-text" })
	if title == nil {
		title = find(document, func(n *html.Node) bool { return n.DataAtom == atom.Title })
	}
	if title != nil {
		page.title = strings.TrimSpace(textOf(title))
		if colon := strings.Index(page.title, " : "); colon >= 0 {
			page.title = strings.TrimSpace(page.title[colon+3:])
		}
	}
	if crumbs := find(document, func(n *html.Node) bool { return attr(n, "id") == "breadcrumbs" }); crumbs != nil {
		walk(crumbs, func(n *html.Node) {
			if n.DataAtom == atom.A {
				target := path.Base(attr(n, "href"))
				if target != "" && !strings.EqualFold(target, "index.html") {
					page.ancestors = append(page.ancestors, target)
				}
			}
		})
	}
	if meta := find(document, func(n *html.Node) bool { return strings.Contains(attr(n, "class"), "page-metadata") }); meta != nil {
		if match := pageMetadataDate.FindStringSubmatch(textOf(meta)); match != nil {
			if at, err := time.Parse("Jan 2, 2006", match[1]); err == nil {
				page.updated = &at
			}
		}
	}
	return page
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := find(child, match); found != nil {
			return found
		}
	}
	return nil
}

func walk(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

func textOf(n *html.Node) string {
	var out strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(n)
	return strings.Join(strings.Fields(out.String()), " ")
}

// markdownOf renders the content as Markdown in the notes' grammar. resolve answers a page
// reference for a link to a page of the space; a picture is counted and left out.
func markdownOf(n *html.Node, resolve func(string) (string, bool), pictures *int) string {
	w := &markdownWriter{resolve: resolve, pictures: pictures}
	w.children(n, "")
	return collapseBlankLines(w.out.String())
}

type markdownWriter struct {
	out      strings.Builder
	resolve  func(string) (string, bool)
	pictures *int
}

func (w *markdownWriter) children(n *html.Node, indent string) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		w.block(child, indent)
	}
}

func (w *markdownWriter) block(n *html.Node, indent string) {
	if n.Type == html.TextNode {
		if text := strings.Join(strings.Fields(n.Data), " "); text != "" {
			w.out.WriteString(text + " ")
		}
		return
	}
	if n.Type != html.ElementNode {
		return
	}
	switch n.DataAtom {
	case atom.Script, atom.Style:
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		level := int(n.Data[1] - '0')
		w.out.WriteString("\n\n" + strings.Repeat("#", level) + " " + strings.TrimSpace(w.inline(n)) + "\n\n")
	case atom.P:
		if text := strings.TrimSpace(w.inline(n)); text != "" {
			w.out.WriteString("\n\n" + indent + text + "\n\n")
		}
	case atom.Br:
		w.out.WriteString("  \n")
	case atom.Hr:
		w.out.WriteString("\n\n---\n\n")
	case atom.Ul, atom.Ol:
		w.out.WriteString("\n\n")
		w.list(n, indent)
		w.out.WriteString("\n")
	case atom.Pre:
		w.out.WriteString("\n\n```\n" + strings.Trim(rawText(n), "\n") + "\n```\n\n")
	case atom.Blockquote:
		inner := markdownOf(n, w.resolve, w.pictures)
		w.out.WriteString("\n\n")
		for _, line := range strings.Split(strings.TrimSpace(inner), "\n") {
			w.out.WriteString(indent + "> " + line + "\n")
		}
		w.out.WriteString("\n")
	case atom.Table:
		w.table(n)
	case atom.Div, atom.Span, atom.Section, atom.Article, atom.Tbody, atom.Thead:
		w.children(n, indent)
	case atom.Img:
		*w.pictures++
	default:
		w.out.WriteString(w.inline(n))
	}
}

func (w *markdownWriter) list(n *html.Node, indent string) {
	number := 0
	for item := n.FirstChild; item != nil; item = item.NextSibling {
		if item.DataAtom != atom.Li {
			continue
		}
		number++
		marker := "- "
		if n.DataAtom == atom.Ol {
			marker = strconv.Itoa(number) + ". "
		}
		w.listItem(item, indent, marker)
	}
}

func (w *markdownWriter) listItem(item *html.Node, indent, marker string) {
	var head strings.Builder
	var nested []*html.Node
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.DataAtom == atom.Ul || child.DataAtom == atom.Ol {
			nested = append(nested, child)
			continue
		}
		if child.Type == html.TextNode {
			head.WriteString(child.Data)
			continue
		}
		if child.DataAtom == atom.P {
			head.WriteString(w.inline(child) + " ")
			continue
		}
		head.WriteString(w.inlineNode(child))
	}
	w.out.WriteString(indent + marker + strings.Join(strings.Fields(head.String()), " ") + "\n")
	for _, list := range nested {
		w.list(list, indent+"  ")
	}
}

func (w *markdownWriter) table(n *html.Node) {
	var rows [][]string
	walk(n, func(row *html.Node) {
		if row.DataAtom != atom.Tr {
			return
		}
		var cells []string
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.DataAtom == atom.Td || cell.DataAtom == atom.Th {
				cells = append(cells, strings.ReplaceAll(strings.TrimSpace(w.inline(cell)), "|", "\\|"))
			}
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	})
	if len(rows) == 0 {
		return
	}
	w.out.WriteString("\n\n")
	for i, cells := range rows {
		w.out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			w.out.WriteString("|" + strings.Repeat(" --- |", len(cells)) + "\n")
		}
	}
	w.out.WriteString("\n")
}

// inline renders a node's content as one line of Markdown.
func (w *markdownWriter) inline(n *html.Node) string {
	var out strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		out.WriteString(w.inlineNode(child))
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func (w *markdownWriter) inlineNode(n *html.Node) string {
	if n.Type == html.TextNode {
		text := strings.Join(strings.Fields(n.Data), " ")
		if text == "" {
			if strings.TrimSpace(n.Data) == "" && n.Data != "" {
				return " "
			}
			return ""
		}
		if strings.HasPrefix(n.Data, " ") || strings.HasPrefix(n.Data, "\n") {
			text = " " + text
		}
		if strings.HasSuffix(n.Data, " ") || strings.HasSuffix(n.Data, "\n") {
			text += " "
		}
		return text
	}
	if n.Type != html.ElementNode {
		return ""
	}
	inner := w.inline(n)
	switch n.DataAtom {
	case atom.Strong, atom.B:
		return wrap(inner, "**")
	case atom.Em, atom.I:
		return wrap(inner, "*")
	case atom.S, atom.Del, atom.Strike:
		return wrap(inner, "~~")
	case atom.Code, atom.Tt:
		return wrap(inner, "`")
	case atom.Br:
		return "  \n"
	case atom.Img:
		*w.pictures++
		return ""
	case atom.A:
		href := attr(n, "href")
		if href == "" || inner == "" {
			return inner
		}
		if address, ok := w.resolve(href); ok {
			return "[" + inner + "](" + address + ")"
		}
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "mailto:") {
			return "[" + inner + "](" + href + ")"
		}
		return inner
	case atom.Script, atom.Style:
		return ""
	}
	return inner
}

func wrap(text, marker string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return marker + strings.TrimSpace(text) + marker
}

func rawText(n *html.Node) string {
	var out strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(n)
	return out.String()
}

func collapseBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
