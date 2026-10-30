// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"strconv"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	CreateReleaseName         = "CreateRelease"
	UpdateReleaseName         = "UpdateRelease"
	ListReleasesName          = "ListReleases"
	ReadReleaseName           = "ReadRelease"
	DeleteReleaseName         = "DeleteRelease"
	AddReleaseEntriesName     = "AddReleaseEntries"
	RemoveReleaseEntriesName  = "RemoveReleaseEntries"
	PublishReleaseName        = "PublishRelease"
	releaseTarget             = "release"
	createPageName            = "CreatePage"
	maxReleasesListed         = 200
	maxEntriesRead            = domain.MaxReleaseEntries
	releaseDateInvalid        = "releases.date_invalid"
	releaseEntriesRequired    = "releases.entries_required"
	releaseEntryElsewhere     = "releases.entry_elsewhere"
	releaseCarryTargetInvalid = "releases.carry_target_invalid"
)

// The stable audit codes of releases (audit.md §2). A release's name and description are a
// person's words and go as fingerprints (rule 10).
const (
	ReleaseCreatedAction        audit.Action = "release.created"
	ReleaseUpdatedAction        audit.Action = "release.updated"
	ReleaseDeletedAction        audit.Action = "release.deleted"
	ReleasePublishedAction      audit.Action = "release.published"
	ReleaseEntriesAddedAction   audit.Action = "release.entries_added"
	ReleaseEntriesRemovedAction audit.Action = "release.entries_removed"
	ReleaseReadAction           audit.Action = "release.read"
)

// The entities releases are in the change log and the replica as (offline-sync.md §3.1), filed
// under the release's hub: a release itself, and an entry's membership by the entry's identifier,
// since an entry is in at most one release.
const (
	ReleaseEntity      = "release"
	ReleaseEntryEntity = "release_entry"
)

// ReleaseOutput is a release as every door returns it (schema Release).
func ReleaseOutput(r domain.Release) usecase.Output {
	var page any
	if !r.NotesPageID.IsZero() {
		page = r.NotesPageID.String()
	}
	return usecase.Output{
		"id": r.ID.String(), "hub_id": r.HubID.String(), "name": r.Name, "description": r.Description,
		"release_on": r.ReleaseOn.String(), "state": string(r.State), "released_at": timeOrNil(r.ReleasedAt),
		"notes_page_id": page, "version": r.Version,
	}
}

// ReleaseEntryRecord is an entry's membership as a device keeps it.
func ReleaseEntryRecord(entryID, releaseID, hubID shared.ID) usecase.Output {
	return usecase.Output{"item_id": entryID.String(), "release_id": releaseID.String(), "hub_id": hubID.String()}
}

func releaseDay(in usecase.Input, field string) (domain.Day, error) {
	at, err := time.Parse(time.DateOnly, in.String(field))
	if err != nil {
		return 0, invalid("/"+field, releaseDateInvalid)
	}
	return domain.DayFrom(at.Year(), at.Month(), at.Day()), nil
}

// hub reads the hub a release belongs to and asks the permission at it; a collection has no
// releases of its own - its hub's are its.
func (p Planner) hub(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission,
	action audit.Action, scope string,
) (work.Container, error) {
	if id.IsZero() {
		return work.Container{}, invalid("/hub_id", "releases.hub_required")
	}
	container, err := p.plan(ctx, actor, id, permission, action, scope)
	if err != nil {
		return work.Container{}, err
	}
	if container.Type != work.ContainerHub {
		return work.Container{}, invalid("/hub_id", "releases.hub_required")
	}
	return container, nil
}

// release reads a release and asks the permission at its hub.
func (p Planner) release(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, permission service.Permission,
	action audit.Action, scope string,
) (domain.Release, error) {
	if id.IsZero() {
		return domain.Release{}, invalid("/release_id", "releases.id_required")
	}
	var release domain.Release
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Releases.Find(ctx, id)
		release = found
		return err
	}); err != nil {
		return domain.Release{}, err
	}
	if _, err := p.plan(ctx, actor, release.HubID, permission, action, scope); err != nil {
		return domain.Release{}, err
	}
	return release, nil
}

// releaseChanged records a release as it now stands, or - payload nil - its removal.
func (p Planner) releaseChanged(ctx context.Context, actor appshared.ActorContext, r domain.Release, removed bool) error {
	var payload usecase.Output
	if !removed {
		payload = ReleaseOutput(r)
	}
	return p.changed(ctx, actor, ReleaseEntity, r.ID, r.HubID, payload)
}

