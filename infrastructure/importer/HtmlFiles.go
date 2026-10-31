// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
)

// HTMLFiles converts one HTML file, or a ZIP of them - a notes app's HTML export, a saved site -
// into the hub's pages as Markdown does (UC-IMP-05 check 7): a file's title is its `<title>`,
// else its name, and its body becomes the page's Markdown in the grammar a wiki's pages get.
type HTMLFiles struct{}

var _ service.Converter = HTMLFiles{}

func (HTMLFiles) Kind() domain.Kind { return domain.KindHTML }

func (HTMLFiles) Convert(_ context.Context, source service.Source) (service.Result, error) {
	return convertFiles(source, htmlFiles)
}

var htmlFiles = fileFormat{
	identity: "html", extensions: []string{".html", ".htm"}, read: readHTMLPage,
	single: func(raw []byte) bool {
		head := bytes.ToLower(raw[:min(len(raw), 1024)])
		return !bytes.HasPrefix(raw, []byte("PK\x03\x04")) &&
			(bytes.Contains(head, []byte("<html")) || bytes.Contains(head, []byte("<!doctype html")) || bytes.Contains(head, []byte("<body")))
	},
}

// readHTMLPage reads one file: the heading that repeats its title is dropped, since the page shows
// its title already. A link or picture inside the export is kept by its path, which the
// conversion then resolves; a picture from outside is left out.
func readHTMLPage(member string, content []byte, _ *int64) (*markdownPage, map[string][]byte, error) {
	document, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, nil, nil
	}
	title := titleOfFile(member)
	if node := find(document, func(n *html.Node) bool { return n.DataAtom == atom.Title }); node != nil {
		if text := strings.TrimSpace(textOf(node)); text != "" {
			title = text
		}
	}
	body := find(document, func(n *html.Node) bool { return n.DataAtom == atom.Body })
	if body == nil {
		body = document
	}
	if heading := find(body, func(n *html.Node) bool { return n.DataAtom == atom.H1 }); heading != nil &&
		strings.TrimSpace(textOf(heading)) == title && heading.Parent != nil {
		heading.Parent.RemoveChild(heading)
	}
	pictures := 0
	writer := &markdownWriter{resolve: insideExport, picture: insideExport, pictures: &pictures}
	return &markdownPage{member: member, title: title, body: markdownOf(body, writer)}, nil, nil
}

// insideExport keeps a link to a path of the export, escaped so that the Markdown link reads it
// back whole; anything with a scheme or a host is not the export's.
func insideExport(href string) (string, bool) {
	address, err := url.Parse(strings.TrimSpace(href))
	if err != nil || address.Scheme != "" || address.Host != "" || address.Path == "" {
		return "", false
	}
	return (&url.URL{Path: address.Path}).String(), true
}
