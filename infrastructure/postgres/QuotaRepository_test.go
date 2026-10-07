// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"reflect"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
)

// The settings document and the port's overrides have to carry the same rows, and every row has to
// travel in both directions.
//
// This exists because the omission is invisible: a field added to the document's struct and to
// neither mapping parses and serialises and is dropped on the way through - an override an
// operator wrote, read back as unset, with nothing failing. The unit tests cannot see it (they hold
// no adapter).
//
// Reflection over the two structs rather than a list somebody keeps: a list is the thing that was
// already wrong.
func TestEveryQuotaOverrideSurvivesTheDocument(t *testing.T) {
	document := reflect.TypeOf(quotasDocument{})
	port := reflect.TypeOf(repository.Overrides{})

	if document.NumField() != port.NumField() {
		t.Fatalf("the document carries %d rows and the port %d", document.NumField(), port.NumField())
	}

	// One override set at a time, so a mapping that dropped exactly one field is caught rather than
	// hidden by the others.
	for i := range port.NumField() {
		name := port.Field(i).Name
		if _, held := document.FieldByName(name); !held {
			t.Errorf("the settings document has no %s", name)
			continue
		}

		value := int64(i + 1)
		overrides := repository.Overrides{}
		reflect.ValueOf(&overrides).Elem().FieldByName(name).Set(reflect.ValueOf(&value))

		there := toDocument(overrides)
		back := fromDocument(there)

		got := reflect.ValueOf(back).FieldByName(name)
		if got.IsNil() {
			t.Errorf("%s does not survive the round trip - an operator writes it and reads it "+
				"back as unset", name)
			continue
		}
		if got.Elem().Int() != value {
			t.Errorf("%s came back as %d, want %d", name, got.Elem().Int(), value)
		}
	}
}
