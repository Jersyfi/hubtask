// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"slices"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ReconcileCisName       = "ReconcileCis"
	ImportCiFileName       = "ImportCiFile"
	ListCiRunsName         = "ListCiRuns"
	ReadCiRunName          = "ReadCiRun"
	ListCiHoldsName        = "ListCiHolds"
	DismissCiHoldName      = "DismissCiHold"
	ListCiDuplicatesName   = "ListCiDuplicates"
	ResolveCiDuplicateName = "ResolveCiDuplicate"
)

func payloadOf(in usecase.Input) (domain.Payload, error) {
	var p domain.Payload
	for _, m := range list(in, "items") {
		attrs, _ := m["attributes"].(map[string]any)
		p.Items = append(p.Items, domain.Item{
			Ref: text(m, "ref"), Class: text(m, "class"), Name: text(m, "name"), NativeKey: text(m, "native_key"), Attributes: attrs,
		})
	}
	for i, m := range list(in, "relations") {
		r := domain.RelationItem{Type: text(m, "type"), From: text(m, "from"), To: text(m, "to")}
		for field, set := range map[string]*shared.ID{"from_id": &r.FromID, "to_id": &r.ToID} {
			if raw := text(m, field); raw != "" {
				id, err := shared.ParseID(raw)
				if err != nil {
					return p, invalid("/relations/"+itoa(i)+"/"+field, "shared.id_malformed")
				}
				*set = id
			}
		}
		p.Relations = append(p.Relations, r)
	}
	return p, nil
}

// RunOutput is a run as every door returns it (schema CiRun).
func RunOutput(run domain.Run, dry bool) usecase.Output {
	decisions := make([]usecase.Output, 0, len(run.Decisions))
	for _, d := range run.Decisions {
		decisions = append(decisions, DecisionOutput(d))
	}
	var id any
	if !dry {
		id = run.ID.String()
	}
	return usecase.Output{
		"id": id, "hub_id": run.HubID.String(), "source": run.Source, "actor_id": run.ActorID.String(),
		"started_at": run.StartedAt, "dry_run": dry, "counts": CountsOutput(run.Counts), "decisions": decisions,
	}
}

func (s Inventory) run(ctx context.Context, actor appshared.ActorContext, hub shared.ID, source string, payload domain.Payload, dry bool) (usecase.Output, error) {
	if err := s.authorizeHub(ctx, actor, hub, true, ReconciledAction, runTarget, hub); err != nil {
		return nil, err
	}
	if source == domain.ManualSource {
		return nil, invalid("/source", "cmdb.source_manual_is_people")
	}
	var out usecase.Output
	write := s.UnitOfWork.Within
	if dry {
		write = s.UnitOfWork.WithinReadOnly
	}
	err := write(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		run, err := s.reconcile(ctx, actor, hub, source, payload, dry)
		if err != nil {
			return err
		}
		out = RunOutput(run, dry)
		if dry {
			return nil
		}
		c := run.Counts
		return s.record(ctx, actor, ReconciledAction, runTarget, run.ID,
			audit.Change{Field: "source", Classification: audit.Open, To: source},
			audit.Change{Field: "created", Classification: audit.Open, To: c.Created},
			audit.Change{Field: "updated", Classification: audit.Open, To: c.Updated},
			audit.Change{Field: "held", Classification: audit.Open, To: c.Held},
			audit.Change{Field: "duplicates", Classification: audit.Open, To: c.Duplicates})
	})
	return out, err
}

var dryRunField = usecase.Field{Name: "dry_run", Kind: usecase.KindBool, Description: "Answer what would happen and write nothing."}

