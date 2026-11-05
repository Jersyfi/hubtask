// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListWorkspaceSamplesName   = "ListWorkspaceSamples"
	MarkWorkspaceSampleName    = "MarkWorkspaceSample"
	RemoveWorkspaceSamplesName = "RemoveWorkspaceSamples"

	WorkspaceSampleMarkedAction   audit.Action = "config.workspace_sample_marked"
	WorkspaceSamplesRemovedAction audit.Action = "config.workspace_samples_removed"
)

// sampleRemoval is how one kind of sample content is removed: the use case removing it alone
// takes, and the field naming the object.
var sampleRemoval = map[domain.SampleKind]struct{ useCase, field string }{
	domain.SampleContainer:  {"TrashContainer", "container_id"},
	domain.SampleEntry:      {"TrashWorkItem", "item_id"},
	domain.SamplePage:       {"TrashPage", "page_id"},
	domain.SampleWhiteboard: {"TrashWhiteboard", "whiteboard_id"},
	domain.SampleGoal:       {"DeleteGoal", "goal_id"},
	domain.SamplePortfolio:  {"DeletePortfolio", "portfolio_id"},
	domain.SampleDashboard:  {"DeleteDashboard", "dashboard_id"},
	domain.SampleTimeRecord: {"DeleteTimeRecord", "record_id"},
}

func workspaceSampleOutput(m domain.SampleMark) map[string]any {
	var version any
	if m.Version != nil {
		version = *m.Version
	}
	return map[string]any{"key": m.Key, "kind": string(m.Kind), "object_id": m.ObjectID, "version": version}
}

// ListWorkspaceSamples answers the marks of the workspace's sample content (user-experience.md §3.6).
type ListWorkspaceSamples struct{ Desk Desk }

func (h ListWorkspaceSamples) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListWorkspaceSamplesName,
		Permission: service.PermissionStructure,
		Summary: "Lists what was marked as the workspace's sample content - hubs, collections, entries, " +
			"pages, whiteboards, goals, portfolios, dashboards and time records - by key, each mark " +
			"naming its object's kind, identifier and the version it was made at.",
		SideEffects: "None.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var marks []domain.SampleMark
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				marks, err = h.Desk.SampleMarks.List(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			rows := make([]any, 0, len(marks))
			for _, m := range marks {
				rows = append(rows, workspaceSampleOutput(m))
			}
			return usecase.Output{"samples": rows}, nil
		}),
	}
}

// MarkWorkspaceSample marks one object as the workspace's sample content.
type MarkWorkspaceSample struct{ Desk Desk }

func (h MarkWorkspaceSample) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       MarkWorkspaceSampleName,
		Permission: service.PermissionStructure,
		Summary: "Marks one object as the workspace's sample content under a key, so that one act " +
			"removes it with the rest. The same object under the same key again changes nothing; a " +
			"key marking another object is refused. A goal, portfolio, dashboard or time record is " +
			"later deleted only while it stands at the version given here.",
		SideEffects: "Writes the mark and an audit entry.",
		TokenScope:  scopeItemsWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The mark's key, a dotted lower-case word."},
			{Name: "kind", Kind: usecase.KindString, Required: true, Description: "What the object is."},
			{Name: "object_id", Kind: usecase.KindID, Required: true, Description: "The object."},
			{Name: "version", Kind: usecase.KindInt, Description: "The object's version as it was made."},
		},
		Audit:    usecase.AuditDeclaration{Action: WorkspaceSampleMarkedAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a mark changes no object."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, WorkspaceSampleMarkedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			mark, err := sampleMarkOf(in)
			if err != nil {
				return nil, err
			}
			var held domain.SampleMark
			err = h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				now := h.Desk.Clock.Now()
				held, err = h.Desk.SampleMarks.Mark(ctx, mark, now)
				if err != nil {
					return err
				}
				if held.Kind != mark.Kind || held.ObjectID != mark.ObjectID {
					return refused("config.workspace_sample_taken", "/key", map[string]string{"key": mark.Key})
				}
				return h.Desk.trail(ctx, actor, WorkspaceSampleMarkedAction, now,
					audit.Change{Field: "key", Classification: audit.Open, To: mark.Key},
					audit.Change{Field: "kind", Classification: audit.Open, To: string(mark.Kind)},
					audit.Change{Field: "object_id", Classification: audit.Open, To: mark.ObjectID})
			})
			if err != nil {
				return nil, err
			}
			return workspaceSampleOutput(held), nil
		}),
	}
}

