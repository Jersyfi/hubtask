// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"bytes"
	"encoding/xml"
	stdhtml "html"
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

// The XML export names each object of the space by its class: a page's current and earlier
// versions, the bodies they hold, their comments, attachments and the people who wrote them.
// Its attachments lie under attachments/<page>/<attachment>/<version>.
type xmlObject struct {
	Class       string          `xml:"class,attr"`
	ID          xmlValue        `xml:"id"`
	Properties  []xmlProperty   `xml:"property"`
	Collections []xmlCollection `xml:"collection"`
}

type xmlValue struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

type xmlProperty struct {
	Name  string    `xml:"name,attr"`
	Class string    `xml:"class,attr"`
	Value string    `xml:",chardata"`
	ID    *xmlValue `xml:"id"`
}

type xmlCollection struct {
	Name     string `xml:"name,attr"`
	Elements []struct {
		Class string   `xml:"class,attr"`
		ID    xmlValue `xml:"id"`
	} `xml:"element"`
}

func (o xmlObject) text(name string) string {
	for _, p := range o.Properties {
		if p.Name == name {
			if p.ID != nil {
				return strings.TrimSpace(p.ID.Value)
			}
			return p.Value
		}
	}
	return ""
}

func (o xmlObject) ref(name string) string {
	for _, p := range o.Properties {
		if p.Name == name && p.ID != nil {
			return strings.TrimSpace(p.ID.Value)
		}
	}
	return ""
}

func (o xmlObject) key() string { return strings.TrimSpace(o.ID.Value) }

// xmlDate reads the export's moments, written in the server's zone without one; they are read as
// UTC, the nearest the export allows.
func xmlDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339Nano} {
		if at, err := time.Parse(layout, value); err == nil {
			at = at.UTC()
			return &at
		}
	}
	return nil
}

type xmlPage struct {
	object   xmlObject
	title    string
	parent   string
	position int
	created  *time.Time
	updated  *time.Time
	body     string
	history  []xmlObject
}