// releaseMembersChanged records the entries' release, or - release zero - that they are in none.
func (p Planner) releaseMembersChanged(ctx context.Context, actor appshared.ActorContext, hub, release shared.ID, entries []shared.ID) error {
	for _, entry := range entries {
		var payload usecase.Output
		if !release.IsZero() {
			payload = ReleaseEntryRecord(entry, release, hub)
		}
		if err := p.changed(ctx, actor, ReleaseEntryEntity, entry, hub, payload); err != nil {
			return err
		}
	}
	return nil
}

// CreateRelease plans a release of a hub: its name, description and the day it is meant to go
// out (PLN-08).
type CreateRelease struct{ Planner Planner }

// Execute plans the release.
func (h CreateRelease) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, fields domain.ReleaseFields) (domain.Release, error) {
	hub, err := h.Planner.hub(ctx, actor, hubID, service.PermissionWriteItems, ReleaseCreatedAction, itemsWrite)
	if err != nil {
		return domain.Release{}, err
	}
	now := h.Planner.Clock.Now()
	release, err := domain.NewRelease(domain.NewReleaseInput{
		ID: h.Planner.IDs.NewID(), TenantID: actor.TenantID, HubID: hub.ID, CreatedBy: actor.AccountID,
		Fields: fields, Now: now, Text: h.Planner.Text,
	})
	if err != nil {
		return domain.Release{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Releases.Insert(ctx, release); err != nil {
			return err
		}
		if err := h.Planner.releaseChanged(ctx, actor, release, false); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleaseCreatedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.ID.String()},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: release.Name},
			audit.Change{Field: "description", Classification: audit.Sensitive, To: release.Description},
			audit.Change{Field: "release_on", Classification: audit.Open, To: release.ReleaseOn.String()})
	})
	if err != nil {
		return domain.Release{}, err
	}
	return release, nil
}

func releaseFieldInputs(required bool) []usecase.Field {
	return []usecase.Field{
		{Name: "name", Kind: usecase.KindString, Required: required, Description: "Up to 120 characters, on one line - a version such as 2.4."},
		{Name: "description", Kind: usecase.KindString, Description: "What the release is about, up to 2000 characters; it opens its notes."},
		{Name: "release_on", Kind: usecase.KindString, Required: required, Description: "The day it is meant to go out, YYYY-MM-DD."},
	}
}

// Descriptor is the catalogue entry.
func (h CreateRelease) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreateReleaseName, Permission: service.PermissionWriteItems,
		Summary: "Plans a release of a hub: a name, a description and the day it is meant to go out. It starts " +
			"PLANNED; entries of the hub's collections join it until it is released.",
		SideEffects: "Writes the release and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append([]usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the release ships."},
		}, releaseFieldInputs(true)...),
		Audit:    usecase.AuditDeclaration{Action: ReleaseCreatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a release is planned beside the entries and changes none."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			day, err := releaseDay(in, "release_on")
			if err != nil {
				return nil, err
			}
			release, err := h.Execute(ctx, actor, hub, domain.ReleaseFields{Name: in.String("name"), Description: in.String("description"), ReleaseOn: day})
			if err != nil {
				return nil, err
			}
			return ReleaseOutput(release), nil
		}),
	}, releaseTarget)
}

// UpdateRelease changes a planned release's name, description or day (PLN-08).
type UpdateRelease struct{ Planner Planner }

// UpdateReleaseCommand is the input, typed; a nil field keeps what stands.
type UpdateReleaseCommand struct {
	ReleaseID         shared.ID
	Name, Description *string
	ReleaseOn         *domain.Day
	Version           int
}

