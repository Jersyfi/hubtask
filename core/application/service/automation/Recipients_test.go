// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	knownGroup   = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	unknownGroup = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
)

type roleKeys map[string]bool

func (r roleKeys) Find(_ context.Context, key string) (identityrepo.CustomRoleRow, error) {
	if !r[key] {
		return identityrepo.CustomRoleRow{}, shared.ErrNotFound
	}
	return identityrepo.CustomRoleRow{}, nil
}

func notifyingHarness() *harness {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)
	for kind, field := range map[string]string{
		"NOTIFY_ACCOUNT": "account_id", "NOTIFY_GROUP": "group_id", "NOTIFY_ROLE": "role",
	} {
		h.writer.Catalogue.(catalogue).known[kind] = usecase.Descriptor{
			Name: kind, TokenScope: automationScope,
			Input: []usecase.Field{{Name: "item_id"}, {Name: field}},
		}
	}
	h.writer.References = &references{present: map[repository.ReferenceKind]map[shared.ID]bool{
		repository.ReferenceGroup:   {knownGroup: true},
		repository.ReferenceAccount: {personID: true},
	}}
	h.writer.CustomRoles = roleKeys{"dispatcher": true}
	return h
}

// UC-AUT-09 check 4: a step naming an account, a group or a role the workspace does not have is
// refused at the save, at the step's own field - inside a branch as well.
func TestANotificationStepNamingAnUnknownRecipientIsRefusedAtTheSave(t *testing.T) {
	for name, action := range map[string]domain.Action{
		"account":   {Kind: "NOTIFY_ACCOUNT", Params: map[string]any{"account_id": "01936f2a-7c1e-7000-8000-0000000000f9"}},
		"group":     {Kind: "NOTIFY_GROUP", Params: map[string]any{"group_id": unknownGroup.String()}},
		"role":      {Kind: "NOTIFY_ROLE", Params: map[string]any{"role": "nobody_has_this"}},
		"malformed": {Kind: "NOTIFY_GROUP", Params: map[string]any{"group_id": "not-an-id"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := notifyingHarness()
			command := validCommand()
			command.Actions = []domain.Action{action}
			_, err := CreateRule{Writer: h.writer}.Execute(context.Background(), writerActor(), command)
			if err == nil {
				t.Fatal("the rule was saved")
			}
			fields := fieldCodes(t, err)
			found := false
			for path, code := range fields {
				if code == "notifications.recipient_unknown" && len(path) > len("/actions/0/params/") {
					found = true
				}
			}
			if !found {
				t.Errorf("refused with %v, want notifications.recipient_unknown at the step's field", fields)
			}
		})
	}

	h := notifyingHarness()
	command := validCommand()
	command.Actions = []domain.Action{{Kind: domain.ActionBranch, Params: map[string]any{
		"condition": "true",
		"then":      []any{map[string]any{"kind": "NOTIFY_GROUP", "params": map[string]any{"group_id": unknownGroup.String()}}},
	}}}
	if _, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), command); err == nil {
		t.Error("a branch's unknown recipient was saved")
	}
}

func TestANotificationStepNamingKnownRecipientsIsSaved(t *testing.T) {
	h := notifyingHarness()
	command := validCommand()
	command.Actions = []domain.Action{
		{Kind: "NOTIFY_ACCOUNT", Params: map[string]any{"account_id": personID.String()}},
		{Kind: "NOTIFY_GROUP", Params: map[string]any{"group_id": knownGroup.String()}},
		{Kind: "NOTIFY_ROLE", Params: map[string]any{"role": "ADMIN"}},
		{Kind: "NOTIFY_ROLE", Params: map[string]any{"role": "dispatcher"}},
	}
	if _, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), command); err != nil {
		t.Fatalf("refused: %v", err)
	}
}

// UC-AUT-09 check 4, later: the check flags a step whose group was removed after the save.
func TestTheCheckFlagsANotificationStepWhoseGroupIsGone(t *testing.T) {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Actions = []domain.Action{{Kind: "NOTIFY_GROUP", Params: map[string]any{"group_id": unknownGroup.String()}}}
	h := newCheck(rule)
	known := h.check.Catalogue.(catalogue)
	known.known["NOTIFY_GROUP"] = usecase.Descriptor{
		Name: "NotifyGroup", TokenScope: automationScope,
		Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID}, {Name: "group_id", Kind: usecase.KindID}},
	}
	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil || len(checked) != 1 {
		t.Fatalf("checking: %d, %v", len(checked), err)
	}
	if codes := findingCodes(checked[0]); codes["/actions/0/params/group_id"] != FindingReferenceGone {
		t.Errorf("findings %v, want the gone group flagged", codes)
	}
}