// convertXML converts a space's XML export: the pages in their tree with every earlier version as
// the page's history, their comments and replies with the words an inline comment was written
// about, and their attachments (UC-IMP-06). A draft, a page in the trash and a comment removed are
// not carried.
func (c Wiki) convertXML(source service.Source, entities []byte, attachments map[string][]byte) (service.Result, error) {
	decoder := xml.NewDecoder(bytes.NewReader(entities))
	decoder.Strict = false
	objects := map[string][]xmlObject{}
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "object" {
			continue
		}
		var object xmlObject
		if err := decoder.DecodeElement(&object, &start); err != nil {
			return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
		}
		objects[object.Class] = append(objects[object.Class], object)
	}
	bodies := map[string]string{}
	for _, body := range objects["BodyContent"] {
		if owner := body.ref("content"); owner != "" {
			bodies[owner] = body.text("body")
		}
	}
	people := map[string]string{}
	for _, person := range objects["ConfluenceUserImpl"] {
		people[person.key()] = strings.TrimSpace(person.text("name"))
	}
	space := ""
	for _, s := range objects["Space"] {
		if name := strings.TrimSpace(s.text("name")); name != "" {
			space = name
			break
		}
	}

	pages := map[string]*xmlPage{}
	byTitle := map[string]string{}
	var historical []xmlObject
	for _, object := range objects["Page"] {
		if object.ref("originalVersion") != "" {
			historical = append(historical, object)
			continue
		}
		if status := strings.TrimSpace(object.text("contentStatus")); status != "" && status != "current" {
			continue
		}
		page := &xmlPage{
			object: object, title: strings.TrimSpace(object.text("title")), parent: object.ref("parent"),
			created: xmlDate(object.text("creationDate")), updated: xmlDate(object.text("lastModificationDate")),
			body: bodies[object.key()],
		}
		page.position, _ = strconv.Atoi(strings.TrimSpace(object.text("position")))
		pages[object.key()] = page
		byTitle[strings.ToLower(page.title)] = object.key()
	}
	if len(pages) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}
	for _, version := range historical {
		if page, ok := pages[version.ref("originalVersion")]; ok {
			page.history = append(page.history, version)
		}
	}

	b := newBuilder(source, "wiki:"+space)
	ids := map[string]shared.ID{}
	keys := make([]string, 0, len(pages))
	for key := range pages {
		ids[key] = b.id("pages", "page:"+key)
		keys = append(keys, key)
	}
	depth := func(key string) int {
		d := 0
		for key != "" && d <= knowledge.MaxDepth {
			page, ok := pages[key]
			if !ok {
				break
			}
			key, d = page.parent, d+1
		}
		return d
	}
	sort.SliceStable(keys, func(i, j int) bool {
		di, dj := depth(keys[i]), depth(keys[j])
		if di != dj {
			return di < dj
		}
		if pages[keys[i]].position != pages[keys[j]].position {
			return pages[keys[i]].position < pages[keys[j]].position
		}
		return pages[keys[i]].title < pages[keys[j]].title
	})

	// Attachments by page and file name, the newest version each.
	type stored struct {
		member  string
		version int
	}
	files := map[string]map[string]stored{}
	attachmentNames := map[string]string{}
	for _, attachment := range objects["Attachment"] {
		if attachment.ref("originalVersion") != "" {
			continue
		}
		attachmentNames[attachment.key()] = strings.TrimSpace(attachment.text("title"))
	}
	for member := range attachments {
		parts := strings.Split(member, "/")
		at := indexOf(parts, "attachments")
		if at < 0 || len(parts) < at+3 {
			continue
		}
		page, attachment := parts[at+1], parts[at+2]
		name := attachmentNames[attachment]
		if name == "" {
			continue
		}
		version := 0
		if len(parts) > at+3 {
			version, _ = strconv.Atoi(parts[at+3])
		}
		if files[page] == nil {
			files[page] = map[string]stored{}
		}
		if known, ok := files[page][name]; !ok || version > known.version {
			files[page][name] = stored{member: member, version: version}
		}
	}

	unmapped := map[string]int{}
	held := map[string]bool{}
	var refused []refusal
	for row, key := range keys {
		page := pages[key]
		parent := page.parent
		if _, known := pages[parent]; !known {
			parent = ""
		}
		for d := depth(parent); d >= knowledge.MaxDepth && parent != ""; d-- {
			parent = pages[parent].parent
		}
		page.parent = parent
		at := source.Now
		if page.updated != nil {
			at = *page.updated
		}
		pictures, earlier := 0, 0
		drawn := map[string]bool{}
		hold := func(member, name string) string {
			held[member] = true
			return "hubtask:media/" + b.pageFile(ids[key], name, attachments[member], at).String()
		}
		// An earlier version draws no file: only the current content's pictures are carried.
		render := func(storage string, live bool) string {
			counter := &earlier
			if live {
				counter = &pictures
			}
			w := &markdownWriter{
				resolve: func(href string) (string, bool) {
					if title, ok := strings.CutPrefix(href, "wiki-page:"); ok {
						if target, known := byTitle[strings.ToLower(title)]; known {
							return "hubtask:page/" + ids[target].String(), true
						}
					}
					return "", false
				},
				picture: func(src string) (string, bool) {
					name, ok := strings.CutPrefix(src, "wiki-attachment:")
					if !ok || !live {
						return "", false
					}
					file, stored := files[key][name]
					if !stored {
						return "", false
					}
					drawn[file.member] = true
					return hold(file.member, name), true
				},
				pictures: counter,
			}
			w.children(storageDocument(storage), "")
			w.flush("")
			return strings.TrimSpace(collapseBlankLines(w.out.String()))
		}
		body := render(page.body, true)
		unmapped["pictures"] += pictures
		var rest []string
		names := make([]string, 0, len(files[key]))
		for name := range files[key] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if file := files[key][name]; !drawn[file.member] {
				rest = append(rest, "![]("+hold(file.member, name)+")")
			}
		}
		if len(rest) > 0 {
			body = strings.TrimSpace(body + "\n\n" + strings.Join(rest, "\n\n"))
		}
		sort.SliceStable(page.history, func(i, j int) bool {
			vi, _ := strconv.Atoi(strings.TrimSpace(page.history[i].text("version")))
			vj, _ := strconv.Atoi(strings.TrimSpace(page.history[j].text("version")))
			return vi < vj
		})
		var versions []PageVersion
		for _, version := range page.history {
			when := source.Now
			if moment := xmlDate(version.text("lastModificationDate")); moment != nil {
				when = *moment
			}
			versions = append(versions, PageVersion{
				Title: version.text("title"), Body: render(bodies[version.key()], false),
				Name: version.text("versionComment"), At: when,
			})
		}
		parentKey := ""
		if parent != "" {
			parentKey = "page:" + parent
		}
		if _, ok := b.page(Page{Key: "page:" + key, ParentKey: parentKey, Title: page.title, Body: body, CreatedAt: page.created, UpdatedAt: page.updated, Versions: versions}); !ok {
			refused = append(refused, refusal{row: row + 1, code: domain.CodeRowTitleMissing})
		}
	}
	if len(b.records["pages"]) > knowledge.MaxPagesPerHub {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	// Comments: threads first, then their replies; the quote of an inline comment from its
	// property.
	quotes := map[string]string{}
	for _, property := range objects["ContentProperty"] {
		if strings.TrimSpace(property.text("name")) == "inline-original-selection" {
			quotes[property.ref("content")] = property.text("stringValue")
		}
	}
	comments := objects["Comment"]
	sort.SliceStable(comments, func(i, j int) bool {
		pi, pj := comments[i].ref("parent") != "", comments[j].ref("parent") != ""
		if pi != pj {
			return !pi
		}
		return comments[i].text("creationDate") < comments[j].text("creationDate")
	})
	for _, comment := range comments {
		if status := strings.TrimSpace(comment.text("contentStatus")); status != "" && status != "current" {
			continue
		}
		page, ok := ids[comment.ref("containerContent")]
		if !ok || !b.pageKnown(page) {
			unmapped["comments"]++
			continue
		}
		when := source.Now
		if moment := xmlDate(comment.text("creationDate")); moment != nil {
			when = *moment
		}
		discard := 0
		w := &markdownWriter{resolve: func(string) (string, bool) { return "", false }, pictures: &discard}
		w.children(storageDocument(bodies[comment.key()]), "")
		text := strings.TrimSpace(collapseBlankLines(w.out.String()))
		parent := comment.ref("parent")
		if !b.pageComment(comment.key(), page, parent, people[comment.ref("creator")], text, quotes[comment.key()], when) {
			unmapped["comments"]++
		}
	}
	for member := range attachments {
		if !held[member] {
			unmapped["attachments"]++
		}
	}
	result := b.result(refused)
	result.Unmapped = unmapped
	return result, nil
}