func sampleMarkOf(in usecase.Input) (domain.SampleMark, error) {
	key := in.String("key")
	if !domain.ValidSampleKey(key) {
		return domain.SampleMark{}, refused("config.sample_key_invalid", "/key", nil)
	}
	kind := domain.SampleKind(in.String("kind"))
	if !kind.Known() {
		return domain.SampleMark{}, refused("config.sample_kind_unknown", "/kind", nil)
	}
	object, err := in.ID("object_id")
	if err != nil {
		return domain.SampleMark{}, err
	}
	mark := domain.SampleMark{Key: key, Kind: kind, ObjectID: object.String()}
	if _, given := in["version"]; given && in["version"] != nil {
		version := int64(in.Int("version"))
		if version < 0 {
			return domain.SampleMark{}, refused("config.sample_version_invalid", "/version", nil)
		}
		mark.Version = &version
	}
	return mark, nil
}

// RemoveWorkspaceSamples removes all of the workspace's sample content in one act.
type RemoveWorkspaceSamples struct{ Desk Desk }

func (h RemoveWorkspaceSamples) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RemoveWorkspaceSamplesName,
		Permission: service.PermissionStructure,
		Summary: "Removes everything marked as the workspace's sample content in one act, each object as " +
			"removing it alone does: hubs, collections, entries, pages and whiteboards into the trash; " +
			"goals, portfolios, dashboards and time records deleted only while they stand at the " +
			"version they were marked with, and otherwise kept and named. Nothing unmarked is touched.",
		SideEffects: "Trashes or deletes each marked object with everything doing so alone writes, " +
			"removes the marks of what went, and writes an audit entry with the counts.",
		TokenScope:  scopeItemsWrite,
		Destructive: true,
		Audit:       usecase.AuditDeclaration{Action: WorkspaceSamplesRemovedAction, TargetType: target, Severity: audit.SeverityNotice, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "each object's removal is its own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, WorkspaceSamplesRemovedAction, scopeItemsWrite, false); err != nil {
				return nil, err
			}
			removed, kept := 0, []any{}
			err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				marks, err := h.Desk.SampleMarks.List(ctx)
				if err != nil {
					return err
				}
				for _, m := range domain.RemovalOrder(marks) {
					code, err := h.Desk.removeSample(ctx, actor, m)
					if err != nil {
						return err
					}
					if code != "" {
						kept = append(kept, map[string]any{"key": m.Key, "kind": string(m.Kind), "object_id": m.ObjectID, "code": code})
						continue
					}
					if err := h.Desk.SampleMarks.Unmark(ctx, m.Key); err != nil {
						return err
					}
					removed++
				}
				return h.Desk.trail(ctx, actor, WorkspaceSamplesRemovedAction, h.Desk.Clock.Now(),
					audit.Change{Field: "removed", Classification: audit.Open, To: strconv.Itoa(removed)},
					audit.Change{Field: "kept", Classification: audit.Open, To: strconv.Itoa(len(kept))})
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"removed": removed, "kept": kept}, nil
		}),
	}
}

// removeSample removes one marked object as removing it alone does, and answers the message code
// of why it stays where it is kept. An object already gone - or gone with its hub - is removed.
func (d Desk) removeSample(ctx context.Context, actor appshared.ActorContext, m domain.SampleMark) (string, error) {
	how, known := sampleRemoval[m.Kind]
	if !known {
		return "config.sample_kind_unknown", nil
	}
	in := usecase.Input{how.field: m.ObjectID}
	if !m.Kind.Trashed() {
		// No trash takes it back: only what still stands as it was made goes.
		if m.Version == nil {
			return "config.sample_unversioned", nil
		}
		in["expected_version"] = int(*m.Version)
	}
	_, err := d.Catalogue.Invoke(ctx, how.useCase, actor, in)
	switch {
	case err == nil, errors.Is(err, shared.ErrNotFound), errors.Is(err, shared.ErrGone):
		return "", nil
	case errors.Is(err, shared.ErrVersionConflict):
		return "config.sample_changed", nil
	}
	return "", err
}
