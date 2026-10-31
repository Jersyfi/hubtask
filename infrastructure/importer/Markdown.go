// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"maps"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Markdown converts a ZIP of Markdown files - a notes tool's folder export, a notes app's
// Markdown export - into the hub's pages in their tree (UC-IMP-05 check 6). A file is a page; its
// first heading, else its name, is its title. A folder's page is the file of the folder's name
// beside it, or its `index.md` or `README.md`; a page in a folder without one hangs under the
// nearest folder that has one. A link to another file of the export becomes a page reference, a
// picture of a file in it the page's file; anything else stays as written.
//
// Identities derive from each file's path, so a fresh export of the same files into the same hub
// lands on the pages the first one made. HTMLFiles and DOCX land their files the same way, each
// file read into Markdown first.
type Markdown struct{}

var _ service.Converter = Markdown{}

func (Markdown) Kind() domain.Kind { return domain.KindMarkdown }

func (Markdown) Convert(_ context.Context, source service.Source) (service.Result, error) {
	return convertFiles(source, markdownFiles)
}

var (
	// A notes app's export names a page "Title 0123456789abcdef0123456789abcdef.md".
	exportSuffix   = regexp.MustCompile(`\s+[0-9a-f]{32}$`)
	markdownLink   = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)(\s+"[^"]*")?\)`)
	markdownHeader = regexp.MustCompile(`^#\s+(.+?)\s*#*\s*$`)
)

type markdownPage struct {
	member string
	title  string
	body   string
	parent string
	depth  int
}

// fileFormat is how one kind of file becomes a page: the extensions it is known by (the first
// names a folder's page), how one file reads into Markdown - with the files it carries inside,
// a document's pictures, keyed by their path in the export - and whether an upload is one such
// file rather than a ZIP of them.
type fileFormat struct {
	identity   string
	extensions []string
	read       func(member string, content []byte, total *int64) (*markdownPage, map[string][]byte, error)
	single     func(raw []byte) bool
}

var markdownFiles = fileFormat{
	identity: "markdown", extensions: []string{".md", ".markdown"},
	read: func(member string, content []byte, _ *int64) (*markdownPage, map[string][]byte, error) {
		return readMarkdownPage(member, content), nil, nil
	},
}

func (f fileFormat) takes(member string) bool {
	return slices.Contains(f.extensions, strings.ToLower(path.Ext(member)))
}

