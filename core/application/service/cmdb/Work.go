// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ListCiWorkName is the use case's name (UC-AST-09 check 7).
const ListCiWorkName = "ListCiWork"

// The reasons an entry is on a CI: it names the CI itself, or a CI beneath it.
const (
	workNames   = "NAMES"
	workBeneath = "BENEATH"
)

// maxCompletedWork bounds the completed entries a CI lists after its open ones.
const maxCompletedWork = 20

// ListCiWork answers the entries a CI is part of: those naming it as affected and those naming a
// CI whose failure it suffers.
type ListCiWork struct{ Affecting Affecting }

// Descriptor is the catalogue entry.
func (h ListCiWork) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiWorkName, Permission: service.PermissionRead,
		Summary: "Answers the entries - tickets, changes, tasks - a configuration item is part of: every entry " +
			"naming it as affected (NAMES) or naming a CI beneath it, one whose failure it suffers up to eight steps " +
			"(BENEATH, with the nearest such CI), each once; open ones first, then at most twenty completed ones. " +
			"Only entries the reader may see, never archived ones.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			return h.Affecting.work(ctx, actor, id)
		}),
	})
}

func (a Affecting) work(ctx context.Context, actor appshared.ActorContext, id shared.ID) (usecase.Output, error) {
	s := a.Inventory
	ci, err := s.placeOf(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeHub(ctx, actor, ci.HubID, false, CIReadAction, ciTarget, id); err != nil {
		return nil, err
	}
	depth := map[shared.ID]int{}
	beneath := map[shared.ID]domain.CI{}
	var rows []domain.Affected
	var entries []workrepo.EntrySummary
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		reg, err := s.registry(ctx)
		if err != nil {
			return err
		}
		sourceSuffers, targetSuffers := reg.types.Suffering()
		walked, err := s.Store.Walk(ctx, id, false, sourceSuffers, targetSuffers, impactDepth)
		if err != nil {
			return err
		}
		near := []shared.ID{id}
		for _, r := range walked {
			if r.CI != id {
				depth[r.CI] = r.Depth
				near = append(near, r.CI)
			}
		}
		cis, err := s.Store.CIsByID(ctx, near[1:])
		if err != nil {
			return err
		}
		for _, c := range cis {
			beneath[c.ID] = c
		}
		if rows, err = s.Assurance.AffectingAny(ctx, near, neighbourLimit); err != nil {
			return err
		}
		items := make([]shared.ID, 0, len(rows))
		for _, r := range rows {
			items = append(items, r.ItemID)
		}
		if len(items) == 0 || a.Pointers.Entries == nil {
			return nil
		}
		if entries, err = a.Pointers.Entries.Entries(ctx, sortedIDs(items)); err != nil {
			return err
		}
		return workservice.WorkflowGate{Workflows: a.Workflows}.NameStatuses(ctx, entries)
	})
	if err != nil {
		return nil, err
	}
	hubs := make([]shared.ID, 0, len(beneath))
	for _, c := range beneath {
		hubs = append(hubs, c.HubID)
	}
	readable, err := a.Pointers.readableHubs(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	visible, err := a.Pointers.visibleEntries(ctx, actor, entries)
	if err != nil {
		return nil, err
	}
	// The rows come latest first; an entry keeps its first place, and the nearest CI beneath it
	// names (the lower identifier on a tie) unless it names this CI itself.
	var order []shared.ID
	seen, names := map[shared.ID]bool{}, map[shared.ID]bool{}
	via := map[shared.ID]shared.ID{}
	for _, r := range rows {
		e, ok := visible[r.ItemID]
		if !ok || e.Archived {
			continue
		}
		if r.CIID != id {
			c, known := beneath[r.CIID]
			if !known || !readable[c.HubID] {
				continue
			}
		}
		if !seen[e.ID] {
			seen[e.ID] = true
			order = append(order, e.ID)
		}
		if r.CIID == id {
			names[e.ID] = true
			continue
		}
		if held := via[e.ID]; held.IsZero() || depth[r.CIID] < depth[held] || depth[r.CIID] == depth[held] && r.CIID < held {
			via[e.ID] = r.CIID
		}
	}
	open, completed := []usecase.Output{}, []usecase.Output{}
	for _, item := range order {
		e := visible[item]
		row := workOutput(e)
		if names[item] {
			row["reason"], row["via"] = workNames, nil
		} else {
			c := beneath[via[item]]
			row["reason"], row["via"] = workBeneath, usecase.Output{"ci_id": c.ID.String(), "name": c.Name, "class": c.Class}
		}
		if !e.Completed {
			open = append(open, row)
		} else if len(completed) < maxCompletedWork {
			completed = append(completed, row)
		}
	}
	return usecase.Output{"ci_id": id.String(), "data": append(open, completed...)}, nil
}

// workOutput is an entry on a CI (schema CiWorkEntry) without its reason.
func workOutput(e workrepo.EntrySummary) usecase.Output {
	var status any
	if e.Status != "" {
		status = usecase.Output{"key": e.Status, "name": stringOrNil(e.StatusName), "category": stringOrNil(e.StatusCategory)}
	}
	return usecase.Output{
		"item_id": e.ID.String(), "key": stringOrNil(e.Key()), "title": e.Title, "entry_type": e.EntryType,
		"collection_id": e.CollectionID.String(), "hub_id": idOrNil(e.HubID), "status": status,
		"completed": e.Completed, "start_at": e.StartAt, "due_at": e.DueAt,
	}
}
