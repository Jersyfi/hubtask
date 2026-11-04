// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"reflect"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
)

// A spreadsheet saved as CSV in a German locale separates by semicolons. The import dialog
// reads its columns so and offers them for the mapping; the server read the same file by
// commas, found no name column, and left every row out.
func TestACsvSeparatedBySemicolonsIsReadAsTheDialogReadsIt(t *testing.T) {
	mapping := &domain.FileMapping{Class: "server", NameColumn: "Name", Columns: map[string]string{"Modell": "model"}}
	p, skipped, err := ParseFile(domain.FormatCSV, []byte("Name;Modell\r\nsrv-01;\"R650, 2U\"\r\n"), mapping)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Item{{Ref: "row-1", Class: "server", Name: "srv-01", Attributes: map[string]any{"model": "R650, 2U"}}}
	if !reflect.DeepEqual(p.Items, want) || len(skipped) != 0 {
		t.Fatalf("read %+v, skipped %v", p.Items, skipped)
	}
	p, _, err = ParseFile(domain.FormatCSV, []byte("Name,Modell\nsrv-01;a,R650\n"), mapping)
	if err != nil || len(p.Items) != 1 || p.Items[0].Name != "srv-01;a" {
		t.Fatalf("a comma table stays one: %+v %v", p.Items, err)
	}
}

