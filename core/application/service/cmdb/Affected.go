// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names (UC-AST-09).
const (
	ListEntryCisName    = "ListEntryCis"
	AffectEntryCiName   = "AffectEntryCi"
	UnaffectEntryCiName = "UnaffectEntryCi"
)

// The audit codes of an entry's affected CIs.
const (
	CIAffectedAction   audit.Action = "cmdb.ci_affected"
	CIUnaffectedAction audit.Action = "cmdb.ci_unaffected"
)

// AffectedEntity is the change log's entity of an entry's affected CI, filed under the CI's hub.
const AffectedEntity = "cmdb_affected"

const (
	itemTarget = "work_item"
	// neighbourLimit bounds the other entries a collision and the related entries are read from.
	neighbourLimit = 500
)

// Affecting is what an entry's affected CIs need beside the inventory: the entries and who may
// see them.
type Affecting struct {
	Inventory Inventory
	Pointers  Pointers
}

func entryPath(s workrepo.EntrySummary) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !s.HubID.IsZero() {
		path = append(path, identity.HubScope(s.HubID))
	}
	return append(path, identity.CollectionScope(s.CollectionID), identity.ItemScope(s.ID))
}

// entry reads an entry's summary outside a write's transaction; an entry not there is not found.
func (a Affecting) entry(ctx context.Context, actor appshared.ActorContext, id shared.ID) (workrepo.EntrySummary, error) {
	var found []workrepo.EntrySummary
	err := a.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		found, err = a.Pointers.Entries.Entries(ctx, []shared.ID{id})
		return err
	})
	if err != nil {
		return workrepo.EntrySummary{}, err
	}
	if len(found) == 0 {
		return workrepo.EntrySummary{}, appshared.ItemNotFound(id)
	}
	return found[0], nil
}

// authorizeEntry asks the permission on the entry: READ to list, WRITE_ITEMS to change what it
// names.
func (a Affecting) authorizeEntry(ctx context.Context, actor appshared.ActorContext, s workrepo.EntrySummary, write bool, action audit.Action) error {
	permission, scope, does := service.PermissionRead, cisRead, service.ItemRead
	if write {
		permission, scope, does = service.PermissionWriteItems, cisWrite, service.ItemChange
	}
	return a.Inventory.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: entryPath(s), Action: action, TokenScope: scope, TargetType: itemTarget,
		TargetID: s.ID, On: access.ItemSubject{Does: does, ID: s.ID},
	})
}

func affectedOutput(a domain.Affected) usecase.Output {
	return usecase.Output{"id": a.ID.String(), "item_id": a.ItemID.String(), "ci_id": a.CIID.String(), "added_at": a.AddedAt}
}

func (a Affecting) announce(ctx context.Context, actor appshared.ActorContext, row domain.Affected, hub shared.ID, removed bool) error {
	s := a.Inventory
	if s.Changes == nil {
		return nil
	}
	change := changelog.Change{
		TenantID: actor.TenantID, Entity: AffectedEntity, EntityID: row.ID, Op: changelog.Upsert,
		ContainerID: hub, ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: affectedOutput(row),
	}
	if removed {
		change.Op, change.Payload = changelog.Delete, nil
	}
	return s.Changes.Record(ctx, change)
}

// ListEntryCis answers the CIs an entry names as affected, what their failure reaches, the other
// open entries on them, and the entries planned on them in an overlapping window.
type ListEntryCis struct{ Affecting Affecting }

// Descriptor is the catalogue entry.
func (h ListEntryCis) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListEntryCisName, Permission: service.PermissionRead,
		Summary: "Answers the configuration items an entry - an incident, a change - names as affected, those the " +
			"reader may read; the active services and offerings their failure reaches; the other open entries naming " +
			"them or a CI they reach; and the collisions - entries planned on the same CI or one it reaches in a " +
			"window overlapping the entry's start and due.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			return h.Affecting.read(ctx, actor, id)
		}),
	})
}