// Execute changes the release.
func (h UpdateRelease) Execute(ctx context.Context, actor appshared.ActorContext, cmd UpdateReleaseCommand) (domain.Release, error) {
	release, err := h.Planner.release(ctx, actor, cmd.ReleaseID, service.PermissionWriteItems, ReleaseUpdatedAction, itemsWrite)
	if err != nil {
		return domain.Release{}, err
	}
	if cmd.Version != 0 && cmd.Version != release.Version {
		return domain.Release{}, shared.ErrVersionConflict.WithDetail("releases.version_conflict")
	}
	before := release
	fields := domain.ReleaseFields{Name: release.Name, Description: release.Description, ReleaseOn: release.ReleaseOn}
	if cmd.Name != nil {
		fields.Name = *cmd.Name
	}
	if cmd.Description != nil {
		fields.Description = *cmd.Description
	}
	if cmd.ReleaseOn != nil {
		fields.ReleaseOn = *cmd.ReleaseOn
	}
	now := h.Planner.Clock.Now()
	if err := release.Change(fields, now, h.Planner.Text); err != nil {
		return domain.Release{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Releases.Update(ctx, release, before.Version); err != nil {
			return err
		}
		changed := release
		changed.Version = before.Version + 1
		if err := h.Planner.releaseChanged(ctx, actor, changed, false); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleaseUpdatedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "name", Classification: audit.Sensitive, From: before.Name, To: release.Name},
			audit.Change{Field: "description", Classification: audit.Sensitive, From: before.Description, To: release.Description},
			audit.Change{Field: "release_on", Classification: audit.Open, From: before.ReleaseOn.String(), To: release.ReleaseOn.String()})
	})
	if err != nil {
		return domain.Release{}, err
	}
	release.Version = before.Version + 1
	return release, nil
}

// Descriptor is the catalogue entry.
func (h UpdateRelease) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdateReleaseName, Permission: service.PermissionWriteItems,
		Summary:     "Changes a planned release's name, description or day; a released one is history (releases.released).",
		SideEffects: "Writes the release and an audit entry.",
		TokenScope:  itemsWrite,
		Input: append(append([]usecase.Field{
			{Name: "release_id", Kind: usecase.KindID, Required: true, Description: "The release."},
		}, releaseFieldInputs(false)...),
			usecase.Field{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a stale one is a conflict."}),
		Audit:    usecase.AuditDeclaration{Action: ReleaseUpdatedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "a release's own fields change no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("release_id")
			if err != nil {
				return nil, err
			}
			cmd := UpdateReleaseCommand{ReleaseID: id, Name: in.OptionalString("name"), Description: in.OptionalString("description"), Version: in.Int("expected_version")}
			if in.Present("release_on") {
				day, err := releaseDay(in, "release_on")
				if err != nil {
					return nil, err
				}
				cmd.ReleaseOn = &day
			}
			release, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return ReleaseOutput(release), nil
		}),
	}, releaseTarget)
}

// ListReleases reads a hub's releases, the latest day first (PLN-08).
type ListReleases struct{ Planner Planner }

// Execute lists them.
func (h ListReleases) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, state domain.ReleaseState) ([]domain.Release, error) {
	if state != "" && state != domain.ReleasePlanned && state != domain.ReleaseReleased {
		return nil, invalid("/state", "releases.state_unknown")
	}
	if _, err := h.Planner.hub(ctx, actor, hubID, service.PermissionRead, ReleaseReadAction, itemsRead); err != nil {
		return nil, err
	}
	var out []domain.Release
	err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Planner.Releases.List(ctx, hubID, state, maxReleasesListed)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListReleases) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListReleasesName, Permission: service.PermissionRead,
		Summary:     "Lists a hub's releases, the latest day first, at most 200: their day, state and the page their notes went to.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "state", Kind: usecase.KindString, Enum: []string{"PLANNED", "RELEASED"}, Description: "Only releases in this state; every state without."},
		},
		Audit: usecase.AuditDeclaration{Action: ReleaseReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			releases, err := h.Execute(ctx, actor, hub, domain.ReleaseState(in.String("state")))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(releases))
			for _, r := range releases {
				data = append(data, ReleaseOutput(r))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, releaseTarget)
}

// ReleaseView is a release with the entries its reader may see.
type ReleaseView struct {
	Release domain.Release
	Entries []planrepo.ReleaseMember
}

// visibleReleaseMembers answers the members a reader may see (T-04).
func (p Planner) visibleReleaseMembers(ctx context.Context, actor appshared.ActorContext, members []planrepo.ReleaseMember) ([]planrepo.ReleaseMember, error) {
	paths := make([][]identity.Scope, 0, len(members))
	for _, m := range members {
		paths = append(paths, entryPath(m.HubID, m.CollectionID, m.EntryID))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return nil, err
	}
	out := make([]planrepo.ReleaseMember, 0, len(members))
	for i, m := range members {
		if visible[i] {
			out = append(out, m)
		}
	}
	return out, nil
}

