// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The members an entry carries. A sub-resource with PUT and DELETE rather than a field on
// the entry, for the reason the labels are one: a set is not a field. Two devices adding two
// different people at once is the case the OR-set exists to serve, and a merge patch carrying the
// whole array would let the later of the two erase the other's (offline-sync.md §4.2).
//
// The assignee is an action route instead, because it is a scalar. The split between these two
// controllers is the split between the two merge rules.

// itemMembersResponse maps the catalogue's answer onto the contract's ItemMembers. The array is
// empty rather than null when the entry carries nobody: a client that iterates it should not have
// to nil-check the field.
func itemMembersResponse(out usecase.Output) openapi.ItemMembers {
	set := openapi.ItemMembers{
		ItemId:    uuidValue(out.String("item_id")),
		MemberIds: []openapi_types.UUID{},
	}
	ids, _ := out["member_ids"].([]string)
	for _, id := range ids {
		set.MemberIds = append(set.MemberIds, uuidValue(id))
	}
	return set
}
