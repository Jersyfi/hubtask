// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestParseKindReadsTheContractsEnumAndRefusesTheRest(t *testing.T) {
	for _, value := range []string{"CSV", "TRELLO", "GOOGLE_TASKS", "MICROSOFT_TODO", "JIRA", "CONFLUENCE", "MARKDOWN", "HTML", "DOCX"} {
		kind, err := importer.ParseKind(value)
		if err != nil || string(kind) != value {
			t.Errorf("%s: %v %v", value, kind, err)
		}
	}
	for _, value := range []string{"", "csv", "jira", "SPREADSHEET"} {
		_, err := importer.ParseKind(value)
		var typed *shared.Error
		if !errors.As(err, &typed) || typed.DetailCode != importer.CodeKindUnknown {
			t.Errorf("%q should be refused by name, got %v", value, err)
		}
		if len(typed.Fields) != 1 || typed.Fields[0].Path != "/kind" {
			t.Errorf("%q: the refusal names the field: %+v", value, typed.Fields)
		}
	}
}

func TestAWikiAndMarkdownLandPages(t *testing.T) {
	if !importer.KindWiki.LandsPages() || !importer.KindMarkdown.LandsPages() || !importer.KindHTML.LandsPages() ||
		!importer.KindDOCX.LandsPages() || importer.KindIssueTracker.LandsPages() {
		t.Error("the kinds that land pages are not the wiki's and Markdown's")
	}
}

func TestAMappingIsRefusedWhereTheRunCouldNotKeepIt(t *testing.T) {
	tooMany := map[string]string{}
	for i := 0; i <= importer.MaxMappingEntries; i++ {
		tooMany[string(rune('a'+i%26))+string(rune('A'+i/26))] = "column"
	}
	long := strings.Repeat("x", 201)
	cases := []struct {
		name    string
		mapping map[string]string
		refused bool
	}{
		{"none", nil, false},
		{"a handful", map[string]string{"title": "Name", "due_at": "Due"}, false},
		{"exactly 200 characters", map[string]string{strings.Repeat("é", 200): strings.Repeat("ü", 200)}, false},
		{"more entries than any table has fields", tooMany, true},
		{"a zero byte in a field", map[string]string{"ti\x00tle": "Name"}, true},
		{"a zero byte in a column", map[string]string{"title": "Na\x00me"}, true},
		{"a field past 200 characters", map[string]string{long: "Name"}, true},
		{"a column past 200 characters", map[string]string{"title": long}, true},
	}
	for _, c := range cases {
		err := importer.CheckMapping(c.mapping)
		if !c.refused {
			if err != nil {
				t.Errorf("%s: refused %v", c.name, err)
			}
			continue
		}
		var typed *shared.Error
		if !errors.As(err, &typed) || typed.DetailCode != importer.CodeMappingUnknown ||
			len(typed.Fields) != 1 || typed.Fields[0].Path != "/mapping" {
			t.Errorf("%s: want a refusal of /mapping, got %v", c.name, err)
		}
		if field := typed.Params["field"]; strings.ContainsRune(field, 0) {
			t.Errorf("%s: the refusal carries a zero byte: %q", c.name, field)
		}
	}
}
