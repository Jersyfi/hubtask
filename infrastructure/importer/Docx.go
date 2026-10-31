// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DOCX converts one word-processor document, or a ZIP of them, into the hub's pages as Markdown
// does (UC-IMP-05 check 7). A document's headings, paragraphs, lists, tables, emphasis, outside
// links and pictures become its Markdown; its title is the one its properties name, else a
// paragraph styled as the title at its start, else its file's name. What a document holds beyond
// that - comments, tracked deletions, fields, text boxes' layout - is not carried.
type DOCX struct{}

var _ service.Converter = DOCX{}

func (DOCX) Kind() domain.Kind { return domain.KindDOCX }

func (DOCX) Convert(_ context.Context, source service.Source) (service.Result, error) {
	return convertFiles(source, docxFiles)
}

var docxFiles = fileFormat{
	identity: "docx", extensions: []string{".docx"}, read: readDocxPage,
	single: func(raw []byte) bool {
		reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return false
		}
		for _, member := range reader.File {
			if member.Name == "word/document.xml" {
				return true
			}
		}
		return false
	},
}

type docxRelation struct {
	target   string
	external bool
}

// docxRun is a stretch of text in one emphasis, inside one link or none.
type docxRun struct {
	text                 strings.Builder
	bold, italic, strike bool
	link                 string
}

type docxParagraph struct {
	style    string
	list     bool
	level    int
	runs     []*docxRun
	pictures []string
}

type docxReader struct {
	relations map[string]docxRelation
	styles    map[string]string
	media     map[string]*zip.File
	total     *int64
	// pictures are the document's pictures by their path in the export, beside the document.
	pictures map[string][]byte
	folder   string
	dir      string
	title    string
	out      strings.Builder
	listing  bool
}

func readDocxPage(member string, content []byte, total *int64) (*markdownPage, map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, nil, nil
	}
	parts := map[string]*zip.File{}
	for _, file := range archive.File {
		parts[file.Name] = file
	}
	if parts["word/document.xml"] == nil {
		return nil, nil, nil
	}
	r := &docxReader{
		relations: map[string]docxRelation{}, styles: map[string]string{}, media: parts, total: total,
		pictures: map[string][]byte{}, folder: titleOfFile(member) + ".media", dir: path.Dir(member),
	}
	if err := r.readRelations(parts["word/_rels/document.xml.rels"]); err != nil {
		return nil, nil, err
	}
	if err := r.readStyles(parts["word/styles.xml"]); err != nil {
		return nil, nil, err
	}
	title, err := docxTitle(parts["docProps/core.xml"], total)
	if err != nil {
		return nil, nil, err
	}
	document, err := readMember(parts["word/document.xml"], total)
	if err != nil {
		return nil, nil, err
	}
	if err := r.readBody(document); err != nil {
		return nil, nil, shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
	}
	switch {
	case title != "":
	case r.title != "":
		title = r.title
	default:
		title = titleOfFile(member)
	}
	return &markdownPage{member: member, title: title, body: collapseBlankLines(r.out.String())}, r.pictures, nil
}

func (r *docxReader) readRelations(file *zip.File) error {
	if file == nil {
		return nil
	}
	content, err := readMember(file, r.total)
	if err != nil {
		return err
	}
	var relations struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(content, &relations) != nil {
		return nil
	}
	for _, item := range relations.Items {
		external := strings.EqualFold(item.Mode, "External")
		target := item.Target
		if !external {
			target = strings.TrimPrefix(target, "/")
			if !strings.HasPrefix(item.Target, "/") {
				target = path.Join("word", target)
			}
		}
		r.relations[item.ID] = docxRelation{target: target, external: external}
	}
	return nil
}

