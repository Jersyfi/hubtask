// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// refusing refuses one permission and answers every other the way its inner authoriser does.
type refusing struct {
	*authorizer
	permission service.Permission
}

func (r refusing) Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error {
	if request.Permission == r.permission {
		r.requests = append(r.requests, request)
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return r.authorizer.Authorize(ctx, actor, request)
}

// UC-ID-27 check 2: changing whether a field is restricted - for writing or for reading - takes the
// field's own permission at the workspace; without it nothing is written.
func TestChangingAFieldsRestrictionTakesItsPermissionAtTheWorkspace(t *testing.T) {
	restricted := true
	for name, attributes := range map[string]domain.CustomFieldAttributes{
		"for writing": {Restricted: &restricted},
		"for reading": {ReadRestricted: &restricted},
	} {
		t.Run(name, func(t *testing.T) {
			h := newCustomFieldHarness(t)
			defined := h.defined(t, DefineCustomFieldCommand{
				CollectionID: collectionID, Key: "salary", Kind: domain.CustomFieldText,
			})
			h.authorizer.requests = nil

			if _, err := (UpdateCustomField{Writer: h.writer()}).Execute(t.Context(), actor(), UpdateCustomFieldCommand{
				FieldID: defined.ID, Attributes: attributes,
			}); err != nil {
				t.Fatalf("a holder of the permission was refused: %v", err)
			}
			asked := false
			for _, request := range h.authorizer.requests {
				if request.Permission == service.FieldWrite("salary") &&
					len(request.Path) == 1 && request.Path[0] == identity.TenantScope() {
					asked = true
				}
			}
			if !asked {
				t.Fatalf("the change did not ask for the field's permission at the workspace: %+v", h.authorizer.requests)
			}

			refused := newCustomFieldHarness(t)
			again := refused.defined(t, DefineCustomFieldCommand{
				CollectionID: collectionID, Key: "salary", Kind: domain.CustomFieldText,
			})
			writer := refused.writer()
			writer.Authorizer = refusing{authorizer: refused.authorizer, permission: service.FieldWrite("salary")}
			_, err := (UpdateCustomField{Writer: writer}).Execute(t.Context(), actor(), UpdateCustomFieldCommand{
				FieldID: again.ID, Attributes: attributes,
			})
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("without the field's permission the change answered %v", err)
			}
			if len(refused.fields.updates) != 0 {
				t.Errorf("a refused change was written: %+v", refused.fields.updates)
			}
		})
	}
}