// ReadRelease reads a release with the entries its reader may see, each done or still open
// (PLN-08).
type ReadRelease struct{ Planner Planner }

// Execute reads the release.
func (h ReadRelease) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (ReleaseView, error) {
	release, err := h.Planner.release(ctx, actor, id, service.PermissionRead, ReleaseReadAction, itemsRead)
	if err != nil {
		return ReleaseView{}, err
	}
	var members []planrepo.ReleaseMember
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		members, err = h.Planner.Releases.Entries(ctx, release.ID, maxEntriesRead)
		return err
	}); err != nil {
		return ReleaseView{}, err
	}
	visible, err := h.Planner.visibleReleaseMembers(ctx, actor, members)
	if err != nil {
		return ReleaseView{}, err
	}
	return ReleaseView{Release: release, Entries: visible}, nil
}

// Output is the shape every door returns (schema ReleaseDetail).
func (v ReleaseView) Output() usecase.Output {
	out := ReleaseOutput(v.Release)
	entries := make([]usecase.Output, 0, len(v.Entries))
	done := 0
	for _, e := range v.Entries {
		if e.Done {
			done++
		}
		entries = append(entries, usecase.Output{
			"item_id": e.EntryID.String(), "title": e.Title, "type_name": e.TypeName, "done": e.Done,
			"collection_id": e.CollectionID.String(), "added_at": e.AddedAt.UTC(),
		})
	}
	out["entries"], out["done_count"], out["open_count"] = entries, done, len(v.Entries)-done
	return out
}

// Descriptor is the catalogue entry.
func (h ReadRelease) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadReleaseName, Permission: service.PermissionRead,
		Summary: "Reads a release with the entries in it the reader may see - each with its title, its type's " +
			"name and whether it is done - and how many are done and open.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "release_id", Kind: usecase.KindID, Required: true, Description: "The release."},
		},
		Audit: usecase.AuditDeclaration{Action: ReleaseReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("release_id")
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	}, releaseTarget)
}

// DeleteRelease removes a planned release; its entries are in none afterwards (PLN-08). A
// released one is the hub's record and stays.
type DeleteRelease struct{ Planner Planner }

// Execute deletes it.
func (h DeleteRelease) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	release, err := h.Planner.release(ctx, actor, id, service.PermissionWriteItems, ReleaseDeletedAction, itemsWrite)
	if err != nil {
		return err
	}
	if err := release.Changeable(); err != nil {
		return err
	}
	now := h.Planner.Clock.Now()
	return h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		members, err := h.Planner.Releases.Entries(ctx, release.ID, maxEntriesRead)
		if err != nil {
			return err
		}
		if err := h.Planner.Releases.Delete(ctx, release.ID); err != nil {
			return err
		}
		if err := h.Planner.releaseMembersChanged(ctx, actor, release.HubID, "", memberIDs(members)); err != nil {
			return err
		}
		if err := h.Planner.releaseChanged(ctx, actor, release, true); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleaseDeletedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "hub_id", Classification: audit.Open, From: release.HubID.String()})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteRelease) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteReleaseName, Permission: service.PermissionWriteItems,
		Summary: "Deletes a planned release; its entries are in no release afterwards and are otherwise " +
			"unchanged. A released one is the hub's record and is not deleted (releases.released).",
		SideEffects: "Deletes the release with its memberships and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input: []usecase.Field{
			{Name: "release_id", Kind: usecase.KindID, Required: true, Description: "The release."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReleaseDeletedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "deleting a planned release changes no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("release_id")
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, h.Execute(ctx, actor, id)
		}),
	}, releaseTarget)
}

func memberIDs(members []planrepo.ReleaseMember) []shared.ID {
	ids := make([]shared.ID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.EntryID)
	}
	return ids
}