// pageKnown reports whether the builder holds a page of that identity.
func (b *builder) pageKnown(id shared.ID) bool {
	for _, record := range b.records["pages"] {
		if record.ID == id.String() {
			return true
		}
	}
	return false
}

var (
	storageCDATA     = regexp.MustCompile(`(?s)<!\[CDATA\[(.*?)\]\]>`)
	storageSelfClose = regexp.MustCompile(`<((?:ac|ri):[A-Za-z-]+)([^<>]*?)/>`)
)

// storageDocument parses a body in the wiki's storage format - XHTML with its own elements for
// pictures, links between pages and macros - into the HTML the Markdown writer reads: a picture
// of an attachment as an image of wiki-attachment:<name>, a link to a page as a link to
// wiki-page:<title>, a code macro as preformatted text and every other macro as its rich body.
func storageDocument(storage string) *html.Node {
	storage = storageCDATA.ReplaceAllStringFunc(storage, func(section string) string {
		return stdhtml.EscapeString(storageCDATA.FindStringSubmatch(section)[1])
	})
	storage = storageSelfClose.ReplaceAllString(storage, "<$1$2></$1>")
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(storage), root)
	if err != nil {
		return root
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	rewriteStorage(root)
	return root
}

func rewriteStorage(n *html.Node) {
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode {
			if replacement := storageElement(child); replacement != child {
				if replacement != nil {
					n.InsertBefore(replacement, child)
				}
				n.RemoveChild(child)
				child = next
				continue
			}
			rewriteStorage(child)
		}
		child = next
	}
}

