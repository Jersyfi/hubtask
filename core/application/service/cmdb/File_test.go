// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"errors"
	"reflect"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

// An inventory names a monitor 27" - a quote inside an unquoted cell, which a strict reader
// refuses together with the whole file.
func TestAStrayQuoteInACellDoesNotRefuseTheFile(t *testing.T) {
	mapping := &domain.FileMapping{Class: "server", NameColumn: "Name", Columns: map[string]string{"Model": "model"}}
	p, _, err := ParseFile(domain.FormatCSV, []byte("Name,Model\nmon-01,Dell 27\" P2723\n"), mapping)
	if err != nil || len(p.Items) != 1 || p.Items[0].Attributes["model"] != "Dell 27\" P2723" {
		t.Fatalf("read %+v %v", p.Items, err)
	}
}

// A bill of materials in either format becomes software CIs keyed by package URL, carrying the
// CPE and version the class identifies by next, and its dependencies become depends_on relations
// (UC-AST-04 check 2).
func TestABillOfMaterialsBecomesSoftwareAndItsDependencies(t *testing.T) {
	cyclone := `{"bomFormat":"CycloneDX","components":[
		{"bom-ref":"a","name":"shop","version":"1.0","purl":"pkg:npm/shop@1.0"},
		{"bom-ref":"b","name":"left-pad","version":"1.3.0","cpe":"cpe:2.3:a:x:left-pad:1.3.0"}],
		"dependencies":[{"ref":"a","dependsOn":["b"]}]}`
	spdxDoc := `{"spdxVersion":"SPDX-2.3","packages":[
		{"SPDXID":"SPDXRef-a","name":"shop","versionInfo":"1.0","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:npm/shop@1.0"}]},
		{"SPDXID":"SPDXRef-b","name":"left-pad","versionInfo":"1.3.0","externalRefs":[{"referenceType":"cpe23Type","referenceLocator":"cpe:2.3:a:x:left-pad:1.3.0"}]}],
		"relationships":[{"spdxElementId":"SPDXRef-a","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-b"}]}`
	for format, content := range map[domain.FileFormat]string{domain.FormatCycloneDX: cyclone, domain.FormatSPDX: spdxDoc} {
		p, skipped, err := ParseFile(format, []byte(content), nil)
		if err != nil || len(skipped) != 0 || len(p.Items) != 2 {
			t.Fatalf("%s: read %+v, skipped %v, %v", format, p.Items, skipped, err)
		}
		shop, pad := p.Items[0], p.Items[1]
		if shop.Class != "software" || shop.NativeKey != "pkg:npm/shop@1.0" || shop.Attributes["version"] != "1.0" {
			t.Errorf("%s: the package with a URL reads %+v", format, shop)
		}
		if pad.Class != "software" || pad.Attributes["cpe"] != "cpe:2.3:a:x:left-pad:1.3.0" || pad.Attributes["version"] != "1.3.0" {
			t.Errorf("%s: the package with a CPE reads %+v", format, pad)
		}
		if len(p.Relations) != 1 || p.Relations[0].Type != domain.RelDependsOn ||
			p.Relations[0].From != shop.Ref || p.Relations[0].To != pad.Ref {
			t.Errorf("%s: the relations are %+v", format, p.Relations)
		}
	}
}

// A file above 5 MiB is refused before it is read, and one that is not its format is unreadable
// (UC-AST-04 check 4).
func TestAFileTooLargeOrUnreadableIsRefusedByItsCode(t *testing.T) {
	mapping := &domain.FileMapping{Class: "server", NameColumn: "Name"}
	if domain.MaxFileBytes != 5<<20 {
		t.Errorf("the bound is %d bytes, want 5 MiB", domain.MaxFileBytes)
	}
	_, _, err := ParseFile(domain.FormatCSV, make([]byte, domain.MaxFileBytes+1), mapping)
	if code := shared.AsError(err).DetailCode; code != "cmdb.file_too_large" {
		t.Errorf("a file over the bound answered %v", err)
	}
	for format, content := range map[domain.FileFormat]string{
		domain.FormatJSON: "not json", domain.FormatCycloneDX: `{"bomFormat":"other"}`, domain.FormatSPDX: "{",
	} {
		var m *domain.FileMapping
		if format == domain.FormatJSON {
			m = mapping
		}
		_, _, err := ParseFile(format, []byte(content), m)
		if code := shared.AsError(err).DetailCode; !errors.Is(err, shared.ErrValidation) || code != "cmdb.file_unreadable" {
			t.Errorf("%s: an unreadable file answered %v", format, err)
		}
	}
}