// hubEntries checks that every entry named lives in one of the hub's collections and that the
// actor may change it; one out of reach answers as not found (T-04).
func (p Planner) hubEntries(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, items []shared.ID) error {
	if len(items) == 0 {
		return invalid("/item_ids", releaseEntriesRequired)
	}
	if len(items) > domain.MaxReleaseEntries {
		return shared.ErrValidation.WithDetail("releases.too_many_entries").
			WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxReleaseEntries)})
	}
	var summaries []workrepo.EntrySummary
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = p.References.Entries(ctx, items)
		return err
	}); err != nil {
		return err
	}
	found := make(map[shared.ID]workrepo.EntrySummary, len(summaries))
	for _, s := range summaries {
		found[s.ID] = s
	}
	paths := make([][]identity.Scope, 0, len(items))
	for _, item := range items {
		s := found[item]
		paths = append(paths, entryPath(s.HubID, s.CollectionID, item))
	}
	visible, err := p.permitted(ctx, actor, paths)
	if err != nil {
		return err
	}
	for i, item := range items {
		s, ok := found[item]
		if !ok || !visible[i] {
			return appshared.ItemNotFound(item)
		}
		if s.HubID != hubID {
			return invalid("/item_ids", releaseEntryElsewhere)
		}
	}
	return nil
}

// AddReleaseEntries puts entries of the hub's collections into a planned release (PLN-08). An
// entry is in one release at a time.
type AddReleaseEntries struct{ Planner Planner }

// Execute adds them, all or none.
func (h AddReleaseEntries) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, items []shared.ID) (domain.Release, error) {
	release, err := h.Planner.release(ctx, actor, id, service.PermissionWriteItems, ReleaseEntriesAddedAction, itemsWrite)
	if err != nil {
		return domain.Release{}, err
	}
	if err := release.Changeable(); err != nil {
		return domain.Release{}, err
	}
	if err := h.Planner.hubEntries(ctx, actor, release.HubID, items); err != nil {
		return domain.Release{}, err
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		of, err := h.Planner.Releases.Of(ctx, items)
		if err != nil {
			return err
		}
		already := map[shared.ID]bool{}
		for _, m := range of {
			if m.ReleaseID != release.ID {
				return shared.ErrConflict.WithDetail("releases.entry_in_another_release").
					WithParams(map[string]string{"item_id": m.EntryID.String()})
			}
			already[m.EntryID] = true
		}
		count, err := h.Planner.Releases.Count(ctx, release.ID)
		if err != nil {
			return err
		}
		added := make([]shared.ID, 0, len(items))
		for _, item := range items {
			if !already[item] {
				added = append(added, item)
			}
		}
		if count+len(added) > domain.MaxReleaseEntries {
			return shared.ErrValidation.WithDetail("releases.too_many_entries").
				WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxReleaseEntries)})
		}
		for _, item := range added {
			if err := h.Planner.Releases.Add(ctx, release.ID, item, now); err != nil {
				return err
			}
		}
		if err := h.Planner.releaseMembersChanged(ctx, actor, release.HubID, release.ID, added); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleaseEntriesAddedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "entries", Classification: audit.Open, To: strconv.Itoa(len(added))})
	})
	return release, err
}

func releaseEntriesInputs() []usecase.Field {
	return []usecase.Field{
		{Name: "release_id", Kind: usecase.KindID, Required: true, Description: "The release."},
		{Name: "item_ids", Kind: usecase.KindIDList, Required: true, Description: "The entries, of the release's hub."},
	}
}

func releaseEntriesHandler(execute func(context.Context, appshared.ActorContext, shared.ID, []shared.ID) (domain.Release, error)) usecase.Handler {
	return usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
		id, err := in.ID("release_id")
		if err != nil {
			return nil, err
		}
		items, err := in.IDList("item_ids")
		if err != nil {
			return nil, err
		}
		release, err := execute(ctx, actor, id, items)
		if err != nil {
			return nil, err
		}
		return ReleaseOutput(release), nil
	})
}

// Descriptor is the catalogue entry.
func (h AddReleaseEntries) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AddReleaseEntriesName, Permission: service.PermissionWriteItems,
		Summary: "Puts entries of the hub's collections into a planned release, all or none; a release holds at " +
			"most 200. An entry is in one release at a time (releases.entry_in_another_release).",
		SideEffects: "Writes the memberships and an audit entry.",
		TokenScope:  itemsWrite,
		Input:       releaseEntriesInputs(),
		Audit:       usecase.AuditDeclaration{Action: ReleaseEntriesAddedAction},
		Activity:    usecase.ActivityDeclaration{Exempt: "a membership is the release's; the entry itself does not change."},
		Handler:     releaseEntriesHandler(h.Execute),
	}, releaseTarget)
}

// RemoveReleaseEntries takes entries out of a planned release (PLN-08).
type RemoveReleaseEntries struct{ Planner Planner }