func (r *docxReader) readStyles(file *zip.File) error {
	if file == nil {
		return nil
	}
	content, err := readMember(file, r.total)
	if err != nil {
		return err
	}
	var styles struct {
		Items []struct {
			ID   string `xml:"styleId,attr"`
			Name struct {
				Value string `xml:"val,attr"`
			} `xml:"name"`
		} `xml:"style"`
	}
	if xml.Unmarshal(content, &styles) != nil {
		return nil
	}
	for _, item := range styles.Items {
		r.styles[item.ID] = strings.ToLower(item.Name.Value)
	}
	return nil
}

func docxTitle(file *zip.File, total *int64) (string, error) {
	if file == nil {
		return "", nil
	}
	content, err := readMember(file, total)
	if err != nil {
		return "", err
	}
	var core struct {
		Title string `xml:"title"`
	}
	if xml.Unmarshal(content, &core) != nil {
		return "", nil
	}
	return strings.TrimSpace(core.Title), nil
}

func attrOf(element xml.StartElement, local string) string {
	for _, a := range element.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// on reads a property's switch: present means on, unless its value says off.
func on(element xml.StartElement) bool {
	switch strings.ToLower(attrOf(element, "val")) {
	case "0", "false", "off", "none":
		return false
	}
	return true
}

// readBody walks the document's body. Only the word-processing namespace's elements count, so a
// drawing's own text frames do not open paragraphs of their own.
func (r *docxReader) readBody(document []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	var (
		paragraph  *docxParagraph
		run        *docxRun
		link       string
		inText     bool
		tables     int
		rows       [][]string
		row, cell  []string
		properties int
	)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "blip" && paragraph != nil {
				r.picture(paragraph, attrOf(t, "embed"))
				continue
			}
			if !wordprocessing(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "tbl":
				tables++
				if tables == 1 {
					rows = nil
				}
			case "tr":
				if tables == 1 {
					row = nil
				}
			case "tc":
				if tables == 1 {
					cell = nil
				}
			case "p":
				paragraph = &docxParagraph{}
			case "pPr":
				properties++
			case "pStyle":
				if paragraph != nil {
					paragraph.style = attrOf(t, "val")
				}
			case "numPr":
				if paragraph != nil {
					paragraph.list = true
				}
			case "ilvl":
				if paragraph != nil {
					paragraph.level, _ = strconv.Atoi(attrOf(t, "val"))
				}
			case "hyperlink":
				link = ""
				if relation, ok := r.relations[attrOf(t, "id")]; ok && relation.external && external(relation.target) {
					link = relation.target
				}
			case "r":
				if paragraph != nil && properties == 0 {
					run = &docxRun{link: link}
					paragraph.runs = append(paragraph.runs, run)
				}
			case "b":
				if run != nil {
					run.bold = on(t)
				}
			case "i":
				if run != nil {
					run.italic = on(t)
				}
			case "strike", "dstrike":
				if run != nil {
					run.strike = on(t)
				}
			case "t":
				inText = run != nil
			case "tab":
				if run != nil {
					run.text.WriteString(" ")
				}
			case "br", "cr":
				if run != nil && attrOf(t, "type") != "page" {
					run.text.WriteString("  \n")
				}
			}
		case xml.CharData:
			if inText {
				run.text.Write(t)
			}
		case xml.EndElement:
			if !wordprocessing(t.Name.Space) {
				continue
			}
			switch t.Name.Local {
			case "t":
				inText = false
			case "r":
				run = nil
			case "pPr":
				properties--
			case "hyperlink":
				link = ""
			case "p":
				if paragraph == nil {
					continue
				}
				if tables > 0 {
					if text := strings.TrimSpace(paragraph.inline() + " " + strings.Join(paragraph.pictures, " ")); text != "" {
						cell = append(cell, text)
					}
				} else {
					r.write(paragraph)
				}
				paragraph = nil
			case "tc":
				if tables == 1 {
					row = append(row, strings.ReplaceAll(strings.Join(cell, " "), "|", "\\|"))
				}
			case "tr":
				if tables == 1 && len(row) > 0 {
					rows = append(rows, row)
				}
			case "tbl":
				tables--
				if tables == 0 {
					r.table(rows)
				}
			}
		}
	}
}

