// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// Drafts of a block many people work with (ui.md A4): a type with its form, an SLA policy, a
// role. A draft changes nothing in force, so it may be half finished - only its shape is checked;
// publishing it is an ordinary plan and apply of its block, held to every rule an apply is, and
// that apply removes the draft it put in force. A workflow keeps its own draft (UC-FLW-03).
const (
	ListConfigDraftsName   = "ListConfigDrafts"
	SaveConfigDraftName    = "SaveConfigDraft"
	DiscardConfigDraftName = "DiscardConfigDraft"

	DraftSavedAction     audit.Action = "config.draft_saved"
	DraftDiscardedAction audit.Action = "config.draft_discarded"

	// The largest draft block, as the column holds it.
	maxDraftBytes = 262144
)

// draftable are the sections whose blocks take a draft.
var draftable = []string{sectionTypes, sectionPolicies, sectionRoles}

// ListConfigDrafts answers the workspace's drafts.
type ListConfigDrafts struct{ Desk Desk }

func (h ListConfigDrafts) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListConfigDraftsName,
		Permission: service.PermissionStructure,
		Summary: "Lists the drafts saved beside the configuration in force - of a type with its form, " +
			"an SLA policy or a role - by section and key, with who saved each and when. Publishing " +
			"one is planning and applying its block.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var drafts []configrepo.Draft
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				drafts, err = h.Desk.Drafts.List(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := make([]any, 0, len(drafts))
			for _, d := range drafts {
				rows = append(rows, map[string]any(DraftOutput(d)))
			}
			return usecase.Output{"drafts": rows}, nil
		}),
	}
}

// DraftOutput is a draft as every door answers it (schema ConfigDraft).
func DraftOutput(d configrepo.Draft) usecase.Output {
	return usecase.Output{
		"section": d.Section, "key": d.Key, "block": map[string]any(d.Block), "saved_by": d.SavedBy.String(),
		"saved_at": d.SavedAt, "version": d.Version,
	}
}

// SaveConfigDraft saves a block's draft.
type SaveConfigDraft struct{ Desk Desk }

func (h SaveConfigDraft) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       SaveConfigDraftName,
		Permission: service.PermissionStructure,
		Summary: "Saves a draft of one block - a type with its form, an SLA policy or a role - " +
			"beside the one in force, which stays in force for everybody until the draft is " +
			"published by planning and applying its block. A draft may be unfinished: only its " +
			"attributes are checked to be the section's.",
		SideEffects: "Stores the draft and writes an audit entry. Changes no configuration in force.",
		TokenScope:  scopePlan,
		Input: []usecase.Field{
			{Name: "section", Kind: usecase.KindString, Required: true, Enum: draftable, Description: "The document's section."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The block's stable key."},
			{Name: "block", Kind: usecase.KindObject, Required: true, Description: "The block as the configuration document writes it."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The draft's version last read, for optimistic locking."},
		},
		Audit:    usecase.AuditDeclaration{Action: DraftSavedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a draft is not in force and not an item; the trail holds the write."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, DraftSavedAction, scopePlan, false); err != nil {
				return nil, err
			}
			return h.Desk.saveDraft(ctx, actor, in)
		}),
	}
}

func (d Desk) saveDraft(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	section, key := in.String("section"), in.String("key")
	block, err := draftBlock(section, key, in["block"])
	if err != nil {
		return nil, err
	}
	var out usecase.Output
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		at := d.Clock.Now()
		saved, ok, err := d.Drafts.Save(ctx, configrepo.Draft{
			Section: section, Key: key, Block: block, SavedBy: actor.AccountID, SavedAt: at,
		}, in.Int("expected_version"))
		if err != nil {
			return err
		}
		if !ok {
			return shared.ErrVersionConflict.WithDetail("config.draft_version_conflict")
		}
		out = DraftOutput(saved)
		return d.trail(ctx, actor, DraftSavedAction, at,
			audit.Change{Field: "block", Classification: audit.Open, To: section + "." + key},
			audit.Change{Field: "version", Classification: audit.Open, To: strconv.Itoa(saved.Version)})
	})
	return out, err
}

// draftBlock checks a draft's shape: a section that takes drafts, a stable key, and a block of
// the section's attributes within the column's size. What the block says is checked when it is
// published.
func draftBlock(name, key string, raw any) (domain.Block, error) {
	if !slices.Contains(draftable, name) {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/section", Code: "config.draft_section"}})
	}
	if !sampleKeyPattern.MatchString(key) {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/key", Code: "config.key_invalid"}})
	}
	normal, err := normalise(raw)
	block, ok := normal.(map[string]any)
	if err != nil || !ok {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/block", Code: "config.document_invalid"}})
	}
	var attributes []string
	for _, s := range sections() {
		if s.name == name {
			attributes = s.attributes
		}
	}
	for attribute := range block {
		if !slices.Contains(attributes, attribute) {
			return nil, domain.ErrRefused([]shared.FieldError{{Path: "/block/" + attribute, Code: "config.attribute_unknown",
				Params: map[string]string{"attribute": attribute}}})
		}
	}
	if encoded, err := json.Marshal(block); err != nil || len(encoded) > maxDraftBytes {
		return nil, domain.ErrRefused([]shared.FieldError{{Path: "/block", Code: "config.draft_too_large"}})
	}
	return domain.Block(block), nil
}

// DiscardConfigDraft throws a block's draft away.
type DiscardConfigDraft struct{ Desk Desk }

func (h DiscardConfigDraft) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        DiscardConfigDraftName,
		Permission:  service.PermissionStructure,
		Summary:     "Throws away the draft of one block; the block in force stays as it is.",
		SideEffects: "Removes the draft and writes an audit entry.",
		TokenScope:  scopePlan,
		Input: []usecase.Field{
			{Name: "section", Kind: usecase.KindString, Required: true, Enum: draftable, Description: "The document's section."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The block's stable key."},
		},
		Audit:    usecase.AuditDeclaration{Action: DraftDiscardedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a draft is not in force and not an item; the trail holds the write."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, DraftDiscardedAction, scopePlan, false); err != nil {
				return nil, err
			}
			section, key := in.String("section"), in.String("key")
			err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				gone, err := h.Desk.Drafts.Discard(ctx, section, key)
				if err != nil {
					return err
				}
				if !gone {
					return shared.ErrNotFound.WithDetail("config.draft_none")
				}
				return h.Desk.trail(ctx, actor, DraftDiscardedAction, h.Desk.Clock.Now(),
					audit.Change{Field: "block", Classification: audit.Open, To: section + "." + key})
			})
			return nil, err
		}),
	}
}

// publishedDrafts removes, in an apply's transaction, the drafts of the workspace's blocks it put
// in force as drafted.
func (d Desk) publishedDrafts(ctx context.Context, hub shared.ID, document domain.Document) error {
	if d.Drafts == nil || !hub.IsZero() {
		return nil
	}
	for _, name := range draftable {
		for key, block := range document[name] {
			if block == nil {
				continue
			}
			if err := d.Drafts.Published(ctx, name, key, block); err != nil {
				return err
			}
		}
	}
	return nil
}