// Execute removes them.
func (h RemoveReleaseEntries) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID, items []shared.ID) (domain.Release, error) {
	release, err := h.Planner.release(ctx, actor, id, service.PermissionWriteItems, ReleaseEntriesRemovedAction, itemsWrite)
	if err != nil {
		return domain.Release{}, err
	}
	if err := release.Changeable(); err != nil {
		return domain.Release{}, err
	}
	if len(items) == 0 {
		return domain.Release{}, invalid("/item_ids", releaseEntriesRequired)
	}
	now := h.Planner.Clock.Now()
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed := make([]shared.ID, 0, len(items))
		for _, item := range items {
			was, err := h.Planner.Releases.Remove(ctx, release.ID, item)
			if err != nil {
				return err
			}
			if was {
				removed = append(removed, item)
			}
		}
		if err := h.Planner.releaseMembersChanged(ctx, actor, release.HubID, "", removed); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleaseEntriesRemovedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "entries", Classification: audit.Open, To: strconv.Itoa(len(removed))})
	})
	return release, err
}

// Descriptor is the catalogue entry.
func (h RemoveReleaseEntries) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveReleaseEntriesName, Permission: service.PermissionWriteItems,
		Summary:     "Takes entries out of a planned release; an entry not in it is passed over.",
		SideEffects: "Removes the memberships and writes an audit entry.",
		TokenScope:  itemsWrite,
		Input:       releaseEntriesInputs(),
		Audit:       usecase.AuditDeclaration{Action: ReleaseEntriesRemovedAction},
		Activity:    usecase.ActivityDeclaration{Exempt: "a membership is the release's; the entry itself does not change."},
		Handler:     releaseEntriesHandler(h.Execute),
	}, releaseTarget)
}

// PublishReleaseCommand is the input, typed.
type PublishReleaseCommand struct {
	ReleaseID shared.ID
	// WriteNotes writes the release's notes as a page of the hub's space, under NotesParentID
	// when one is named.
	WriteNotes    bool
	NotesParentID shared.ID
	// CarryTo is a planned release of the same hub the open entries move into; zero takes them
	// out of the release.
	CarryTo shared.ID
	Version int
}

// PublishRelease marks a planned release as out (PLN-08): what is done ships in it, what is open
// moves into the next release or out of it, and - when asked - its notes are written as a page of
// the hub's knowledge space, naming the done entries the releaser may see. The page is written by
// CreatePage, so it is checked, recorded and kept as one by hand is.
type PublishRelease struct{ Planner Planner }

// Execute releases it.
func (h PublishRelease) Execute(ctx context.Context, actor appshared.ActorContext, cmd PublishReleaseCommand) (domain.Release, error) {
	release, err := h.Planner.release(ctx, actor, cmd.ReleaseID, service.PermissionWriteItems, ReleasePublishedAction, itemsWrite)
	if err != nil {
		return domain.Release{}, err
	}
	if err := release.Changeable(); err != nil {
		return domain.Release{}, err
	}
	if cmd.Version != 0 && cmd.Version != release.Version {
		return domain.Release{}, shared.ErrVersionConflict.WithDetail("releases.version_conflict")
	}
	var carry domain.Release
	if !cmd.CarryTo.IsZero() {
		if carry, err = h.Planner.release(ctx, actor, cmd.CarryTo, service.PermissionWriteItems, ReleasePublishedAction, itemsWrite); err != nil {
			return domain.Release{}, err
		}
		if carry.ID == release.ID || carry.HubID != release.HubID || carry.State != domain.ReleasePlanned {
			return domain.Release{}, invalid("/carry_to", releaseCarryTargetInvalid)
		}
	}
	var members []planrepo.ReleaseMember
	if err := h.Planner.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		members, err = h.Planner.Releases.Entries(ctx, release.ID, maxEntriesRead)
		return err
	}); err != nil {
		return domain.Release{}, err
	}
	open := make([]shared.ID, 0, len(members))
	for _, m := range members {
		if !m.Done {
			open = append(open, m.EntryID)
		}
	}
	var page shared.ID
	if cmd.WriteNotes {
		if page, err = h.writeNotes(ctx, actor, release, members, cmd.NotesParentID); err != nil {
			return domain.Release{}, err
		}
	}
	before := release.Version
	now := h.Planner.Clock.Now()
	if err := release.Release(page, now); err != nil {
		return domain.Release{}, err
	}
	err = h.Planner.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Planner.Releases.Update(ctx, release, before); err != nil {
			return err
		}
		for _, entry := range open {
			if _, err := h.Planner.Releases.Remove(ctx, release.ID, entry); err != nil {
				return err
			}
			if !carry.ID.IsZero() {
				if err := h.Planner.Releases.Add(ctx, carry.ID, entry, now); err != nil {
					return err
				}
			}
		}
		if err := h.Planner.releaseMembersChanged(ctx, actor, release.HubID, carry.ID, open); err != nil {
			return err
		}
		changed := release
		changed.Version = before + 1
		if err := h.Planner.releaseChanged(ctx, actor, changed, false); err != nil {
			return err
		}
		return h.Planner.record(ctx, actor, ReleasePublishedAction, releaseTarget, release.ID, now,
			audit.Change{Field: "state", Classification: audit.Open, From: string(domain.ReleasePlanned), To: string(domain.ReleaseReleased)},
			audit.Change{Field: "open_entries", Classification: audit.Open, To: strconv.Itoa(len(open))},
			audit.Change{Field: "carry_to", Classification: audit.Open, To: carry.ID.String()},
			audit.Change{Field: "notes_page_id", Classification: audit.Open, To: page.String()})
	})
	if err != nil {
		return domain.Release{}, err
	}
	release.Version = before + 1
	return release, nil
}

