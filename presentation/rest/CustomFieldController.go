// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The vocabulary a workspace adds to its entries, answered by the generated mapping
// (Doors.gen.go); customFieldResponse draws a definition wherever one is answered.

// customFieldResponse maps the catalogue's projection onto the contract's schema.
func customFieldResponse(out usecase.Output) openapi.CustomFieldDefinition {
	createdAt := timeValue(out["created_at"])
	updatedAt := timeValue(out["updated_at"])

	definition := openapi.CustomFieldDefinition{
		Id:             uuidValue(out.String("id")),
		Key:            out.String("key"),
		Kind:           openapi.CustomFieldKind(out.String("kind")),
		Options:        stringsOf(out["options"]),
		IsRequired:     boolOf(out["is_required"]),
		Restricted:     optionalBool(boolOf(out["restricted"])),
		ReadRestricted: optionalBool(boolOf(out["read_restricted"])),
		AppliesTo:      itemTypesOf(out["applies_to"]),
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
		Version:        out.Int("version"),
	}
	// Always present, as null for a workspace-wide field: absent would say this server does not
	// know about scopes, which is a different statement from "this one is everywhere".
	if scope := out.String("collection_id"); scope != "" {
		collectionID := uuidValue(scope)
		definition.CollectionId = &collectionID
	}
	if currency := out.String("currency"); currency != "" {
		definition.Currency = &currency
	}
	if formula := out.String("formula"); formula != "" {
		definition.Formula = &formula
	}
	return definition
}

// stringsOf reads a projection's list of strings, as an empty array rather than null: a client
// renders the option editor from it, and null would make it special-case the kinds with no choices.
func stringsOf(value any) []string {
	values, _ := value.([]string)
	if values == nil {
		return []string{}
	}
	return values
}

func boolOf(value any) bool {
	flag, _ := value.(bool)
	return flag
}

func itemTypesOf(value any) []openapi.ItemType {
	names, _ := value.([]string)

	types := make([]openapi.ItemType, 0, len(names))
	for _, name := range names {
		types = append(types, openapi.ItemType(name))
	}
	return types
}

// optionalBool answers a flag the contract leaves optional.
func optionalBool(value bool) *bool { return &value }
