// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestParseKindReadsTheContractsEnumAndRefusesTheRest(t *testing.T) {
	for _, value := range []string{"CSV", "TRELLO", "GOOGLE_TASKS", "MICROSOFT_TODO"} {
		kind, err := importer.ParseKind(value)
		if err != nil || string(kind) != value {
			t.Errorf("%s: %v %v", value, kind, err)
		}
	}
	for _, value := range []string{"", "csv", "JIRA"} {
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
