// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
)

// xlsxType is the media type of a spreadsheet in the Office Open XML format (ECMA-376).
const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// The fixed parts of a workbook of one sheet: the smallest package a spreadsheet opens. Written
// here with the standard library rather than through a spreadsheet library - one sheet of plain
// cells needs five small parts and no formatting, so no dependency is carried for it.
const (
	xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`
	xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`
	xlsxSheetOpen  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`
	xlsxSheetClose = `</sheetData></worksheet>`
)

// xlsxWorkbook is the workbook part naming its one sheet; the name is a fixed word, never content.
func xlsxWorkbook(sheet string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` +
		xmlText(sheet) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`
}

// writeXlsx streams one sheet: the header row, then each row. A text is an inline string, which a
// spreadsheet never evaluates - no formula prefix is needed, unlike CSV; a number and a boolean
// keep their type, so a sum over the minutes works as it opens.
func writeXlsx(w io.Writer, sheet string, header []string, rows func(yield func([]any) error) error) error {
	archive := zip.NewWriter(w)
	for _, part := range []struct{ name, body string }{
		{"[Content_Types].xml", xlsxContentTypes},
		{"_rels/.rels", xlsxRootRels},
		{"xl/workbook.xml", xlsxWorkbook(sheet)},
		{"xl/_rels/workbook.xml.rels", xlsxWorkbookRels},
	} {
		f, err := archive.Create(part.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, part.body); err != nil {
			return err
		}
	}
	f, err := archive.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, xlsxSheetOpen); err != nil {
		return err
	}
	line := 0
	write := func(cells []any) error {
		line++
		var b strings.Builder
		b.WriteString(`<row r="` + strconv.Itoa(line) + `">`)
		for i, cell := range cells {
			ref := xlsxColumn(i) + strconv.Itoa(line)
			switch v := cell.(type) {
			case nil:
				continue
			case bool:
				flag := "0"
				if v {
					flag = "1"
				}
				b.WriteString(`<c r="` + ref + `" t="b"><v>` + flag + `</v></c>`)
			case int:
				b.WriteString(`<c r="` + ref + `"><v>` + strconv.Itoa(v) + `</v></c>`)
			case float64:
				b.WriteString(`<c r="` + ref + `"><v>` + strconv.FormatFloat(v, 'f', -1, 64) + `</v></c>`)
			case string:
				b.WriteString(`<c r="` + ref + `" t="inlineStr"><is><t xml:space="preserve">` + xmlText(v) + `</t></is></c>`)
			}
		}
		b.WriteString(`</row>`)
		_, err := io.WriteString(f, b.String())
		return err
	}
	head := make([]any, len(header))
	for i, name := range header {
		head[i] = name
	}
	if err := write(head); err != nil {
		return err
	}
	if err := rows(write); err != nil {
		return err
	}
	if _, err := io.WriteString(f, xlsxSheetClose); err != nil {
		return err
	}
	return archive.Close()
}

// xlsxColumn is a column's letters: A … Z, AA … .
func xlsxColumn(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}

// xmlText escapes a text for an element or an attribute, and drops what XML 1.0 cannot carry.
func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r != 0xFFFE && r != 0xFFFF {
			return r
		}
		return -1
	}, s)))
	return b.String()
}
