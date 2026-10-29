// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
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
	ListCisName         = "ListCis"
	ReadCiName          = "ReadCi"
	CreateCiName        = "CreateCi"
	UpdateCiName        = "UpdateCi"
	SetCiComplianceName = "SetCiCompliance"
	RelateCisName       = "RelateCis"
	UnrelateCisName     = "UnrelateCis"
)

// impactDepth bounds an impact walk.
const impactDepth = 8

// ListCis answers a page of a hub's configuration items.
type ListCis struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCis) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCisName, Permission: service.PermissionRead,
		Summary: "Lists a hub's configuration items in a stable order, a page at a time, narrowed to a class " +
			"and its descendants when one is given; merged duplicates are left out unless asked for.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose inventory it is."},
			{Name: "class", Kind: usecase.KindString, Description: "A class key; its descendants are included."},
			{Name: "after", Kind: usecase.KindID, Description: "The cursor: the last identifier of the page before."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 500; 100 when not given."},
			{Name: "include_merged", Kind: usecase.KindBool, Description: "Also the duplicates merged into a survivor."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			after, err := in.ID("after")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, false, CIReadAction, ciTarget, hub); err != nil {
				return nil, err
			}
			limit := limitOf(in, 100, 500)
			rows := []usecase.Output{}
			var next any
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := h.Inventory.hub(ctx, hub); err != nil {
					return err
				}
				var classes []string
				if key := in.String("class"); key != "" {
					reg, err := h.Inventory.registry(ctx)
					if err != nil {
						return err
					}
					if _, ok := reg.classes.Find(key); !ok {
						return invalid("/class", "cmdb.class_unknown", "key", key)
					}
					for _, c := range reg.classes.All() {
						if reg.classes.IsA(c.Key, key) {
							classes = append(classes, c.Key)
						}
					}
				}
				cis, err := h.Inventory.Store.ListCIs(ctx, hub, classes, after, limit)
				if err != nil {
					return err
				}
				for _, ci := range cis {
					if ci.MergedInto.IsZero() || in.Bool("include_merged") {
						rows = append(rows, CIOutput(ci, nil))
					}
				}
				if len(cis) == limit {
					next = cis[len(cis)-1].ID.String()
				}
				return nil
			})
			return usecase.Output{"data": rows, "next": next}, err
		}),
	})
}

// ReadCi reads one configuration item with its relations and the protection need it inherits.
type ReadCi struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCiName, Permission: service.PermissionRead,
		Summary: "Reads one configuration item: its class, attributes with the source that last wrote each, " +
			"lifecycle, compliance fields (owner, location, protection need, review date, quantity), its " +
			"relations both ways and the protection need it inherits from everything that suffers when it " +
			"fails (the maximum principle; a dimension fixed with a reason keeps its own).",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input:       []usecase.Field{{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."}},
		Audit:       usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
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
			var out usecase.Output
			err = h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				effective, err := h.Inventory.inherited(ctx, reg, ci)
				if err != nil {
					return err
				}
				relations, err := h.Inventory.Store.RelationsOf(ctx, id)
				if err != nil {
					return err
				}
				out = CIOutput(ci, &effective)
				rels := make([]usecase.Output, 0, len(relations))
				for _, r := range relations {
					rels = append(rels, RelationOutput(r))
				}
				out["relations"] = rels
				return nil
			})
			return out, err
		}),
	})
}

// inherited answers a CI's protection need with what everything that suffers when it fails
// needs, walked downstream.
func (s Inventory) inherited(ctx context.Context, reg registry, ci domain.CI) (domain.Protection, error) {
	sourceSuffers, targetSuffers := reg.types.Suffering()
	reached, err := s.Store.Walk(ctx, ci.ID, true, sourceSuffers, targetSuffers, impactDepth)
	if err != nil {
		return domain.Protection{}, err
	}
	ids := make([]shared.ID, 0, len(reached))
	for _, r := range reached {
		ids = append(ids, r.CI)
	}
	cis, err := s.Store.CIsByID(ctx, ids)
	if err != nil {
		return domain.Protection{}, err
	}
	effective := ci.Compliance.Protection.Protection
	for _, other := range cis {
		if other.Stage.Active() {
			effective = effective.Max(other.Compliance.Protection.Protection)
		}
	}
	own := ci.Compliance.Protection
	for _, d := range own.Fixed {
		switch d {
		case "c":
			effective.C = own.C
		case "i":
			effective.I = own.I
		case "a":
			effective.A = own.A
		}
	}
	return effective, nil
}

