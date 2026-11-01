// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

type entryExplainer struct {
	path []domain.Scope
	on   access.ItemSubject
}

func (e *entryExplainer) ExplainEntry(
	_ context.Context, _ appshared.ActorContext, _ shared.ID, path []domain.Scope, on access.ItemSubject,
) (access.EntryExplained, error) {
	e.path, e.on = path, on
	return access.EntryExplained{Held: access.Held{Roles: []string{"CONTRIBUTOR"}}, Does: on.Does, Reason: access.NotAssignee}, nil
}

func explainingEntries(roles Roles, explainer *entryExplainer) ExplainAccess {
	return ExplainAccess{
		Roles: roles, Entries: explainer,
		Containers: containerFinder{byID: map[shared.ID]work.Container{
			hubID:        {ID: hubID, Type: work.ContainerHub},
			collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID},
		}},
		Items: itemFinder{byID: map[shared.ID]work.WorkItem{itemID: {ID: itemID, CollectionID: collectionID, AssigneeID: otherHubID}}},
	}
}

// "I can see the ticket but cannot change it": the administrator asks about the entry and is told
// the decision a request meets there, with the assignee read from the stored entry.
func TestExplainAccessAnswersTheDecisionAtAnEntry(t *testing.T) {
	explainer := &entryExplainer{}
	explain := explainingEntries(rolesWith(newRoleStore(), &authorizer{}, &auditSink{}, &heldBy{}), explainer)

	out, err := explain.Descriptor().Handler.Invoke(t.Context(), admin(), usecase.Input{
		"account_id": invitedID.String(), "scope_type": "ITEM", "scope_id": itemID.String(), "action": "CHANGE",
	})
	if err != nil {
		t.Fatalf("explaining an entry: %v", err)
	}
	entry, _ := out["entry"].(map[string]any)
	if entry["action"] != "CHANGE" || entry["permitted"] != false || entry["reason"] != "NOT_ASSIGNEE" {
		t.Fatalf("the entry's answer is %v", out)
	}
	want := []domain.Scope{domain.TenantScope(), domain.HubScope(hubID), domain.CollectionScope(collectionID)}
	if len(explainer.path) != len(want) || explainer.path[2] != want[2] {
		t.Errorf("asked along %v, want %v", explainer.path, want)
	}
	if explainer.on.Assignee != otherHubID || explainer.on.ID != itemID || explainer.on.Does != service.ItemChange {
		t.Errorf("asked about %+v, want the stored entry's assignee", explainer.on)
	}
}

// An entry the caller does not reach is not found, about oneself too: the explanation discloses no
// entry a guessed identifier names (T-04).
func TestExplainAccessDoesNotDiscloseAnEntryTheCallerCannotReach(t *testing.T) {
	roles := rolesWith(newRoleStore(), &authorizer{}, &auditSink{}, &heldBy{})
	roles.Reach = reach{hidden: map[shared.ID]bool{itemID: true}}
	explainer := &entryExplainer{}
	_, err := explainingEntries(roles, explainer).Execute(t.Context(), admin(), shared.ID(""), domain.ItemScope(itemID), "", "")
	if !errors.Is(err, shared.ErrNotFound) || explainer.on.ID != "" {
		t.Fatalf("an unreachable entry answered %v and was explained: %+v", err, explainer.on)
	}
}