// ReconcileCis runs a source's payload through the reconciliation.
type ReconcileCis struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReconcileCis) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReconcileCisName, Permission: service.PermissionWriteItems,
		Summary: "Runs a source's payload of configuration items and relations through the one reconciliation " +
			"every source writes through. Each item is identified - by the source's own key first, then by its " +
			"class's identifier rules in order - and created, updated under the sources' precedence, held for " +
			"review when it cannot be identified or the source may not create it, or reported as matching " +
			"several CIs, which opens a duplicate to merge. Every decision names the rule that matched and " +
			"the attributes taken or kept. A dry run answers the same and writes nothing.",
		SideEffects: "Writes CIs, identifiers, relations, the run, its holds and duplicates, and audit entries; nothing on a dry run.",
		TokenScope:  cisWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose inventory it is."},
			{Name: "source", Kind: usecase.KindString, Required: true, Description: "The source's key; not `manual`."},
			{Name: "items", Kind: usecase.KindList, Required: true, Description: "Up to 2000 items: ref, class, name, native_key, attributes."},
			{Name: "relations", Kind: usecase.KindList, Description: "Up to 5000 relations: type, from and to by an item's ref, or from_id and to_id by a CI."},
			dryRunField,
		},
		Audit: usecase.AuditDeclaration{Action: ReconciledAction, TargetType: runTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			payload, err := payloadOf(in)
			if err != nil {
				return nil, err
			}
			return h.Inventory.run(ctx, actor, hub, in.String("source"), payload, in.Bool("dry_run"))
		}),
	})
}

// ImportCiFile reads a file of configuration items and runs it through the reconciliation.
type ImportCiFile struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ImportCiFile) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ImportCiFileName, Permission: service.PermissionWriteItems,
		Summary: "Imports a file of configuration items as a source: a CSV table or a JSON array read by a " +
			"mapping (the source's saved one, or one given), or a CycloneDX or SPDX bill of materials whose " +
			"components become software CIs identified by package URL, CPE or name and version, with their " +
			"dependencies. It runs through the reconciliation like every source; a dry run answers what would " +
			"happen, row by row, and writes nothing. Rows without a name are reported by number.",
		SideEffects: "Writes what the reconciliation writes; nothing on a dry run.",
		TokenScope:  cisWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose inventory it is."},
			{Name: "source", Kind: usecase.KindString, Required: true, Description: "The source's key; not `manual`."},
			{Name: "format", Kind: usecase.KindString, Required: true, Enum: []string{"CSV", "JSON", "CYCLONEDX", "SPDX"}, Description: "The file's shape."},
			{Name: "content", Kind: usecase.KindString, Required: true, Description: "The file's text, at most 5 MiB."},
			{Name: "mapping", Kind: usecase.KindObject, Description: "How a CSV or JSON file is read, instead of the source's saved mapping."},
			dryRunField,
		},
		Audit: usecase.AuditDeclaration{Action: ReconciledAction, TargetType: runTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			mapping := mappingOf(in["mapping"])
			key := in.String("source")
			if mapping == nil {
				if err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
					sources, err := h.Inventory.Store.Sources(ctx)
					for _, s := range sources {
						if s.Key == key {
							mapping = s.Mapping
						}
					}
					return err
				}); err != nil {
					return nil, err
				}
			}
			raw, _ := in["content"].(string)
			payload, skipped, err := ParseFile(domain.FileFormat(in.String("format")), []byte(raw), mapping)
			if err != nil {
				return nil, err
			}
			out, err := h.Inventory.run(ctx, actor, hub, key, payload, in.Bool("dry_run"))
			if err != nil {
				return nil, err
			}
			out["skipped_rows"] = append([]int{}, skipped...)
			return out, nil
		}),
	})
}

// ListCiRuns answers a hub's last reconciliation runs.
type ListCiRuns struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiRuns) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiRunsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's last reconciliation runs, the newest first, each with its source, who ran it and " +
			"its figures (created, updated, unchanged, held, duplicates, refused, relations).",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 200; 50 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: runTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, false, CIReadAction, runTarget, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				runs, err := h.Inventory.Store.Runs(ctx, hub, limitOf(in, 50, 200))
				for _, r := range runs {
					out := RunOutput(r, false)
					delete(out, "decisions")
					rows = append(rows, out)
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	})
}

// ReadCiRun reads one run with every decision it made.
type ReadCiRun struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCiRun) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCiRunName, Permission: service.PermissionRead,
		Summary: "Reads one reconciliation run with its explanation: for every item, the outcome, the CI, the " +
			"identifier rule that matched, the attributes taken and those kept and by which source, the " +
			"candidates of a duplicate and the reason of a hold or refusal - by attribute name, never value.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input:       []usecase.Field{{Name: "run_id", Kind: usecase.KindID, Required: true, Description: "The run."}},
		Audit:       usecase.AuditDeclaration{Action: CIReadAction, TargetType: runTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("run_id")
			if err != nil {
				return nil, err
			}
			var run domain.Run
			if err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				run, err = h.Inventory.Store.FindRun(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, run.HubID, false, CIReadAction, runTarget, id); err != nil {
				return nil, err
			}
			return RunOutput(run, false), nil
		}),
	})
}