func itemOf(in usecase.Input, class string) domain.Item {
	attrs, _ := in["attributes"].(map[string]any)
	return domain.Item{Ref: "item-1", Class: class, Name: in.String("name"), Attributes: attrs}
}

// CreateCi makes a configuration item by hand, through the reconciliation as source `manual`.
type CreateCi struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h CreateCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreateCiName, Permission: service.PermissionWriteItems,
		Summary: "Makes a configuration item in a hub by hand: a class that is not abstract, a name and typed " +
			"attributes. It goes through the same reconciliation every source does, as source `manual`: when " +
			"the hub already holds a CI its identifiers name, nothing is made and the answer names that CI.",
		SideEffects: "Writes the CI, its identifiers and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose inventory it joins."},
			{Name: "class", Kind: usecase.KindString, Required: true, Description: "The class key."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Its name, up to 200 characters."},
			{Name: "attributes", Kind: usecase.KindObject, Description: "Attribute keys to values of their kind."},
		},
		Audit: usecase.AuditDeclaration{Action: CICreatedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, hub, true, CICreatedAction, ciTarget, hub); err != nil {
				return nil, err
			}
			item := itemOf(in, in.String("class"))
			var ci domain.CI
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				p, err := h.Inventory.newPass(ctx, actor, hub, domain.ManualSource, false)
				if err != nil {
					return err
				}
				p.run.ID = ""
				if class, ok := p.reg.classes.Find(item.Class); !ok || class.Abstract || class.ArchivedAt != nil {
					return invalid("/class", "cmdb.class_not_instantiable", "key", item.Class)
				}
				_, ident, err := p.identify(ctx, item)
				if err != nil {
					return err
				}
				if len(ident.Matches) > 0 {
					return shared.ErrConflict.WithDetail("cmdb.ci_exists").WithParams(map[string]string{"ci_id": ident.Matches[0].String()})
				}
				d, err := p.create(ctx, item, p.reg.classes.Resolve(item.Class), domain.Decision{Ref: item.Ref})
				if err != nil {
					return err
				}
				if d.Outcome != domain.OutcomeCreate {
					return invalid("/", d.Reason)
				}
				ci, err = h.Inventory.Store.FindCI(ctx, d.CI, false)
				return err
			})
			if err != nil {
				return nil, err
			}
			return CIOutput(ci, nil), nil
		}),
	})
}

// UpdateCi changes a configuration item's name, attributes or class by hand.
type UpdateCi struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h UpdateCi) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UpdateCiName, Permission: service.PermissionWriteItems,
		Summary: "Changes a configuration item by hand - its name, attributes (null clears one), or its class " +
			"to a more specific one - through the reconciliation as source `manual`: an attribute a source of " +
			"a lower rank number last wrote is kept, and the answer names what was kept from whom.",
		SideEffects: "Writes the CI, its identifiers and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "name", Kind: usecase.KindString, Description: "A new name."},
			{Name: "class", Kind: usecase.KindString, Description: "A descendant of its class, to refine it."},
			{Name: "attributes", Kind: usecase.KindObject, Description: "Attribute keys to new values; null clears."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: CIChangedAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			placed, err := h.Inventory.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, placed.HubID, true, CIChangedAction, ciTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				p, err := h.Inventory.newPass(ctx, actor, placed.HubID, domain.ManualSource, false)
				if err != nil {
					return err
				}
				p.run.ID = ""
				ci, err := h.Inventory.Store.FindCI(ctx, id, true)
				if err != nil {
					return err
				}
				if in.Present("expected_version") && in.Int("expected_version") != ci.Version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				class := ci.Class
				if c := in.String("class"); c != "" {
					if !p.reg.classes.IsA(c, ci.Class) {
						return invalid("/class", "cmdb.class_not_descendant", "key", c)
					}
					class = c
				}
				d, err := p.update(ctx, ci, itemOf(in, class), domain.Decision{Ref: "item-1"})
				if err != nil {
					return err
				}
				if d.Outcome == domain.OutcomeRefused {
					return invalid("/", d.Reason)
				}
				ci, err = h.Inventory.Store.FindCI(ctx, id, false)
				if err != nil {
					return err
				}
				out = CIOutput(ci, nil)
				out["decision"] = DecisionOutput(d)
				return nil
			})
			return out, err
		}),
	})
}