func (a Affecting) read(ctx context.Context, actor appshared.ActorContext, id shared.ID) (usecase.Output, error) {
	self, err := a.entry(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := a.authorizeEntry(ctx, actor, self, false, CIReadAction); err != nil {
		return nil, err
	}
	s := a.Inventory
	var rows []domain.Affected
	var cis []domain.CI
	reachedCIs := map[shared.ID]domain.CI{}
	var neighbours []domain.Affected
	var entries []workrepo.EntrySummary
	var reg registry
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if rows, err = s.Assurance.AffectedOf(ctx, id); err != nil {
			return err
		}
		ids := make([]shared.ID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.CIID)
		}
		if cis, err = s.Store.CIsByID(ctx, ids); err != nil {
			return err
		}
		if reg, err = s.registry(ctx); err != nil {
			return err
		}
		sourceSuffers, targetSuffers := reg.types.Suffering()
		var reachedIDs []shared.ID
		for _, ci := range cis {
			walked, err := s.Store.Walk(ctx, ci.ID, true, sourceSuffers, targetSuffers, impactDepth)
			if err != nil {
				return err
			}
			for _, r := range walked {
				reachedIDs = append(reachedIDs, r.CI)
			}
		}
		reached, err := s.Store.CIsByID(ctx, reachedIDs)
		if err != nil {
			return err
		}
		for _, ci := range reached {
			reachedCIs[ci.ID] = ci
		}
		near := append(append([]shared.ID{}, ids...), reachedIDs...)
		if neighbours, err = s.Assurance.AffectingAny(ctx, near, neighbourLimit); err != nil {
			return err
		}
		var others []shared.ID
		for _, n := range neighbours {
			if n.ItemID != id {
				others = append(others, n.ItemID)
			}
		}
		if len(others) > 0 {
			entries, err = a.Pointers.Entries.Entries(ctx, sortedIDs(others))
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	hubs := make([]shared.ID, 0, len(cis)+len(reachedCIs))
	for _, ci := range cis {
		hubs = append(hubs, ci.HubID)
	}
	for _, ci := range reachedCIs {
		hubs = append(hubs, ci.HubID)
	}
	readable, err := a.Pointers.readableHubs(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	visible, err := a.Pointers.visibleEntries(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	addedAt := map[shared.ID]domain.Affected{}
	for _, r := range rows {
		addedAt[r.CIID] = r
	}
	own := map[shared.ID]bool{}
	listed := []usecase.Output{}
	for _, ci := range cis {
		if !readable[ci.HubID] {
			continue
		}
		own[ci.ID] = true
		listed = append(listed, usecase.Output{
			"ci_id": ci.ID.String(), "hub_id": ci.HubID.String(), "class": ci.Class, "name": ci.Name,
			"stage": string(ci.Stage), "added_at": addedAt[ci.ID].AddedAt,
		})
	}
	reachedOwn := map[shared.ID]bool{}
	services := []usecase.Output{}
	for _, ci := range sortedCIs(reachedCIs) {
		if !readable[ci.HubID] || own[ci.ID] {
			continue
		}
		reachedOwn[ci.ID] = true
		if reg.classes.IsA(ci.Class, "service") && ci.Stage.Active() {
			services = append(services, usecase.Output{"ci_id": ci.ID.String(), "class": ci.Class, "name": ci.Name})
		}
	}
	related := []usecase.Output{}
	seen := map[shared.ID]bool{}
	var candidates []domain.Neighbour
	for _, n := range neighbours {
		e, ok := visible[n.ItemID]
		if !ok || e.Completed || e.Archived || !(own[n.CIID] || reachedOwn[n.CIID]) {
			continue
		}
		candidates = append(candidates, domain.Neighbour{ItemID: e.ID, CIID: n.CIID, Window: domain.Window{Start: e.StartAt, End: e.DueAt}})
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		related = append(related, entryOutput(e, n.CIID))
	}
	collisions := []usecase.Output{}
	for _, c := range domain.Collisions(id, domain.Window{Start: self.StartAt, End: self.DueAt}, own, reachedOwn, candidates) {
		e := visible[c.ItemID]
		row := entryOutput(e, c.CIID)
		row["direct"] = c.Direct
		collisions = append(collisions, row)
	}
	return usecase.Output{
		"item_id": id.String(), "cis": listed, "services": services, "related": related, "collisions": collisions,
	}, nil
}

func entryOutput(e workrepo.EntrySummary, ci shared.ID) usecase.Output {
	return usecase.Output{
		"item_id": e.ID.String(), "title": e.Title, "handle": e.Handle, "number": e.Number, "entry_type": e.EntryType,
		"ci_id": ci.String(), "start_at": e.StartAt, "due_at": e.DueAt,
	}
}

func sortedCIs(m map[shared.ID]domain.CI) []domain.CI {
	ids := make([]shared.ID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	out := make([]domain.CI, 0, len(ids))
	for _, id := range sortedIDs(ids) {
		out = append(out, m[id])
	}
	return out
}

// AffectEntryCi names a CI as affected by an entry.
type AffectEntryCi struct{ Affecting Affecting }

// Descriptor is the catalogue entry.
func (h AffectEntryCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: AffectEntryCiName, Permission: service.PermissionWriteItems,
		Summary: "Names a configuration item as affected by an entry: changing the entry needs WRITE_ITEMS on it, " +
			"and the CI must be one the writer may read. At most fifty per entry; naming one again changes nothing.",
		SideEffects: "Writes the affected CI, an audit entry and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
		},
		Audit: usecase.AuditDeclaration{Action: CIAffectedAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Affecting.change(ctx, actor, in, false)
		}),
	})
}

