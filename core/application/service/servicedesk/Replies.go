// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListReplyTemplatesName  = "ListReplyTemplates"
	DefineReplyTemplateName = "DefineReplyTemplate"
	DeleteReplyTemplateName = "DeleteReplyTemplate"
	RenderReplyTemplateName = "RenderReplyTemplate"
)

// ReplyTemplateOutput is the shape every door returns (api/openapi.yaml, schema ReplyTemplate).
func ReplyTemplateOutput(t domain.ReplyTemplate) usecase.Output {
	return usecase.Output{"key": t.Key, "name": t.Name, "body": t.Body, "version": t.Version}
}

// ListReplyTemplates reads the workspace's canned replies (UC-SVC-06).
type ListReplyTemplates struct{ Desk Desk }

// Execute answers every template by key, for any member of the workspace.
func (h ListReplyTemplates) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.ReplyTemplate, error) {
	if err := h.Desk.authorizeRead(ctx, actor, RepliesReadAction, replyTarget); err != nil {
		return nil, err
	}
	var out []domain.ReplyTemplate
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Desk.Replies.List(ctx)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListReplyTemplates) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListReplyTemplatesName, Permission: service.PermissionRead,
		Summary:     "Lists the workspace's canned replies: each one's key, name and text with its placeholders.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: RepliesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			templates, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(templates))
			for _, t := range templates {
				data = append(data, ReplyTemplateOutput(t))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, replyTarget)
}

// DefineReplyTemplateCommand is the input, typed.
type DefineReplyTemplateCommand struct {
	Key, Name, Body string
	// ExpectedVersion guards a replacement; 0 replaces whatever stands.
	ExpectedVersion int
}

// DefineReplyTemplate creates a canned reply under a key, or replaces the one there (UC-SVC-06).
type DefineReplyTemplate struct{ Desk Desk }

// Execute defines the template.
func (h DefineReplyTemplate) Execute(ctx context.Context, actor appshared.ActorContext, cmd DefineReplyTemplateCommand) (domain.ReplyTemplate, error) {
	if err := h.Desk.authorizeWrite(ctx, actor, ReplyDefinedAction, replyTarget); err != nil {
		return domain.ReplyTemplate{}, err
	}
	now := h.Desk.Clock.Now()
	var out domain.ReplyTemplate
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Replies.Find(ctx, strings.TrimSpace(cmd.Key))
		switch {
		case errors.Is(err, shared.ErrNotFound):
			if out, err = domain.NewReplyTemplate(domain.NewReplyTemplateInput{
				ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key, Name: cmd.Name,
				Body: cmd.Body, Now: now, Text: h.Desk.Text,
			}); err != nil {
				return err
			}
			if err := h.Desk.Replies.Insert(ctx, out); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
				return shared.ErrVersionConflict.WithDetail("reply_templates.version_conflict")
			}
			if out, err = current.Change(cmd.Name, cmd.Body, now, h.Desk.Text); err != nil {
				return err
			}
			if err := h.Desk.Replies.Update(ctx, out, current.Version); err != nil {
				return err
			}
			out.Version = current.Version + 1
		}
		// The name and the text are a person's words (rule 10).
		return h.Desk.record(ctx, actor, ReplyDefinedAction, replyTarget, out.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: out.Key},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: out.Name},
			audit.Change{Field: "body", Classification: audit.Sensitive, To: out.Body})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h DefineReplyTemplate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineReplyTemplateName,
		Summary: "Creates a canned reply under a key, or replaces the one there. Its text may name the " +
			"placeholders {{entry.title}}, {{entry.id}}, {{requester.name}} and {{agent.name}}; any other is " +
			"refused with reply_templates.variable_unknown.",
		SideEffects: "Writes the template and an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "body", Kind: usecase.KindString, Required: true, Description: "The reply's text, Markdown, up to 20,000 characters."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a replacement of a newer one is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: ReplyDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			defined, err := h.Execute(ctx, actor, DefineReplyTemplateCommand{
				Key: in.String("key"), Name: in.String("name"), Body: in.String("body"),
				ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return ReplyTemplateOutput(defined), nil
		}),
	}, replyTarget)
}

// DeleteReplyTemplate removes a canned reply (UC-SVC-06).
type DeleteReplyTemplate struct{ Desk Desk }

// Execute deletes the template.
func (h DeleteReplyTemplate) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if err := h.Desk.authorizeWrite(ctx, actor, ReplyDeletedAction, replyTarget); err != nil {
		return err
	}
	now := h.Desk.Clock.Now()
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Replies.Find(ctx, key)
		if err != nil {
			return err
		}
		if err := h.Desk.Replies.Delete(ctx, key); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, ReplyDeletedAction, replyTarget, current.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, From: current.Key})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteReplyTemplate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteReplyTemplateName,
		Summary:     "Deletes a canned reply. Replies already sent with it stay as they were written.",
		SideEffects: "Deletes the template and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template's key."},
		},
		Audit: usecase.AuditDeclaration{Action: ReplyDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, replyTarget)
}

// RenderReplyTemplate fills a canned reply for one entry (UC-SVC-06): the handler reads it, edits
// it if need be, and sends it as a comment. Rendering sends nothing.
type RenderReplyTemplate struct {
	Desk       Desk
	Items      workrepo.Items
	Containers workrepo.Containers
	Accounts   identityrepo.Accounts
}

// Execute answers the template's text with the entry's values in its placeholders, for whoever
// may read the entry.
func (h RenderReplyTemplate) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID, key string) (string, error) {
	if err := actor.RequireScope(deskRead); err != nil {
		return "", err
	}
	var (
		template   domain.ReplyTemplate
		values     = map[string]string{"entry.id": itemID.String(), "agent.name": actor.AccountName}
		assignee   shared.ID
		collection []identity.Scope
		requester  shared.ID
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		if found.DeletedAt != nil {
			return appshared.ItemNotFound(itemID)
		}
		values["entry.title"], assignee, requester = found.Title, found.AssigneeID, found.CreatedBy
		container, err := h.Containers.Find(ctx, found.CollectionID)
		if err != nil {
			return err
		}
		collection = service.ContainerScopes(container)
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := h.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: collection, Action: ReplyRenderedAction,
		TokenScope: deskRead, TargetType: replyTarget, TargetID: itemID,
		On: access.ItemSubject{Does: service.ItemRead, ID: itemID, Assignee: assignee},
	}); err != nil {
		return "", err
	}
	err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if template, err = h.Desk.Replies.Find(ctx, strings.TrimSpace(key)); err != nil {
			return err
		}
		if requester.IsZero() || h.Accounts == nil {
			return nil
		}
		account, err := h.Accounts.Find(ctx, requester)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			// An account erased since: the placeholder stays empty rather than naming anybody.
			return nil
		case err != nil:
			return err
		}
		values["requester.name"] = account.DisplayName
		return nil
	})
	if err != nil {
		return "", err
	}
	return template.Render(values), nil
}

// Descriptor is the catalogue entry.
func (h RenderReplyTemplate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RenderReplyTemplateName, Permission: service.PermissionRead,
		Summary: "Fills a canned reply for an entry: the entry's title and identifier, the name of who raised " +
			"it and of who answers, in the template's placeholders. Sends nothing; the text is a comment's " +
			"once the handler adds it.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry the reply is for."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The template's key."},
		},
		Audit: usecase.AuditDeclaration{Action: ReplyRenderedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			body, err := h.Execute(ctx, actor, itemID, in.String("key"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"key": in.String("key"), "body": body}, nil
		}),
	}, replyTarget)
}
