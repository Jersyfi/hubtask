// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	UpdateTemplateName = "UpdateTemplate"
	DeleteTemplateName = "DeleteTemplate"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	TemplateUpdatedAction audit.Action = "template.updated"
	TemplateDeletedAction audit.Action = "template.deleted"
)

// UpdateTemplate changes a template's name, description or tree.
type UpdateTemplate struct {
	Writer TemplateWriter
}

// DeleteTemplate removes a template, leaving the trees it has stamped out standing.
type DeleteTemplate struct {
	Writer TemplateWriter
}

// ChangeTemplateCommand is the input of both, typed. The patch is the update's; a deletion carries
// none, which is the one field the two do not share.
type ChangeTemplateCommand struct {
	TemplateID shared.ID
	Patch      domain.TemplatePatch
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute changes the template and returns it.
func (h UpdateTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeTemplateCommand,
) (domain.Template, error) {
	if cmd.Patch.IsEmpty() {
		return domain.Template{}, shared.ErrValidation.
			WithDetail("templates.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "templates.update_empty"})
	}
	return h.Writer.change(ctx, actor, cmd, changingTemplate)
}

// Execute removes the template.
func (h DeleteTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeTemplateCommand,
) error {
	_, err := h.Writer.change(ctx, actor, cmd, removingTemplate)
	return err
}

// templateChange is which change the caller asked for. Not a boolean, for the reason the comment's
// is not: this is the parameter that decides which of two audit trails is written.
type templateChange bool

const (
	changingTemplate templateChange = true
	removingTemplate templateChange = false
)

func (c templateChange) action() audit.Action {
	if c == changingTemplate {
		return TemplateUpdatedAction
	}
	return TemplateDeletedAction
}

// change is the whole of both use cases.
func (w TemplateWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeTemplateCommand,
	want templateChange,
) (domain.Template, error) {
	if cmd.TemplateID.IsZero() {
		return domain.Template{}, templateIDRequired()
	}

	// The template and its scope are read before the permission question, because the answer
	// depends on where it is defined. Nothing read here is trusted afterwards - the state that
	// decides the write is read again inside the transaction.
	var current domain.Template
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		current, err = w.findTemplate(ctx, cmd.TemplateID)
		return err
	})
	if err != nil {
		return domain.Template{}, err
	}

	path, err := w.scopePath(ctx, actor, current.Scope, current.ScopeID)
	if err != nil {
		return domain.Template{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       path,
		Action:     want.action(),
		TokenScope: templatesWrite,
		TargetType: templateTarget,
		TargetID:   current.ID,
	}); err != nil {
		return domain.Template{}, err
	}

	var changed domain.Template
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		stored, err := w.findTemplate(ctx, cmd.TemplateID)
		if err != nil {
			return err
		}

		if want == removingTemplate {
			removed := stored.Removed(now)
			expected := cmd.ExpectedVersion
			if expected == 0 {
				expected = stored.Version
			}
			if err := w.Templates.SetDeleted(ctx, removed, expected); err != nil {
				return err
			}
			removed.Version = expected + 1

			// A deletion carries no payload: there is nothing left to describe, and the trees it
			// stamped out are ordinary entries that outlive it.
			if err := w.Changes.Record(ctx, changelog.Change{
				TenantID:    removed.TenantID,
				Entity:      templateTarget,
				EntityID:    removed.ID,
				Op:          changelog.Delete,
				ContainerID: removed.ScopeID,
				ActorID:     actor.AccountID,
				HLC:         w.HLC.Next(),
			}); err != nil {
				return err
			}
			changed = removed
			return w.recordAudit(
				ctx, actor, removed, TemplateDeletedAction, wholeTemplateAudit(removed), now)
		}

		wanted, changes, err := stored.Changed(cmd.Patch, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// Already what was asked for. Nothing is written, no version is spent and nothing is
			// recorded - and the If-Match is still honoured, because the state the caller was
			// reasoning about is not the state that is there.
			if err := ensureTemplateVersion(stored, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = stored
			return nil
		}
		if cmd.Patch.Root != nil {
			if err := w.ensureTreeIsPossible(ctx, wanted); err != nil {
				return err
			}
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			expected = stored.Version
		}
		if err := w.Templates.Update(ctx, wanted, expected); err != nil {
			return err
		}
		wanted.Version = expected + 1

		if err := w.recordWhole(ctx, actor, wanted); err != nil {
			return err
		}
		if err := w.recordAudit(
			ctx, actor, wanted, TemplateUpdatedAction, templateFieldAudit(changes), now,
		); err != nil {
			return err
		}
		changed = wanted
		return nil
	})
	if err != nil {
		return domain.Template{}, err
	}
	return changed, nil
}