// SetCiCompliance sets a configuration item's lifecycle and compliance fields.
type SetCiCompliance struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h SetCiCompliance) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetCiComplianceName, Permission: service.PermissionWriteItems,
		Summary: "Sets a configuration item's lifecycle (stage and status) and its compliance fields - owner (a " +
			"person or a group), location (a CI of a location class in the same hub), protection need for " +
			"confidentiality, integrity and availability (NORMAL, HIGH, VERY_HIGH; a dimension fixed against " +
			"inheritance needs a reason), the day of its next review and the quantity of alike objects it " +
			"stands for. A field left out stays; null clears it.",
		SideEffects: "Writes the CI and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "stage", Kind: usecase.KindString, Enum: stageNames(), Description: "Where it is in its life."},
			{Name: "status", Kind: usecase.KindString, Enum: []string{"OK", "PENDING", "BLOCKED"}, Description: "How it stands in its stage."},
			{Name: "owner_account_id", Kind: usecase.KindID, Description: "The person who owns it; null clears."},
			{Name: "owner_group_id", Kind: usecase.KindID, Description: "The group that owns it; null clears."},
			{Name: "location_id", Kind: usecase.KindID, Description: "A CI of a location class in the same hub; null clears."},
			{Name: "protection", Kind: usecase.KindObject, Description: "c, i, a (NORMAL, HIGH, VERY_HIGH or null), fixed (of c, i, a) and reason."},
			{Name: "review_due", Kind: usecase.KindString, Description: "The day of its next review, YYYY-MM-DD; null clears."},
			{Name: "quantity", Kind: usecase.KindInt, Description: "How many alike objects it stands for, at least 1."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: CIChangedAction, TargetType: ciTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			placed, err := h.Inventory.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, placed.HubID, true, CIChangedAction, ciTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				ci, err := h.Inventory.Store.FindCI(ctx, id, true)
				if err != nil {
					return err
				}
				if in.Present("expected_version") && in.Int("expected_version") != ci.Version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				next, fields, err := h.Inventory.compliance(ctx, ci, in)
				if err != nil {
					return err
				}
				next.UpdatedBy, next.UpdatedAt = actor.AccountID, h.Inventory.Clock.Now()
				if err := h.Inventory.Store.UpdateCI(ctx, next, ci.Version); err != nil {
					return err
				}
				next.Version++
				out = CIOutput(next, nil)
				if err := h.Inventory.announce(ctx, actor, next); err != nil {
					return err
				}
				return h.Inventory.recordCI(ctx, actor, CIChangedAction, next, fields)
			})
			return out, err
		}),
	})
}

func stageNames() []string {
	out := make([]string, 0, len(domain.Stages))
	for _, s := range domain.Stages {
		out = append(out, string(s))
	}
	return out
}

func optionalID(in usecase.Input, field string, set func(shared.ID)) error {
	if !in.Present(field) {
		return nil
	}
	id, err := in.ID(field)
	if err != nil {
		return err
	}
	set(id)
	return nil
}

func (s Inventory) compliance(ctx context.Context, ci domain.CI, in usecase.Input) (domain.CI, []string, error) {
	next := ci
	var fields []string
	if in.Present("stage") || in.Present("status") {
		if in.String("stage") != "" {
			next.Stage = domain.Stage(in.String("stage"))
		}
		if in.String("status") != "" {
			next.Status = domain.Status(in.String("status"))
		}
		if err := domain.CheckLifecycle(next.Stage, next.Status); err != nil {
			return ci, nil, err
		}
		fields = append(fields, domain.FieldLifecycle)
	}
	c := &next.Compliance
	for field, set := range map[string]func(shared.ID){
		"owner_account_id": func(id shared.ID) { c.OwnerAccount = id },
		"owner_group_id":   func(id shared.ID) { c.OwnerGroup = id },
		"location_id":      func(id shared.ID) { c.Location = id },
	} {
		if err := optionalID(in, field, set); err != nil {
			return ci, nil, err
		}
	}
	if in.Present("location_id") && !c.Location.IsZero() {
		reg, err := s.registry(ctx)
		if err != nil {
			return ci, nil, err
		}
		location, err := s.Store.FindCI(ctx, c.Location, false)
		if err != nil || location.HubID != ci.HubID || !reg.classes.IsA(location.Class, "location") {
			return ci, nil, invalid("/location_id", "cmdb.location_invalid")
		}
	}
	if m, ok := in["protection"].(map[string]any); ok {
		c.Protection = domain.OwnProtection{
			Protection: domain.Protection{C: domain.Level(text(m, "c")), I: domain.Level(text(m, "i")), A: domain.Level(text(m, "a"))},
			Fixed:      texts(m["fixed"]), Reason: text(m, "reason"),
		}
	}
	if in.Present("review_due") {
		c.ReviewDue = nil
		if day := in.String("review_due"); day != "" {
			parsed, err := time.Parse(time.DateOnly, day)
			if err != nil {
				return ci, nil, invalid("/review_due", "cmdb.review_due_invalid")
			}
			c.ReviewDue = &parsed
		}
	}
	if in.Present("quantity") {
		c.Quantity = in.Int("quantity")
	}
	if err := domain.CheckCompliance(*c); err != nil {
		return ci, nil, err
	}
	for _, f := range []string{"owner_account_id", "owner_group_id", "location_id", "protection", "review_due", "quantity"} {
		if in.Present(f) {
			fields = append(fields, f)
		}
	}
	return next, fields, nil
}

