// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const docxBody = `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
  xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Titel"/></w:pPr><w:r><w:t>Onboarding</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="berschrift1"/><w:rPr><w:b/></w:rPr></w:pPr><w:r><w:t>First day</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">Bring </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>your badge</w:t></w:r><w:r><w:t xml:space="preserve"> and read </w:t></w:r><w:hyperlink r:id="rId2"><w:r><w:t>the guide</w:t></w:r></w:hyperlink><w:r><w:t>.</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>Laptop</w:t></w:r></w:p>
<w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>charger</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Room</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Floor</w:t></w:r></w:p></w:tc></w:tr>
<w:tr><w:tc><w:p><w:r><w:t>A|1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:p><w:r><w:drawing><a:graphic><a:graphicData><a:blip r:embed="rId3"/></a:graphicData></a:graphic></w:drawing></w:r></w:p>
<w:p><w:r><w:delText>gone</w:delText></w:r><w:r><w:rPr><w:b w:val="0"/></w:rPr><w:t>The end</w:t></w:r></w:p>
</w:body></w:document>`

const docxStyles = `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:style w:styleId="Titel"><w:name w:val="Title"/></w:style>
<w:style w:styleId="berschrift1"><w:name w:val="heading 1"/></w:style>
</w:styles>`

const docxRelations = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId2" Type="hyperlink" Target="https://example.org/guide" TargetMode="External"/>
<Relationship Id="rId3" Type="image" Target="media/image1.png"/>
</Relationships>`

func docxFile(t *testing.T, body string) []byte {
	t.Helper()
	return markdownZip(t, map[string]string{
		"[Content_Types].xml":          "<Types/>",
		"word/document.xml":            body,
		"word/styles.xml":              docxStyles,
		"word/_rels/document.xml.rels": docxRelations,
		"word/media/image1.png":        "\x89PNG\r\n\x1a\npng bytes",
	})
}

// UC-IMP-05 check 7: a document becomes a page - its title, headings by their style's name,
// emphasis, an outside link, a list, a table and its picture as the page's file.
func TestADocumentBecomesAPage(t *testing.T) {
	source := trackerSource(t, docxFile(t, docxBody))
	source.Name = "onboarding"
	result, err := DOCX{}.Convert(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	page := titled(result.Records["pages"], "Onboarding")
	if page == nil {
		t.Fatalf("pages: %v", result.Records["pages"])
	}
	body := page["body"].(string)
	for _, want := range []string{
		"# First day\n",
		"Bring **your badge** and read [the guide](https://example.org/guide).",
		"- Laptop\n  - *charger*\n",
		"| Room | Floor |\n| --- | --- |\n| A\\|1 | 2 |",
		"![](hubtask:media/",
		"The end",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "gone") || strings.Contains(body, "Onboarding") {
		t.Errorf("a deletion or the title is in the body:\n%s", body)
	}
	if len(result.Records["page_media"]) != 1 {
		t.Errorf("files held: %d", len(result.Records["page_media"]))
	}
}

// A ZIP of documents lands them in their folders' tree, as Markdown files do.
func TestAFolderOfDocumentsBecomesTheHubsPages(t *testing.T) {
	plain := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>text</w:t></w:r></w:p></w:body></w:document>`
	result, err := DOCX{}.Convert(context.Background(), trackerSource(t, markdownZip(t, map[string]string{
		"Policies.docx":        string(docxFile(t, plain)),
		"Policies/Travel.docx": string(docxFile(t, plain)),
	})))
	if err != nil {
		t.Fatal(err)
	}
	policies, travel := titled(result.Records["pages"], "Policies"), titled(result.Records["pages"], "Travel")
	if policies == nil || travel == nil || travel["parent_id"] != policies["id"] {
		t.Fatalf("tree: %v", result.Records["pages"])
	}
}

func TestWhatIsNoDocumentIsRefused(t *testing.T) {
	for name, content := range map[string][]byte{
		"not a zip":          []byte("plain text"),
		"a zip of no docx":   markdownZip(t, map[string]string{"a.txt": "x"}),
		"a docx of no words": markdownZip(t, map[string]string{"x.docx": "not a zip"}),
	} {
		_, err := DOCX{}.Convert(context.Background(), trackerSource(t, content))
		if !errors.Is(err, shared.ErrValidation) || !strings.Contains(err.Error(), domain.CodeFileNotKind) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