// templateFieldAudit records what moved. The names are content and are classified as such; the
// tree is recorded as the fact that it changed rather than as its content (rule 10).
func templateFieldAudit(changes []domain.FieldChange) []audit.Change {
	recorded := make([]audit.Change, 0, len(changes))
	for _, change := range changes {
		classification := audit.Open
		if change.Field == domain.FieldTemplateName ||
			change.Field == domain.FieldTemplateDescription {
			classification = audit.Sensitive
		}
		if change.Field == domain.FieldTemplateNodes {
			recorded = append(recorded, audit.Change{
				Field: change.Field, Classification: audit.Open, To: "changed",
			})
			continue
		}
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: classification,
			From: change.From, To: change.To,
		})
	}
	return recorded
}

// ensureTemplateVersion holds a no-op change to the version the caller read.
func ensureTemplateVersion(template domain.Template, expected int) error {
	if expected == 0 || expected == template.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("templates.version_conflict").
		WithParams(map[string]string{
			"template_id": template.ID.String(), "expected_version": strconv.Itoa(expected),
		})
}

// Descriptor is the catalogue entry.
func (h UpdateTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateTemplateName,
		Summary: "Changes a template's name, description or tree. A merge patch - a member that " +
			"is not sent is not touched - and the tree travels whole, because a tree is a shape " +
			"rather than a list of settings. The scope and the root type are not changeable: a " +
			"template that moved scope would move out from under the people who could use it, " +
			"and one whose root type changed would produce a different kind of thing under the " +
			"same name. Needs STRUCTURE at the template's scope.",
		SideEffects: "Writes the changed fields, records the change for offline clients and " +
			"writes an audit entry.",
		TokenScope: templatesWrite,
		Input: []usecase.Field{
			{
				Name: "template_id", Kind: usecase.KindID, Required: true,
				Description: "The template to change.",
			},
			{Name: "name", Kind: usecase.KindString, Description: "What it is called."},
			{Name: "description", Kind: usecase.KindString, Description: "What it is for."},
			{
				Name: "nodes", Kind: usecase.KindList,
				Description: "The tree, as one root node carrying its children.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplateUpdatedAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("template_id")
	if err != nil {
		return nil, err
	}

	patch := domain.TemplatePatch{
		Name:        in.OptionalString("name"),
		Description: in.OptionalString("description"),
	}
	if in.Present("nodes") {
		root, err := templateTreeOf(in)
		if err != nil {
			return nil, err
		}
		patch.Root = &root
	}

	template, err := h.Execute(ctx, actor, ChangeTemplateCommand{
		TemplateID: id, Patch: patch, ExpectedVersion: in.Int("expected_version"),
	})
	if err != nil {
		return nil, err
	}
	return templateOutput(template), nil
}

// Descriptor is the catalogue entry.
func (h DeleteTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteTemplateName,
		Summary: "Removes a template. A soft delete: the trees it has already stamped out are " +
			"ordinary entries and outlive it, and a template defined afterwards under the same " +
			"name is a new one rather than this one coming back. Needs STRUCTURE at the " +
			"template's scope.",
		SideEffects: "Marks the template deleted, records the deletion for offline clients and " +
			"writes an audit entry.",
		TokenScope: templatesWrite,
		Input: []usecase.Field{
			{
				Name: "template_id", Kind: usecase.KindID, Required: true,
				Description: "The template to remove.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplateDeletedAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("template_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, ChangeTemplateCommand{
		TemplateID: id, ExpectedVersion: in.Int("expected_version"),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