// storageElement answers what a storage element becomes: itself when it is plain HTML, a new
// node, or nil when it carries nothing to read.
func storageElement(n *html.Node) *html.Node {
	element := func(a atom.Atom, attrs ...html.Attribute) *html.Node {
		return &html.Node{Type: html.ElementNode, Data: a.String(), DataAtom: a, Attr: attrs}
	}
	textNode := func(text string) *html.Node { return &html.Node{Type: html.TextNode, Data: text} }
	child := func(name string) *html.Node {
		return find(n, func(c *html.Node) bool { return c != n && c.Data == name })
	}
	switch n.Data {
	case "ac:image":
		if attachment := child("ri:attachment"); attachment != nil {
			return element(atom.Img, html.Attribute{Key: "src", Val: "wiki-attachment:" + path.Base(attr(attachment, "ri:filename"))})
		}
		if address := child("ri:url"); address != nil {
			return element(atom.Img, html.Attribute{Key: "src", Val: attr(address, "ri:value")})
		}
		return nil
	case "ac:link":
		label := ""
		if body := child("ac:plain-text-link-body"); body != nil {
			label = textOf(body)
		} else if body := child("ac:link-body"); body != nil {
			label = textOf(body)
		}
		if page := child("ri:page"); page != nil {
			title := attr(page, "ri:content-title")
			if label == "" {
				label = title
			}
			link := element(atom.A, html.Attribute{Key: "href", Val: "wiki-page:" + title})
			link.AppendChild(textNode(label))
			return link
		}
		if label == "" {
			if attachment := child("ri:attachment"); attachment != nil {
				label = attr(attachment, "ri:filename")
			}
		}
		return textNode(label)
	case "ac:structured-macro", "ac:macro":
		name := ""
		for _, a := range n.Attr {
			if a.Key == "ac:name" {
				name = a.Val
			}
		}
		if name == "code" || name == "noformat" {
			pre := element(atom.Pre)
			if body := child("ac:plain-text-body"); body != nil {
				pre.AppendChild(textNode(rawText(body)))
			}
			return pre
		}
		if body := child("ac:rich-text-body"); body != nil {
			section := element(atom.Div)
			for c := body.FirstChild; c != nil; {
				next := c.NextSibling
				body.RemoveChild(c)
				section.AppendChild(c)
				c = next
			}
			rewriteStorage(section)
			return section
		}
		return nil
	case "ac:parameter", "ac:emoticon", "ac:placeholder":
		return nil
	case "ac:task-list":
		list := element(atom.Ul)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Data != "ac:task" {
				continue
			}
			item := element(atom.Li)
			mark := "[ ] "
			if status := find(c, func(s *html.Node) bool { return s.Data == "ac:task-status" }); status != nil && strings.TrimSpace(textOf(status)) == "complete" {
				mark = "[x] "
			}
			item.AppendChild(textNode(mark))
			if body := find(c, func(s *html.Node) bool { return s.Data == "ac:task-body" }); body != nil {
				item.AppendChild(textNode(textOf(body)))
			}
			list.AppendChild(item)
		}
		return list
	}
	if strings.HasPrefix(n.Data, "ac:") || strings.HasPrefix(n.Data, "ri:") {
		section := element(atom.Span)
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			n.RemoveChild(c)
			section.AppendChild(c)
			c = next
		}
		rewriteStorage(section)
		return section
	}
	return n
}