// HoldOutput is a held item (schema CiHold).
func HoldOutput(h domain.Hold) usecase.Output {
	attrs := h.Item.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return usecase.Output{
		"id": h.ID.String(), "hub_id": h.HubID.String(), "run_id": h.Run.String(), "source": h.Source, "reason": h.Reason,
		"item": usecase.Output{"ref": h.Item.Ref, "class": h.Item.Class, "name": h.Item.Name, "native_key": h.Item.NativeKey, "attributes": attrs},
		"created_at": h.CreatedAt,
	}
}

// ListCiHolds answers the items a hub's reconciliation holds for review.
type ListCiHolds struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiHolds) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiHoldsName, Permission: service.PermissionRead,
		Summary: "Lists the items the reconciliation held for review in a hub instead of making a CI nobody " +
			"could find again: without a usable identifier, from a source that may not create, or of a class " +
			"the source may not write. Each is resolved by a person making the CI or dismissing it.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 500; 100 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: runTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, false, CIReadAction, runTarget, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				holds, err := h.Inventory.Store.Holds(ctx, hub, limitOf(in, 100, 500))
				for _, hold := range holds {
					rows = append(rows, HoldOutput(hold))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	})
}

// DismissCiHold closes a held item.
type DismissCiHold struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h DismissCiHold) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DismissCiHoldName, Permission: service.PermissionWriteItems,
		Summary: "Closes an item the reconciliation held - after a person made its CI by hand, or because it " +
			"is not one. The hold stays on record with who closed it.",
		SideEffects: "Closes the hold and writes an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{{Name: "hold_id", Kind: usecase.KindID, Required: true, Description: "The hold."}},
		Audit: usecase.AuditDeclaration{Action: HoldDismissedAction, TargetType: runTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hold_id")
			if err != nil {
				return nil, err
			}
			var hold domain.Hold
			if err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				hold, err = h.Inventory.Store.FindHold(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hold.HubID, true, HoldDismissedAction, runTarget, id); err != nil {
				return nil, err
			}
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := h.Inventory.Store.ResolveHold(ctx, id, h.Inventory.Clock.Now(), actor.AccountID); err != nil {
					return err
				}
				return h.Inventory.record(ctx, actor, HoldDismissedAction, runTarget, id,
					audit.Change{Field: "reason", Classification: audit.Open, To: hold.Reason})
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"id": id.String(), "dismissed": true}, nil
		}),
	})
}

// DuplicateOutput is an open duplicate (schema CiDuplicate).
func DuplicateOutput(d domain.Duplicate) usecase.Output {
	ids := make([]string, 0, len(d.CIs))
	for _, c := range d.CIs {
		ids = append(ids, c.String())
	}
	return usecase.Output{"id": d.ID.String(), "hub_id": d.HubID.String(), "ci_ids": ids, "rule": d.Rule, "created_at": d.CreatedAt}
}

// ListCiDuplicates answers a hub's open duplicates.
type ListCiDuplicates struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiDuplicates) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiDuplicatesName, Permission: service.PermissionRead,
		Summary: "Lists a hub's open duplicates: sets of configuration items one reported item matched by the " +
			"same identifier rule, waiting for a person to merge them or say they are distinct.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 500; 100 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, false, CIReadAction, ciTarget, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := h.Inventory.Store.Duplicates(ctx, hub, limitOf(in, 100, 500))
				for _, d := range list {
					rows = append(rows, DuplicateOutput(d))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	})
}

