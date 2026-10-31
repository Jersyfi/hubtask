// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"reflect"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-AST-04: an asset register kept in Excel is read as its CSV export is - by the same mapping,
// rows without a name reported by number - with its purchase date a date, not a day number.
func TestAWorkbookIsReadAsItsCsvWould(t *testing.T) {
	var file bytes.Buffer
	archive := zip.NewWriter(&file)
	for name, body := range map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Assets" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/styles.xml":              `<styleSheet><cellXfs><xf numFmtId="0"/><xf numFmtId="14"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c><c r="B1" t="inlineStr"><is><t>Serial</t></is></c><c r="C1" t="inlineStr"><is><t>Purchased</t></is></c></row>` +
			`<row r="2"><c r="A2" t="inlineStr"><is><t>srv-01</t></is></c><c r="B2"><v>4711</v></c><c r="C2" s="1"><v>46325</v></c></row>` +
			`<row r="3"><c r="B3"><v>4712</v></c></row>` +
			`</sheetData></worksheet>`,
	} {
		f, _ := archive.Create(name)
		_, _ = f.Write([]byte(body))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	mapping := &domain.FileMapping{Class: "server", NameColumn: "Name", NativeKeyColumn: "Serial", Columns: map[string]string{"Purchased": "purchased_on"}}

	fromXLSX, skipped, err := ParseFile(domain.FormatXLSX, []byte(base64.StdEncoding.EncodeToString(file.Bytes())), mapping)
	if err != nil {
		t.Fatal(err)
	}
	fromCSV, _, err := ParseFile(domain.FormatCSV, []byte("Name,Serial,Purchased\nsrv-01,4711,2026-10-30\n,4712,\n"), mapping)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromXLSX, fromCSV) || !reflect.DeepEqual(skipped, []int{2}) {
		t.Errorf("the workbook read %+v skipping %v, its CSV %+v", fromXLSX, skipped, fromCSV)
	}

	for name, content := range map[string]string{"not base64": "Name,Serial\n", "no workbook": base64.StdEncoding.EncodeToString([]byte("Name,Serial\n"))} {
		_, _, err := ParseFile(domain.FormatXLSX, []byte(content), mapping)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != "cmdb.file_unreadable" {
			t.Errorf("%s answered %v", name, err)
		}
	}
}
