// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"strconv"
	"strings"
	"unicode"
)

// File is one exported page: where it stands in the exported tree and its content.
type File struct {
	PageID  string
	Path    string
	Content string
}

// MarkdownFile writes a page as a Markdown file with front matter: its identifier, its title and
// its parent, so that an import elsewhere - or here again - rebuilds the tree (UC-DOC-09). The body
// is the page's projection as stored; a reference stays its `hubtask:` address.
func MarkdownFile(page Page) string {
	var out strings.Builder
	out.WriteString("---\n")
	out.WriteString("id: " + page.ID.String() + "\n")
	out.WriteString("title: " + strconv.Quote(page.Title) + "\n")
	if !page.ParentID.IsZero() {
		out.WriteString("parent: " + page.ParentID.String() + "\n")
	}
	if page.Template {
		out.WriteString("template: true\n")
	}
	if page.ContentLanguage != "" {
		out.WriteString("language: " + page.ContentLanguage + "\n")
	}
	out.WriteString("---\n\n")
	out.WriteString(page.Body)
	if page.Body != "" && !strings.HasSuffix(page.Body, "\n") {
		out.WriteString("\n")
	}
	return out.String()
}

// Tree writes a page and the pages under it as Markdown files, each under a folder named after its
// parent: `Handbook.md`, `Handbook/Onboarding.md`. pages is the subtree, root first; a page whose
// parent is not among them is left out. Two siblings of one name are told apart by a number.
func Tree(pages []Page, rootID string) []File {
	byParent := map[string][]Page{}
	var root *Page
	for i := range pages {
		if pages[i].ID.String() == rootID {
			root = &pages[i]
			continue
		}
		parent := pages[i].ParentID.String()
		byParent[parent] = append(byParent[parent], pages[i])
	}
	if root == nil {
		return nil
	}
	var files []File
	var walk func(page Page, folder string, taken map[string]int)
	walk = func(page Page, folder string, taken map[string]int) {
		name := FileName(page.Title)
		if n := taken[strings.ToLower(name)]; n > 0 {
			taken[strings.ToLower(name)] = n + 1
			name += "-" + strconv.Itoa(n+1)
		} else {
			taken[strings.ToLower(name)] = 1
		}
		files = append(files, File{PageID: page.ID.String(), Path: folder + name + ".md", Content: MarkdownFile(page)})
		children := map[string]int{}
		for _, child := range byParent[page.ID.String()] {
			walk(child, folder+name+"/", children)
		}
	}
	walk(*root, "", map[string]int{})
	return files
}

// FileName is a title as a file name every file system takes: letters, digits, spaces, dashes and
// underscores, bounded, never empty.
func FileName(title string) string {
	var out strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			out.WriteRune(r)
		case r == ' ', r == '.':
			out.WriteRune(' ')
		}
		if out.Len() >= 80 {
			break
		}
	}
	name := strings.Join(strings.Fields(out.String()), " ")
	if name == "" {
		return "page"
	}
	return name
}
