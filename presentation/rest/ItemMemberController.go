// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The members an entry carries (C-01). A sub-resource with PUT and DELETE rather than a field on
// the entry, for the reason the labels are one: a set is not a field. Two devices adding two
// different people at once is the case the OR-set exists to serve, and a merge patch carrying the
// whole array would let the later of the two erase the other's (offline-sync.md §4.2).
//
// The assignee is an action route instead, because it is a scalar. The split between these two
// controllers is the split between the two merge rules.

const (
	addMemberUseCase    = "AddMember"
	removeMemberUseCase = "RemoveMember"
)

// AddMember answers PUT /items/{itemId}/members/{accountId}.
func (c *RestController) AddMember(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, accountID openapi.AccountId,
) {
	c.itemMember(w, r, addMemberUseCase, itemID, accountID)
}

// RemoveMember answers DELETE /items/{itemId}/members/{accountId}.
func (c *RestController) RemoveMember(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, accountID openapi.AccountId,
) {
	c.itemMember(w, r, removeMemberUseCase, itemID, accountID)
}

// itemMember is what the two share: both name the same pair in the path, take no body, and answer
// with the set the entry now carries.
//
// No ETag. Neither operation touches the entry's own row, so there is no version of the entry to
// write against - and a header saying otherwise would invite a client to send an If-Match that
// nothing here honours (api-guidelines.md §5).
func (c *RestController) itemMember(
	w http.ResponseWriter, r *http.Request, name string,
	itemID openapi.ItemId, accountID openapi.AccountId,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), name, actorOf(r), usecase.Input{
		"item_id":    itemID.String(),
		"account_id": accountID.String(),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, itemMembersResponse(out))
}

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
