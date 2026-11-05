// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// recipientUnknownCode is the notification steps' own refusal, given at the save as at the run.
const recipientUnknownCode = "notifications.recipient_unknown"

// RoleKeys answers a custom role by its key. Hold keeps it, inside the writing transaction, from a
// deletion that counts the rules naming it (DeleteRole); a role already gone is ErrNotFound.
type RoleKeys interface {
	Find(ctx context.Context, key string) (identityrepo.CustomRoleRow, error)
	Hold(ctx context.Context, key string) error
}

// recipientFields is which parameter of which notification step names its recipient, and what
// kind of thing it names. A role is a key, not an identifier, and is resolved apart.
var recipientFields = map[string]struct {
	param string
	kind  repository.ReferenceKind
}{
	"NOTIFY_ACCOUNT": {param: "account_id", kind: repository.ReferenceAccount},
	"NOTIFY_GROUP":   {param: "group_id", kind: repository.ReferenceGroup},
	"NOTIFY_ROLE":    {param: "role"},
}

// checkRecipients refuses a notification step that names an account, a group or a role the
// workspace does not have (UC-AUT-09 check 4) - on both arms of every branch, as checkActions
// reads them. Its removal later is the check's finding (Check.go).
func (w Writer) checkRecipients(ctx context.Context, actor appshared.ActorContext, actions []domain.Action) error {
	var findings []shared.FieldError
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		findings, err = w.unknownRecipients(ctx, actions, "/actions", 0)
		return err
	})
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		return shared.ErrValidation.WithDetail("automation.actions_invalid").WithFields(findings...)
	}
	return nil
}

// holdRoles holds every custom role the rule's NOTIFY_ROLE steps name, in the write's own
// transaction: the check above ran in a transaction of its own, and a role deleted in between would
// otherwise be named by a switched-on rule its deletion did not count.
func (w Writer) holdRoles(ctx context.Context, actions []domain.Action, at string, depth int) error {
	var gone []shared.FieldError
	for i, action := range actions {
		path := at + "/" + itoa(i)
		if action.Kind == domain.ActionBranch {
			branch, err := domain.ReadBranch(action.Params, path, depth)
			if err != nil {
				return err
			}
			for name, arm := range map[string][]domain.Action{"then": branch.Then, "else": branch.Else} {
				if err := w.holdRoles(ctx, arm, path+"/params/"+name, depth+1); err != nil {
					return err
				}
			}
			continue
		}
		key, named := action.Params["role"].(string)
		if action.Kind != "NOTIFY_ROLE" || !named || identity.Role(key).Valid() || w.CustomRoles == nil {
			continue
		}
		err := w.CustomRoles.Hold(ctx, key)
		if errors.Is(err, shared.ErrNotFound) {
			gone = append(gone, shared.FieldError{Path: path + "/params/role", Code: recipientUnknownCode})
			continue
		}
		if err != nil {
			return err
		}
	}
	if len(gone) > 0 {
		return shared.ErrValidation.WithDetail("automation.actions_invalid").WithFields(gone...)
	}
	return nil
}

func (w Writer) unknownRecipients(
	ctx context.Context, actions []domain.Action, at string, depth int,
) ([]shared.FieldError, error) {
	var findings []shared.FieldError
	for i, action := range actions {
		path := at + "/" + itoa(i)
		if action.Kind == domain.ActionBranch {
			branch, err := domain.ReadBranch(action.Params, path, depth)
			if err != nil {
				return nil, err
			}
			for name, arm := range map[string][]domain.Action{"then": branch.Then, "else": branch.Else} {
				nested, err := w.unknownRecipients(ctx, arm, path+"/params/"+name, depth+1)
				if err != nil {
					return nil, err
				}
				findings = append(findings, nested...)
			}
			continue
		}
		field, notifies := recipientFields[action.Kind]
		if !notifies {
			continue
		}
		value, present := action.Params[field.param].(string)
		if !present {
			// Absent is the run's refusal, as for every required parameter (checkActions).
			continue
		}
		known, err := w.recipientKnown(ctx, field.kind, value)
		if err != nil {
			return nil, err
		}
		if !known {
			findings = append(findings, shared.FieldError{
				Path: path + "/params/" + field.param, Code: recipientUnknownCode,
			})
		}
	}
	return findings, nil
}

func (w Writer) recipientKnown(ctx context.Context, kind repository.ReferenceKind, value string) (bool, error) {
	if kind == "" {
		if identity.Role(value).Valid() {
			return true, nil
		}
		if w.CustomRoles == nil {
			return false, shared.ErrInternal.WithDetail("automation.recipients_unavailable")
		}
		_, err := w.CustomRoles.Find(ctx, value)
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		return err == nil, err
	}
	id, err := shared.ParseID(value)
	if err != nil {
		return false, nil
	}
	if w.References == nil {
		// Fail closed: a build that cannot resolve a recipient cannot promise the step reaches one.
		return false, shared.ErrInternal.WithDetail("automation.recipients_unavailable")
	}
	return w.References.Exists(ctx, kind, id)
}
