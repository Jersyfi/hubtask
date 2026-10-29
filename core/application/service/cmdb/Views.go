// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ReadCmdbHealthName = "ReadCmdbHealth"
	ReadCiImpactName   = "ReadCiImpact"
)

// hubCIs reads a hub's whole inventory, page by page.
func (s Inventory) hubCIs(ctx context.Context, hub shared.ID) ([]domain.CI, error) {
	var out []domain.CI
	var after shared.ID
	for len(out) < domain.MaxCIsPerHub {
		page, err := s.Store.ListCIs(ctx, hub, nil, after, 1000)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < 1000 {
			break
		}
		after = page[len(page)-1].ID
	}
	return out, nil
}

// conflictRuns is how many of a hub's latest runs a source's conflict rate is read from.
const conflictRuns = 20

// recentRuns reads a hub's latest runs with their decisions.
func (s Inventory) recentRuns(ctx context.Context, hub shared.ID) ([]domain.Run, error) {
	listed, err := s.Store.Runs(ctx, hub, conflictRuns)
	if err != nil {
		return nil, err
	}
	runs := make([]domain.Run, 0, len(listed))
	for _, r := range listed {
		run, err := s.Store.FindRun(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ReadCmdbHealth judges a hub's inventory.
type ReadCmdbHealth struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCmdbHealth) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCmdbHealthName, Permission: service.PermissionRead,
		Summary: "Judges a hub's inventory of active configuration items: completeness (required and " +
			"recommended attributes and an owner filled), correctness (stale - unconfirmed by any source for " +
			"its class's period -, orphaned - without a relation its class needs -, or in an open duplicate), " +
			"CIs without an owner and reviews overdue, per class and in all, with the CIs found wanting and why; " +
			"the relations a program reported and no longer confirms; each service's score over the CIs it rests " +
			"on; and each source's conflict rate over the hub's latest twenty runs.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, false, CIReadAction, ciTarget, hub); err != nil {
				return nil, err
			}
			var health domain.Health
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := h.Inventory.hub(ctx, hub); err != nil {
					return err
				}
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				cis, err := h.Inventory.hubCIs(ctx, hub)
				if err != nil {
					return err
				}
				relations, err := h.Inventory.Store.HubRelations(ctx, hub)
				if err != nil {
					return err
				}
				open, err := h.Inventory.Store.Duplicates(ctx, hub, 1000)
				if err != nil {
					return err
				}
				dups := map[shared.ID]bool{}
				for _, d := range open {
					for _, c := range d.CIs {
						dups[c] = true
					}
				}
				runs, err := h.Inventory.recentRuns(ctx, hub)
				if err != nil {
					return err
				}
				health = domain.Judge(cis, relations, reg.classes, reg.types, dups, runs, h.Inventory.Clock.Now())
				return nil
			})
			if err != nil {
				return nil, err
			}
			return HealthOutput(health), nil
		}),
	})
}

// HealthOutput is a hub's health (schema CmdbHealth).
func HealthOutput(h domain.Health) usecase.Output {
	classes := make([]usecase.Output, 0, len(h.Classes))
	for _, c := range h.Classes {
		classes = append(classes, usecase.Output{
			"class": c.Class, "total": c.Total, "completeness": c.Completeness, "stale": c.Stale,
			"orphans": c.Orphans, "duplicates": c.Duplicates,
		})
	}
	problems := make([]usecase.Output, 0, len(h.Problems))
	for _, p := range h.Problems {
		problems = append(problems, usecase.Output{
			"ci_id": p.CI.String(), "class": p.Class, "kinds": append([]string{}, p.Kinds...),
			"missing": append([]string{}, p.Missing...),
		})
	}
	services := make([]usecase.Output, 0, len(h.Services))
	for _, sh := range h.Services {
		services = append(services, usecase.Output{
			"ci_id": sh.Service.String(), "class": sh.Class, "name": sh.Name, "cis": sh.CIs,
			"wanting": sh.Wanting, "score": sh.Score,
		})
	}
	sources := make([]usecase.Output, 0, len(h.Sources))
	for _, c := range h.Sources {
		sources = append(sources, usecase.Output{"source": c.Source, "reported": c.Reported, "kept": c.Kept, "conflict_rate": c.Rate})
	}
	return usecase.Output{
		"total": h.Total, "completeness": h.Completeness, "correctness": h.Correctness, "stale": h.Stale,
		"orphans": h.Orphans, "duplicates": h.Duplicates, "without_owner": h.WithoutOwner,
		"review_overdue": h.ReviewOverdue, "classes": classes, "problems": problems,
		"stale_relations": h.StaleRelations, "services": services, "sources": sources,
	}
}

// ReadCiImpact walks what a configuration item's failure reaches, or what it depends on.
type ReadCiImpact struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCiImpact) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCiImpactName, Permission: service.PermissionRead,
		Summary: "Walks the impact of a configuration item along the relations whose type carries impact: " +
			"DOWNSTREAM, everything that suffers when it fails - with the services and offerings among them -; " +
			"UPSTREAM, everything whose failure it suffers. Each CI is named once, at its nearest depth, up " +
			"to eight steps.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "direction", Kind: usecase.KindString, Enum: []string{"DOWNSTREAM", "UPSTREAM"}, Description: "Which way to walk; DOWNSTREAM when not given."},
			{Name: "depth", Kind: usecase.KindInt, Description: "How many steps, 1 to 8; 8 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			ci, err := h.Inventory.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, ci.HubID, false, CIReadAction, ciTarget, id); err != nil {
				return nil, err
			}
			depth := in.Int("depth")
			if depth <= 0 || depth > impactDepth {
				depth = impactDepth
			}
			downstream := in.String("direction") != "UPSTREAM"
			var out usecase.Output
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				sourceSuffers, targetSuffers := reg.types.Suffering()
				reached, err := h.Inventory.Store.Walk(ctx, id, downstream, sourceSuffers, targetSuffers, depth)
				if err != nil {
					return err
				}
				out, err = h.Inventory.impactOutput(ctx, reg, reached)
				return err
			})
			if err != nil {
				return nil, err
			}
			out["ci_id"], out["direction"] = id.String(), map[bool]string{true: "DOWNSTREAM", false: "UPSTREAM"}[downstream]
			return out, nil
		}),
	})
}

func (s Inventory) impactOutput(ctx context.Context, reg registry, reached []repository.Reached) (usecase.Output, error) {
	ids := make([]shared.ID, 0, len(reached))
	for _, r := range reached {
		ids = append(ids, r.CI)
	}
	cis, err := s.Store.CIsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[shared.ID]domain.CI{}
	for _, ci := range cis {
		byID[ci.ID] = ci
	}
	rows, services := []usecase.Output{}, []string{}
	for _, r := range reached {
		ci, ok := byID[r.CI]
		if !ok {
			continue
		}
		rows = append(rows, usecase.Output{
			"ci_id": ci.ID.String(), "class": ci.Class, "name": ci.Name, "stage": string(ci.Stage), "depth": r.Depth,
		})
		if reg.classes.IsA(ci.Class, "service") && ci.Stage.Active() {
			services = append(services, ci.ID.String())
		}
	}
	return usecase.Output{"reached": rows, "services": services}, nil
}