// UnaffectEntryCi removes a CI from an entry's affected ones.
type UnaffectEntryCi struct{ Affecting Affecting }

// Descriptor is the catalogue entry.
func (h UnaffectEntryCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnaffectEntryCiName, Permission: service.PermissionWriteItems,
		Summary:     "Removes a configuration item from those an entry names as affected; needs WRITE_ITEMS on the entry.",
		SideEffects: "Removes the affected CI, writes an audit entry and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
		},
		Audit: usecase.AuditDeclaration{Action: CIUnaffectedAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Affecting.change(ctx, actor, in, true)
		}),
	})
}

func (a Affecting) change(ctx context.Context, actor appshared.ActorContext, in usecase.Input, remove bool) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	ciID, err := in.ID("ci_id")
	if err != nil {
		return nil, err
	}
	action := CIAffectedAction
	if remove {
		action = CIUnaffectedAction
	}
	self, err := a.entry(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	if err := a.authorizeEntry(ctx, actor, self, true, action); err != nil {
		return nil, err
	}
	if self.Archived {
		return nil, shared.ErrConflict.WithDetail("items.archived")
	}
	s := a.Inventory
	ci, err := s.placeOf(ctx, actor, ciID)
	if err != nil {
		return nil, err
	}
	readable, err := a.Pointers.readableHubs(ctx, actor, []shared.ID{ci.HubID})
	if err != nil {
		return nil, err
	}
	if !readable[ci.HubID] {
		return nil, shared.ErrNotFound.WithDetail("cmdb.ci_not_found")
	}
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if remove {
			row, err := s.Assurance.RemoveAffected(ctx, itemID, ciID)
			if err != nil {
				return err
			}
			if err := a.announce(ctx, actor, row, ci.HubID, true); err != nil {
				return err
			}
			return s.record(ctx, actor, action, itemTarget, itemID, audit.Change{Field: "ci_id", Classification: audit.Open, To: ciID.String()})
		}
		current, err := s.Assurance.AffectedOf(ctx, itemID)
		if err != nil {
			return err
		}
		for _, r := range current {
			if r.CIID == ciID {
				return nil
			}
		}
		if len(current) >= domain.MaxAffectedPerEntry {
			return invalid("/ci_id", "cmdb.affected_too_many")
		}
		row := domain.Affected{ID: s.IDs.NewID(), ItemID: itemID, CIID: ciID, AddedBy: actor.AccountID, AddedAt: s.Clock.Now()}
		if _, err := s.Assurance.AddAffected(ctx, row); err != nil {
			return err
		}
		if err := a.announce(ctx, actor, row, ci.HubID, false); err != nil {
			return err
		}
		return s.record(ctx, actor, action, itemTarget, itemID, audit.Change{Field: "ci_id", Classification: audit.Open, To: ciID.String()})
	})
	if err != nil {
		return nil, err
	}
	return a.read(ctx, actor, itemID)
}