// ResolveCiDuplicate merges a duplicate into its survivor, or closes it as distinct.
type ResolveCiDuplicate struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ResolveCiDuplicate) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ResolveCiDuplicateName, Permission: service.PermissionWriteItems,
		Summary: "Resolves an open duplicate. With a survivor, every other CI of it is merged in: the survivor " +
			"keeps its identity and values and takes the attributes it lacks, the others' source keys and " +
			"relations; each other CI is retired as merged into it and kept, never deleted. Without one, the " +
			"CIs are recorded as distinct.",
		SideEffects: "Writes the CIs, moves identifiers and relations, closes the duplicate and writes audit entries.",
		TokenScope:  cisWrite,
		Input: []usecase.Field{
			{Name: "duplicate_id", Kind: usecase.KindID, Required: true, Description: "The duplicate."},
			{Name: "survivor_id", Kind: usecase.KindID, Description: "The CI that stays; absent says the CIs are distinct."},
		},
		Audit: usecase.AuditDeclaration{Action: DuplicateResolvedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("duplicate_id")
			if err != nil {
				return nil, err
			}
			survivor, err := in.ID("survivor_id")
			if err != nil {
				return nil, err
			}
			var dup domain.Duplicate
			if err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				dup, err = h.Inventory.Store.FindDuplicate(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, dup.HubID, true, DuplicateResolvedAction, ciTarget, id); err != nil {
				return nil, err
			}
			if !survivor.IsZero() && !slices.Contains(dup.CIs, survivor) {
				return nil, invalid("/survivor_id", "cmdb.survivor_not_in_duplicate")
			}
			var out usecase.Output
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				now := h.Inventory.Clock.Now()
				if err := h.Inventory.Store.ResolveDuplicate(ctx, id, now, actor.AccountID, survivor); err != nil {
					return err
				}
				if survivor.IsZero() {
					out = usecase.Output{"id": id.String(), "merged": false}
					return h.Inventory.record(ctx, actor, DuplicateResolvedAction, ciTarget, id,
						audit.Change{Field: "outcome", Classification: audit.Open, To: "distinct"})
				}
				kept, err := h.Inventory.merge(ctx, actor, survivor, sortedIDs(dup.CIs), now)
				if err != nil {
					return err
				}
				out = CIOutput(kept, nil)
				out["merged"] = true
				return h.Inventory.record(ctx, actor, DuplicateResolvedAction, ciTarget, id,
					audit.Change{Field: "outcome", Classification: audit.Open, To: "merged"},
					audit.Change{Field: "survivor", Classification: audit.Open, To: survivor.String()})
			})
			return out, err
		}),
	})
}

func (s Inventory) merge(ctx context.Context, actor appshared.ActorContext, survivorID shared.ID, cis []shared.ID, now time.Time) (domain.CI, error) {
	survivor, err := s.Store.FindCI(ctx, survivorID, true)
	if err != nil {
		return domain.CI{}, err
	}
	reg, err := s.registry(ctx)
	if err != nil {
		return domain.CI{}, err
	}
	expected := survivor.Version
	for _, other := range cis {
		if other == survivorID {
			continue
		}
		dup, err := s.Store.FindCI(ctx, other, true)
		if err != nil {
			return domain.CI{}, err
		}
		if dup.HubID != survivor.HubID || !dup.MergedInto.IsZero() {
			continue
		}
		var merged domain.CI
		survivor, merged = domain.Merge(survivor, dup, now, actor.AccountID)
		// The duplicate's relations move or are dropped: a device is told of every one as gone,
		// and of the survivor's as they stand once all are merged.
		moving, err := s.Store.RelationsOf(ctx, other)
		if err != nil {
			return domain.CI{}, err
		}
		if err := s.announceUnrelated(ctx, actor, moving...); err != nil {
			return domain.CI{}, err
		}
		if err := s.Store.MergeInto(ctx, survivorID, other); err != nil {
			return domain.CI{}, err
		}
		if err := s.Store.UpdateCI(ctx, merged, dup.Version); err != nil {
			return domain.CI{}, err
		}
		merged.Version = dup.Version + 1
		if err := s.announce(ctx, actor, merged); err != nil {
			return domain.CI{}, err
		}
		if err := s.recordCI(ctx, actor, CIMergedAction, merged, []string{"merged_into"}); err != nil {
			return domain.CI{}, err
		}
	}
	if err := s.Store.UpdateCI(ctx, survivor, expected); err != nil {
		return domain.CI{}, err
	}
	if err := s.Store.PutIdentifiers(ctx, survivor, survivor.Identifiers(reg.classes.Resolve(survivor.Class))); err != nil {
		return domain.CI{}, err
	}
	survivor.Version = expected + 1
	kept, err := s.Store.RelationsOf(ctx, survivorID)
	if err != nil {
		return domain.CI{}, err
	}
	if err := s.announce(ctx, actor, survivor); err != nil {
		return domain.CI{}, err
	}
	if err := s.announceRelations(ctx, actor, kept...); err != nil {
		return domain.CI{}, err
	}
	return survivor, s.recordCI(ctx, actor, CIChangedAction, survivor, []string{"attributes"})
}
