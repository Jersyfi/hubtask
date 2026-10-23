// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FirstColumn answers whether a column is the first of its board: the leftmost one that is not
// deleted. A board is answered left to right (repository.Buckets.List), so it is the first of the
// list. An empty board has no first column, and no column is past one.
func FirstColumn(board []Bucket, bucketID shared.ID) bool {
	for _, bucket := range board {
		if !bucket.IsDeleted() {
			return bucket.ID == bucketID
		}
	}
	return true
}

// MissingRequiredFields answers the keys of the required custom fields an entry carries no value
// for, sorted (UC-WRK-31 check 2). Only the definitions in force for its collection count -
// deleted ones and those for other types do not - and a collection's own definition wins over a
// workspace-wide one under the same key, as everywhere a key is resolved (domain-model.md §3.5).
//
// A value is the entry's visible one: a read answers only values whose definition still lives, so
// a key recreated after deletion starts empty and counts as missing.
func MissingRequiredFields(definitions []CustomFieldDefinition, item WorkItem) []string {
	inForce := map[string]CustomFieldDefinition{}
	for _, definition := range definitions {
		if definition.DeletedAt != nil {
			continue
		}
		if held, seen := inForce[definition.Key]; seen && !held.CollectionID.IsZero() {
			continue
		}
		inForce[definition.Key] = definition
	}
	var missing []string
	for key, definition := range inForce {
		if !definition.IsRequired || !slices.Contains(definition.AppliesTo, item.Type) {
			continue
		}
		if !hasValue(item.CustomFields[key]) {
			missing = append(missing, key)
		}
	}
	slices.Sort(missing)
	return missing
}

// hasValue says whether a stored custom value is one: an empty text or an empty list is not.
func hasValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	}
	return true
}

// RequiredFieldsMissing is the refusal of an entry entering a column past the first without its
// required fields: a validation failure naming every missing key, once in the parameters and
// once per field, so a form can mark each one.
func RequiredFieldsMissing(keys []string) error {
	fields := make([]shared.FieldError, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, shared.FieldError{
			Path: "/custom_fields/" + key, Code: "fields.value_required", Params: map[string]string{"key": key},
		})
	}
	return shared.ErrValidation.
		WithDetail("items.required_fields_missing").
		WithParams(map[string]string{"keys": strings.Join(keys, ", "), "count": strconv.Itoa(len(keys))}).
		WithFields(fields...)
}
