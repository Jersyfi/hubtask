// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package spreadsheet

import (
	"archive/zip"
	"bytes"
	"errors"
	"reflect"
	"testing"
)

const (
	workbookPart = `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Assets" sheetId="1" r:id="rId7"/><sheet name="Other" sheetId="2" r:id="rId8"/></sheets></workbook>`
	relsPart = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Target="worksheets/sheet2.xml"/><Relationship Id="rId7" Target="worksheets/sheet1.xml"/></Relationships>`
	sharedPart = `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>name</t></si><si><t>purchased</t></si><si><t>price</t></si><si><r><t>Laptop </t></r><r><t>14"</t></r></si></sst>`
	stylesPart = `<?xml version="1.0" encoding="UTF-8"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts><numFmt numFmtId="164" formatCode="dd.mm.yyyy"/><numFmt numFmtId="165" formatCode="#,##0.00 &quot;EUR&quot;"/></numFmts><cellXfs><xf numFmtId="0"/><xf numFmtId="164"/><xf numFmtId="165"/><xf numFmtId="22"/></cellXfs></styleSheet>`
	sheetPart = `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="inlineStr"><is><t>active</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>3</v></c><c r="B2" s="1"><v>46325</v></c><c r="C2" s="2"><v>1299.5</v></c><c r="D2" t="b"><v>1</v></c></row>
<row r="4"><c r="A4" t="str"><f>A2</f><v>Dock</v></c><c r="C4"><v>89</v></c><c r="E4" s="3"><v>46325.5</v></c></row>
</sheetData></worksheet>`
)

func workbook(t testing.TB, parts map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for name, body := range parts {
		f, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func register(t testing.TB) []byte {
	return workbook(t, map[string]string{
		"xl/workbook.xml": workbookPart, "xl/_rels/workbook.xml.rels": relsPart,
		"xl/sharedStrings.xml": sharedPart, "xl/styles.xml": stylesPart,
		"xl/worksheets/sheet1.xml": sheetPart, "xl/worksheets/sheet2.xml": `<worksheet><sheetData/></worksheet>`,
	})
}

// A register kept in a spreadsheet reads as its CSV export would: dates as ISO dates rather than a
// locale's day numbers, numbers as stored, a formula as its value, a gap as an empty row or cell.
func TestTheFirstSheetReadsAsRowsOfText(t *testing.T) {
	rows, err := Rows(register(t))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"name", "purchased", "price", "active"},
		{`Laptop 14"`, "2026-10-30", "1299.5", "true"},
		nil,
		{"Dock", "", "89", "", "2026-10-30T12:00:00"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows\n%q\nwant\n%q", rows, want)
	}
}

func TestA1904WorkbookCountsFromItsOwnEpoch(t *testing.T) {
	if got := serialDate(0, true); got != "1904-01-01" {
		t.Errorf("day 0 of 1904 is %s", got)
	}
	if got := serialDate(61, false); got != "1900-03-01" {
		t.Errorf("day 61 of 1900 is %s", got)
	}
}

func TestWhatIsNoWorkbookIsRefused(t *testing.T) {
	for name, data := range map[string][]byte{
		"not a zip":   []byte("name,price\nDock,89\n"),
		"no workbook": workbook(t, map[string]string{"xl/worksheets/sheet1.xml": sheetPart}),
		"a shared string out of range": workbook(t, map[string]string{
			"xl/workbook.xml": workbookPart, "xl/_rels/workbook.xml.rels": relsPart,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>9</v></c></row></sheetData></worksheet>`,
		}),
		"a column past the bound": workbook(t, map[string]string{
			"xl/workbook.xml": workbookPart, "xl/_rels/workbook.xml.rels": relsPart,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="ZZZ1"><v>1</v></c></row></sheetData></worksheet>`,
		}),
		"rows out of order": workbook(t, map[string]string{
			"xl/workbook.xml": workbookPart, "xl/_rels/workbook.xml.rels": relsPart,
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="3"/><row r="2"/></sheetData></worksheet>`,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Rows(data); !errors.Is(err, ErrUnreadable) {
				t.Errorf("answered %v", err)
			}
		})
	}
}

// Whatever bytes arrive, the reader answers rows or ErrUnreadable within its bounds - never a
// panic and never a table past MaxRows by MaxColumns.
func FuzzRows(f *testing.F) {
	f.Add(register(f))
	f.Add([]byte("PK\x03\x04"))
	f.Add(workbook(f, map[string]string{
		"xl/workbook.xml": workbookPart, "xl/_rels/workbook.xml.rels": relsPart,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>-1</v></c></row></sheetData></worksheet>`,
	}))
	f.Fuzz(func(t *testing.T, data []byte) {
		rows, err := Rows(data)
		if err != nil {
			if !errors.Is(err, ErrUnreadable) {
				t.Fatalf("an error that is not ErrUnreadable: %v", err)
			}
			return
		}
		if len(rows) > MaxRows {
			t.Fatalf("%d rows", len(rows))
		}
		for _, row := range rows {
			if len(row) > MaxColumns {
				t.Fatalf("%d columns", len(row))
			}
		}
	})
}