func convertFiles(source service.Source, format fileFormat) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	pages := map[string]*markdownPage{}
	files := map[string][]byte{}
	var total int64
	take := func(name string, content []byte) error {
		if !format.takes(name) {
			files[name] = content
			return nil
		}
		page, held, err := format.read(name, content, &total)
		if err != nil {
			return err
		}
		if page != nil {
			pages[name] = page
		}
		maps.Copy(files, held)
		return nil
	}
	switch {
	case format.single != nil && format.single(raw):
		name := strings.TrimSpace(source.Name)
		if name == "" || strings.ContainsAny(name, "/\\") {
			name = "document"
		}
		if err := take(name+format.extensions[0], raw); err != nil {
			return service.Result{}, err
		}
	case !bytes.HasPrefix(raw, []byte("PK\x03\x04")):
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	default:
		reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
		}
		for _, member := range reader.File {
			name := path.Clean(strings.ReplaceAll(member.Name, "\\", "/"))
			if member.FileInfo().IsDir() || strings.HasPrefix(name, "../") || strings.HasPrefix(path.Base(name), ".") ||
				strings.HasPrefix(name, "__MACOSX/") {
				continue
			}
			content, err := readMember(member, &total)
			if err != nil {
				return service.Result{}, err
			}
			if err := take(name, content); err != nil {
				return service.Result{}, err
			}
		}
	}
	if len(pages) == 0 || len(pages) > knowledge.MaxPagesPerHub {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	members := make([]string, 0, len(pages))
	for member := range pages {
		members = append(members, member)
	}
	sort.Strings(members)
	for _, member := range members {
		pages[member].parent = folderPageOf(pages, member, format.extensions)
	}
	for _, member := range members {
		page := pages[member]
		for depth := markdownDepth(pages, page.parent); depth >= knowledge.MaxDepth && page.parent != ""; depth-- {
			page.parent = pages[page.parent].parent
		}
		page.depth = markdownDepth(pages, member)
	}
	sort.SliceStable(members, func(i, j int) bool { return pages[members[i]].depth < pages[members[j]].depth })

	b := newBuilder(source, format.identity)
	ids := map[string]shared.ID{}
	for _, member := range members {
		ids[member] = b.id("pages", "page:"+member)
	}
	var refused []refusal
	for row, member := range members {
		page := pages[member]
		dir := path.Dir(member)
		body := markdownLink.ReplaceAllStringFunc(page.body, func(link string) string {
			parts := markdownLink.FindStringSubmatch(link)
			target, ok := memberOf(dir, parts[3])
			if !ok {
				return link
			}
			if parts[1] == "!" {
				if content, held := files[target]; held {
					return "![" + parts[2] + "](hubtask:media/" + b.pageFile(ids[member], path.Base(target), content, source.Now).String() + ")"
				}
				return link
			}
			if id, known := ids[target]; known {
				return "[" + parts[2] + "](hubtask:page/" + id.String() + ")"
			}
			return link
		})
		parentKey := ""
		if page.parent != "" {
			parentKey = "page:" + page.parent
		}
		if _, ok := b.page(Page{Key: "page:" + member, ParentKey: parentKey, Title: page.title, Body: strings.TrimSpace(body)}); !ok {
			refused = append(refused, refusal{row: row + 1, code: domain.CodeRowTitleMissing})
		}
	}
	return b.result(refused), nil
}

// titleOfFile is a page's title from its file's name, a notes app's identifier suffix dropped.
func titleOfFile(member string) string {
	return exportSuffix.ReplaceAllString(strings.TrimSuffix(path.Base(member), path.Ext(member)), "")
}

// readMarkdownPage reads a file's title - its first line's heading, else its name - and its body
// without that heading.
func readMarkdownPage(member string, content []byte) *markdownPage {
	text := strings.ReplaceAll(strings.TrimPrefix(string(content), "\ufeff"), "\r\n", "\n")
	title := titleOfFile(member)
	lines := strings.SplitN(strings.TrimLeft(text, "\n"), "\n", 2)
	if match := markdownHeader.FindStringSubmatch(lines[0]); match != nil {
		title = match[1]
		text = ""
		if len(lines) == 2 {
			text = lines[1]
		}
	}
	return &markdownPage{member: member, title: title, body: text}
}

// folderPageOf answers the page of the nearest folder above the file that has one: the file of
// the folder's name beside it, or the folder's index or README in one of the format's extensions.
// A folder's own index or README starts above its folder, so a parent is always higher up.
func folderPageOf(pages map[string]*markdownPage, member string, extensions []string) string {
	start := path.Dir(member)
	base := strings.ToLower(strings.TrimSuffix(path.Base(member), path.Ext(member)))
	if base == "index" || base == "readme" {
		start = path.Dir(start)
	}
	for dir := start; dir != "." && dir != "/"; dir = path.Dir(dir) {
		above := path.Dir(dir)
		for _, extension := range extensions {
			for _, candidate := range []string{
				path.Join(dir, "index"+extension), path.Join(dir, "README"+extension), path.Join(above, path.Base(dir)+extension),
			} {
				if _, ok := pages[candidate]; ok && candidate != member {
					return candidate
				}
			}
		}
	}
	return ""
}

func markdownDepth(pages map[string]*markdownPage, member string) int {
	depth := 0
	for seen := 0; member != "" && seen <= knowledge.MaxDepth+1; seen++ {
		depth++
		member = pages[member].parent
	}
	return depth
}