func wordprocessing(space string) bool {
	return strings.HasSuffix(space, "/wordprocessingml/2006/main") || strings.HasSuffix(space, "/wordprocessingml/main")
}

// picture keeps a drawn picture as a file beside the document, by a path the conversion resolves.
func (r *docxReader) picture(paragraph *docxParagraph, id string) {
	relation, ok := r.relations[id]
	if !ok || relation.external || r.media[relation.target] == nil {
		return
	}
	content, err := readMember(r.media[relation.target], r.total)
	if err != nil {
		return
	}
	name := path.Join(r.folder, path.Base(relation.target))
	r.pictures[path.Join(r.dir, name)] = content
	paragraph.pictures = append(paragraph.pictures, "![]("+(&url.URL{Path: name}).String()+")")
}

// inline renders the paragraph's runs, neighbours of one emphasis and link joined.
func (p *docxParagraph) inline() string {
	var out strings.Builder
	for i := 0; i < len(p.runs); {
		first := p.runs[i]
		var text strings.Builder
		for ; i < len(p.runs) && sameEmphasis(p.runs[i], first); i++ {
			text.WriteString(p.runs[i].text.String())
		}
		piece := text.String()
		core := strings.TrimSpace(piece)
		if core == "" {
			out.WriteString(piece)
			continue
		}
		lead, trail := piece[:strings.Index(piece, core)], piece[strings.Index(piece, core)+len(core):]
		if first.strike {
			core = "~~" + core + "~~"
		}
		if first.italic {
			core = "*" + core + "*"
		}
		if first.bold {
			core = "**" + core + "**"
		}
		if first.link != "" {
			core = "[" + core + "](" + first.link + ")"
		}
		out.WriteString(lead + core + trail)
	}
	return strings.TrimRight(out.String(), " ")
}

func sameEmphasis(a, b *docxRun) bool {
	return a.bold == b.bold && a.italic == b.italic && a.strike == b.strike && a.link == b.link
}

// headingLevel reads a paragraph's style as a heading: "heading N" by the style's name, which a
// localised document keeps while its identifiers change; 0 is no heading, -1 the title.
func (r *docxReader) headingLevel(style string) int {
	name := r.styles[style]
	if name == "" {
		name = strings.ToLower(style)
	}
	if name == "title" {
		return -1
	}
	if level, ok := strings.CutPrefix(strings.ReplaceAll(name, " ", ""), "heading"); ok {
		if n, err := strconv.Atoi(level); err == nil && n >= 1 {
			return min(n, 6)
		}
	}
	return 0
}

func (r *docxReader) write(p *docxParagraph) {
	text := strings.TrimSpace(p.inline())
	level := r.headingLevel(p.style)
	list := p.list || r.styles[p.style] == "list paragraph"
	if list && text != "" && level == 0 {
		if !r.listing {
			r.out.WriteString("\n\n")
		}
		r.listing = true
		r.out.WriteString(strings.Repeat("  ", min(max(p.level, 0), 8)) + "- " + text + "\n")
	} else if text != "" {
		r.listing = false
		switch {
		case level == -1 && r.title == "" && strings.TrimSpace(r.out.String()) == "":
			r.title = text
		case level != 0:
			r.out.WriteString("\n\n" + strings.Repeat("#", max(level, 1)) + " " + text + "\n\n")
		default:
			r.out.WriteString("\n\n" + text + "\n\n")
		}
	}
	for _, picture := range p.pictures {
		r.listing = false
		r.out.WriteString("\n\n" + picture + "\n\n")
	}
}

func (r *docxReader) table(rows [][]string) {
	if len(rows) == 0 {
		return
	}
	r.listing = false
	width := 0
	for _, cells := range rows {
		width = max(width, len(cells))
	}
	r.out.WriteString("\n\n")
	for i, cells := range rows {
		for len(cells) < width {
			cells = append(cells, "")
		}
		r.out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			r.out.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
		}
	}
	r.out.WriteString("\n")
}