// RelateCis joins two configuration items of a hub by a relation type.
type RelateCis struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h RelateCis) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RelateCisName, Permission: service.PermissionWriteItems,
		Summary: "Joins two configuration items of one hub by a relation type, read from the source (\"the " +
			"shop runs on web1\"); the type's class pairs decide whether it may join them. A relation that " +
			"exists is seen again.",
		SideEffects: "Writes the relation and an audit entry.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "source_ci_id", Kind: usecase.KindID, Required: true, Description: "The CI the relation is read from."},
			{Name: "target_ci_id", Kind: usecase.KindID, Required: true, Description: "The CI it points at."},
			{Name: "type", Kind: usecase.KindString, Required: true, Description: "The relation type's key."},
		},
		Audit: usecase.AuditDeclaration{Action: RelatedAction, TargetType: relationTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			from, err := in.ID("source_ci_id")
			if err != nil {
				return nil, err
			}
			to, err := in.ID("target_ci_id")
			if err != nil {
				return nil, err
			}
			placed, err := h.Inventory.placeOf(ctx, actor, from)
			if err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, placed.HubID, true, RelatedAction, relationTarget, from); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := h.Inventory.registry(ctx)
				if err != nil {
					return err
				}
				t, ok := reg.types.Find(in.String("type"))
				if !ok {
					return invalid("/type", "cmdb.relation_type_unknown", "key", in.String("type"))
				}
				source, err := h.Inventory.Store.FindCI(ctx, from, false)
				if err != nil {
					return err
				}
				target, err := h.Inventory.Store.FindCI(ctx, to, false)
				if err != nil {
					return err
				}
				if err := domain.CheckRelation(t, source, target, reg.classes); err != nil {
					return err
				}
				now := h.Inventory.Clock.Now()
				r := domain.Relation{
					ID: h.Inventory.IDs.NewID(), HubID: source.HubID, Type: t.Key, SourceCI: from, TargetCI: to,
					DataSource: domain.ManualSource, FirstSeen: now, LastSeen: now, CreatedBy: actor.AccountID,
				}
				stored, _, err := h.Inventory.Store.PutRelation(ctx, r)
				if err != nil {
					return err
				}
				if r, err = h.Inventory.Store.FindRelation(ctx, stored); err != nil {
					return err
				}
				out = RelationOutput(r)
				if err := h.Inventory.announceRelations(ctx, actor, r); err != nil {
					return err
				}
				return h.Inventory.record(ctx, actor, RelatedAction, relationTarget, r.ID,
					audit.Change{Field: "type", Classification: audit.Open, To: r.Type})
			})
			return out, err
		}),
	})
}

// UnrelateCis removes a relation.
type UnrelateCis struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h UnrelateCis) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnrelateCisName, Permission: service.PermissionWriteItems,
		Summary: "Removes a relation between two configuration items. A source that reports it again makes it again.",
		SideEffects: "Removes the relation and writes an audit entry.", TokenScope: cisWrite, Destructive: true,
		Input: []usecase.Field{{Name: "relation_id", Kind: usecase.KindID, Required: true, Description: "The relation."}},
		Audit: usecase.AuditDeclaration{Action: UnrelatedAction, TargetType: relationTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("relation_id")
			if err != nil {
				return nil, err
			}
			var r domain.Relation
			if err := h.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				r, err = h.Inventory.Store.FindRelation(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			if err := h.Inventory.authorizeHub(ctx, actor, r.HubID, true, UnrelatedAction, relationTarget, id); err != nil {
				return nil, err
			}
			err = h.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := h.Inventory.Store.DeleteRelation(ctx, id); err != nil {
					return err
				}
				if err := h.Inventory.announceUnrelated(ctx, actor, r); err != nil {
					return err
				}
				return h.Inventory.record(ctx, actor, UnrelatedAction, relationTarget, id,
					audit.Change{Field: "type", Classification: audit.Open, To: r.Type})
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"id": id.String(), "removed": true}, nil
		}),
	})
}