// writeNotes writes the notes page, naming the done entries the releaser may see (T-04).
func (h PublishRelease) writeNotes(
	ctx context.Context, actor appshared.ActorContext, release domain.Release, members []planrepo.ReleaseMember, parent shared.ID,
) (shared.ID, error) {
	visible, err := h.Planner.visibleReleaseMembers(ctx, actor, members)
	if err != nil {
		return "", err
	}
	done := make([]domain.NoteEntry, 0, len(visible))
	for _, m := range visible {
		if m.Done {
			done = append(done, domain.NoteEntry{ID: m.EntryID, Title: m.Title, TypeName: m.TypeName})
		}
	}
	in := usecase.Input{"hub_id": release.HubID.String(), "title": release.Name, "body": domain.Notes(release, done)}
	if !parent.IsZero() {
		in["parent_id"] = parent.String()
	}
	out, err := h.Planner.Catalogue.Invoke(ctx, createPageName, actor, in)
	if err != nil {
		return "", err
	}
	id, _ := out["id"].(string)
	if id == "" {
		return "", shared.ErrInternal.WithDetail("releases.notes_unwritten")
	}
	return shared.ID(id), nil
}

// Descriptor is the catalogue entry.
func (h PublishRelease) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: PublishReleaseName, Permission: service.PermissionWriteItems,
		Summary: "Marks a planned release as released. Its done entries ship in it; its open ones move into another " +
			"planned release of the hub (carry_to) or out of it. With write_notes, its notes - the description and " +
			"the done entries the caller may see, under their type's name - are written as a page of the hub's " +
			"space, which the release then names.",
		SideEffects: "Writes the release, moves its open entries, writes the notes page through CreatePage when " +
			"asked, and an audit entry.",
		TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "release_id", Kind: usecase.KindID, Required: true, Description: "The release."},
			{Name: "write_notes", Kind: usecase.KindBool, Description: "Write the release notes as a page of the hub's space."},
			{Name: "notes_parent_id", Kind: usecase.KindID, Description: "The page the notes sit under; the top of the space without."},
			{Name: "carry_to", Kind: usecase.KindID, Description: "A planned release of the same hub the open entries move into."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a stale one is a conflict."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReleasePublishedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "releasing changes no entry; its notes page records its own activity."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("release_id")
			if err != nil {
				return nil, err
			}
			cmd := PublishReleaseCommand{ReleaseID: id, WriteNotes: in.Bool("write_notes"), Version: in.Int("expected_version")}
			for field, target := range map[string]*shared.ID{"notes_parent_id": &cmd.NotesParentID, "carry_to": &cmd.CarryTo} {
				if in.String(field) != "" {
					if *target, err = in.ID(field); err != nil {
						return nil, err
					}
				}
			}
			release, err := h.Execute(ctx, actor, cmd)
			if err != nil {
				return nil, err
			}
			return ReleaseOutput(release), nil
		}),
	}, releaseTarget)
}
