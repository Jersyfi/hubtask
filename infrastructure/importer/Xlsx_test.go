// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// sheetOf writes a table as a workbook of inline strings, as a spreadsheet saves one.
func sheetOf(t *testing.T, table string) []byte {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(table)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, record := range records {
		sheet.WriteString(`<row r="` + strconv.Itoa(i+1) + `">`)
		for j, value := range record {
			if value == "" {
				continue
			}
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(value))
			sheet.WriteString(`<c r="` + string(rune('A'+j)) + strconv.Itoa(i+1) + `" t="inlineStr"><is><t>` + escaped.String() + `</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	for name, body := range map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Tasks" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
	} {
		f, _ := archive.Create(name)
		_, _ = f.Write([]byte(body))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// UC-IMP-01: a workbook's first sheet lands exactly as the same table saved as CSV - the same
// collection, entries, tree, buckets, labels and refusals by row number - so the mapping and the
// form check through IMPORT are the CSV importer's own.
func TestAWorkbookLandsAsItsCsvWould(t *testing.T) {
	table := sample + ",no title on this row,,,,,\n"
	fromCSV, err := CSV{}.Convert(context.Background(), source(t, table, map[string]string{"notes": "notes"}))
	if err != nil {
		t.Fatal(err)
	}
	workbook := source(t, "", map[string]string{"notes": "notes"})
	workbook.Content = bytes.NewReader(sheetOf(t, table))
	fromXLSX, err := XLSX{}.Convert(context.Background(), workbook)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromCSV, fromXLSX) {
		t.Errorf("the workbook landed differently from its CSV:\n%+v\n%+v", fromXLSX, fromCSV)
	}
	if len(fromXLSX.Refused) != 1 || fromXLSX.Refused[0].Row != 5 {
		t.Errorf("refusals %+v, want row 5", fromXLSX.Refused)
	}
}

func TestAFileThatIsNoWorkbookIsRefusedAsNotTheKind(t *testing.T) {
	plain := source(t, sample, nil)
	_, err := XLSX{}.Convert(context.Background(), plain)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != domain.CodeFileNotKind {
		t.Errorf("a CSV sent as a workbook answered %v", err)
	}
}
