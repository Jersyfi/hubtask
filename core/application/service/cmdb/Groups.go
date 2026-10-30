// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names (UC-AST-11).
const (
	ListCiGroupsName       = "ListCiGroups"
	DefineCiGroupName      = "DefineCiGroup"
	ListCiGroupMembersName = "ListCiGroupMembers"
	ReadServiceMapName     = "ReadServiceMap"
)

// GroupDefinedAction is the audit code of a group defined or changed.
const GroupDefinedAction audit.Action = "cmdb.group_defined"

const groupTarget = "cmdb_ci_group"

// GroupEntity is the change log's entity of a CI group, filed under its hub.
const GroupEntity = "cmdb_ci_group"

// GroupOutput is a group (schema CiGroup).
func GroupOutput(g domain.Group, members int) usecase.Output {
	stages := make([]string, 0, len(g.Filter.Stages))
	for _, s := range g.Filter.Stages {
		stages = append(stages, string(s))
	}
	attrs := map[string]any{}
	for k, v := range g.Filter.Attributes {
		attrs[k] = v
	}
	out := usecase.Output{
		"id": g.ID.String(), "hub_id": g.HubID.String(), "key": g.Key, "name": g.Name,
		"filter": usecase.Output{
			"classes": append([]string{}, g.Filter.Classes...), "stages": stages, "name_has": g.Filter.NameHas,
			"attributes": attrs,
		},
		"service_id": idOrNil(g.ServiceID), "updated_at": g.UpdatedAt, "version": g.Version,
	}
	if members >= 0 {
		out["members"] = members
	}
	return out
}

func filterOf(raw any) domain.Filter {
	m, _ := raw.(map[string]any)
	f := domain.Filter{Classes: texts(m["classes"]), NameHas: text(m, "name_has"), Attributes: textMap(m["attributes"])}
	for _, s := range texts(m["stages"]) {
		f.Stages = append(f.Stages, domain.Stage(s))
	}
	return f
}

// ListCiGroups answers a hub's CI groups with how many CIs each holds now.
type ListCiGroups struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiGroups) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiGroupsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's CI groups - sets defined by a filter on class, stage, name and attribute values, " +
			"never by listing CIs - with how many CIs each holds now and the service each stands for, if any.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: groupTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, false, CIReadAction, groupTarget, hub); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				groups, err := s.Assurance.Groups(ctx, hub)
				if err != nil || len(groups) == 0 {
					return err
				}
				reg, err := s.registry(ctx)
				if err != nil {
					return err
				}
				cis, err := s.hubCIs(ctx, hub)
				if err != nil {
					return err
				}
				for _, g := range groups {
					data = append(data, GroupOutput(g, len(g.Members(cis, reg.classes))))
				}
				return nil
			})
			return usecase.Output{"data": data}, err
		}),
	})
}

// DefineCiGroup defines or changes a hub's CI group by its key.
type DefineCiGroup struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h DefineCiGroup) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineCiGroupName, Permission: service.PermissionWriteItems,
		Summary: "Defines a CI group of a hub under a key, or changes it: a name and a filter - classes (with those " +
			"below them), stages, a text the name holds and attribute values to equal, without case; at least one. " +
			"When it names a service CI of the hub, the members are what that service rests on beside its relations.",
		SideEffects: "Writes the group, an audit entry and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The group's key, lower case."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What it is called."},
			{Name: "filter", Kind: usecase.KindObject, Required: true, Description: "classes, stages, name_has and attributes (key to value)."},
			{Name: "service_id", Kind: usecase.KindID, Description: "A service CI of the hub the group stands for; null clears."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; zero or absent for a new key."},
		},
		Audit: usecase.AuditDeclaration{Action: GroupDefinedAction, TargetType: groupTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, true, GroupDefinedAction, groupTarget, hub); err != nil {
				return nil, err
			}
			var serviceID shared.ID
			if err := optionalID(in, "service_id", func(id shared.ID) { serviceID = id }); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				reg, err := s.registry(ctx)
				if err != nil {
					return err
				}
				if !serviceID.IsZero() {
					svc, err := s.Store.FindCI(ctx, serviceID, false)
					if err != nil || svc.HubID != hub || !reg.classes.IsA(svc.Class, "service") {
						return invalid("/service_id", "cmdb.group_service_invalid")
					}
				}
				existing, err := s.Assurance.Groups(ctx, hub)
				if err != nil {
					return err
				}
				id, version := s.IDs.NewID(), 0
				for _, g := range existing {
					if g.Key == in.String("key") {
						id, version = g.ID, g.Version
					}
				}
				if version == 0 && len(existing) >= domain.MaxGroupsPerHub {
					return invalid("/key", "cmdb.groups_too_many")
				}
				if in.Present("expected_version") && in.Int("expected_version") != version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				g, err := domain.NewGroup(id, actor.TenantID, hub, actor.AccountID, domain.GroupInput{
					Key: in.String("key"), Name: in.String("name"), Filter: filterOf(in["filter"]), ServiceID: serviceID,
				}, reg.classes, s.Clock.Now())
				if err != nil {
					return err
				}
				if err := s.Assurance.PutGroup(ctx, g, version); err != nil {
					return err
				}
				g.Version = version + 1
				cis, err := s.hubCIs(ctx, hub)
				if err != nil {
					return err
				}
				out = GroupOutput(g, len(g.Members(cis, reg.classes)))
				if s.Changes != nil {
					if err := s.Changes.Record(ctx, changelog.Change{
						TenantID: actor.TenantID, Entity: GroupEntity, EntityID: g.ID, Op: changelog.Upsert,
						ContainerID: hub, ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: GroupOutput(g, -1),
					}); err != nil {
						return err
					}
				}
				return s.record(ctx, actor, GroupDefinedAction, groupTarget, g.ID,
					audit.Change{Field: "key", Classification: audit.Open, To: g.Key},
					audit.Change{Field: "name", Classification: audit.Sensitive, To: g.Name})
			})
			return out, err
		}),
	})
}

// ListCiGroupMembers answers the CIs a group holds now.
type ListCiGroupMembers struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiGroupMembers) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiGroupMembersName, Permission: service.PermissionRead,
		Summary:     "Answers the CIs a group's filter holds now, in the hub's inventory, merged ones never.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "group_id", Kind: usecase.KindID, Required: true, Description: "The group."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: groupTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("group_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			var g domain.Group
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				g, err = s.Assurance.FindGroup(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, g.HubID, false, CIReadAction, groupTarget, id); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				reg, err := s.registry(ctx)
				if err != nil {
					return err
				}
				cis, err := s.hubCIs(ctx, g.HubID)
				if err != nil {
					return err
				}
				for _, ci := range g.Members(cis, reg.classes) {
					data = append(data, usecase.Output{"ci_id": ci.ID.String(), "class": ci.Class, "name": ci.Name, "stage": string(ci.Stage)})
				}
				return nil
			})
			return usecase.Output{"group": GroupOutput(g, len(data)), "data": data}, err
		}),
	})
}

// mapLayer places a class in the service map: offerings on top, then services, application
// services, software and data, virtual and cloud resources, and what they all stand on.
func mapLayer(classes domain.Classes, class string) int {
	switch {
	case classes.IsA(class, "business_offering"), classes.IsA(class, "technical_offering"):
		return 0
	case classes.IsA(class, "application_service"):
		return 2
	case classes.IsA(class, "service"):
		return 1
	case classes.IsA(class, "software"), classes.IsA(class, "database_instance"), classes.IsA(class, "information_object"):
		return 3
	case classes.IsA(class, "virtual_machine"), classes.IsA(class, "cloud_resource"),
		classes.IsA(class, "container_cluster"), classes.IsA(class, "virtualization_cluster"):
		return 4
	default:
		return 5
	}
}

const maxMapNodes = 2000

// ReadServiceMap answers a hub's service map: its active services and offerings and everything
// they rest on, in layers, with the relations between them.
type ReadServiceMap struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadServiceMap) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadServiceMapName, Permission: service.PermissionRead,
		Summary: "Answers a hub's service map: every active service and offering, everything whose failure they " +
			"suffer up to eight steps and the members of the groups standing for them, each placed in a layer - " +
			"offerings, services, application services, software and data, virtual and cloud resources, what they " +
			"stand on - with the relations carrying impact between them and each group's membership.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "service_id", Kind: usecase.KindID, Description: "Only this service or offering and what it rests on."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			var only shared.ID
			if err := optionalID(in, "service_id", func(id shared.ID) { only = id }); err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, false, CIReadAction, ciTarget, hub); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				out, err = s.serviceMap(ctx, hub, only)
				return err
			})
			return out, err
		}),
	})
}

func (s Inventory) serviceMap(ctx context.Context, hub, only shared.ID) (usecase.Output, error) {
	reg, err := s.registry(ctx)
	if err != nil {
		return nil, err
	}
	cis, err := s.hubCIs(ctx, hub)
	if err != nil {
		return nil, err
	}
	byID := map[shared.ID]domain.CI{}
	for _, ci := range cis {
		byID[ci.ID] = ci
	}
	groups, err := s.Assurance.Groups(ctx, hub)
	if err != nil {
		return nil, err
	}
	sourceSuffers, targetSuffers := reg.types.Suffering()
	in := map[shared.ID]bool{}
	var order []shared.ID
	add := func(id shared.ID) {
		if !in[id] && len(order) < maxMapNodes {
			in[id] = true
			order = append(order, id)
		}
	}
	type membership struct {
		service shared.ID
		member  shared.ID
		key     string
	}
	var members []membership
	for _, ci := range cis {
		if !reg.classes.IsA(ci.Class, "service") || !ci.Stage.Active() || ci.Status == domain.StatusMerged {
			continue
		}
		if !only.IsZero() && ci.ID != only {
			continue
		}
		add(ci.ID)
		var reached []repository.Reached
		if reached, err = s.Store.Walk(ctx, ci.ID, false, sourceSuffers, targetSuffers, impactDepth); err != nil {
			return nil, err
		}
		for _, r := range reached {
			add(r.CI)
		}
		for _, g := range groups {
			if g.ServiceID != ci.ID {
				continue
			}
			for _, m := range g.Members(cis, reg.classes) {
				add(m.ID)
				members = append(members, membership{ci.ID, m.ID, g.Key})
			}
		}
	}
	nodes := make([]usecase.Output, 0, len(order))
	for _, id := range order {
		ci, ok := byID[id]
		if !ok {
			continue
		}
		nodes = append(nodes, usecase.Output{
			"ci_id": id.String(), "class": ci.Class, "name": ci.Name, "stage": string(ci.Stage),
			"layer": mapLayer(reg.classes, ci.Class),
		})
	}
	relations, err := s.Store.HubRelations(ctx, hub)
	if err != nil {
		return nil, err
	}
	edges := []usecase.Output{}
	for _, r := range relations {
		t, ok := reg.types.Find(r.Type)
		if !ok || t.Impact == domain.ImpactNone || !in[r.SourceCI] || !in[r.TargetCI] {
			continue
		}
		edges = append(edges, usecase.Output{"source": r.SourceCI.String(), "target": r.TargetCI.String(), "type": r.Type})
	}
	slices.SortFunc(members, func(a, b membership) int { return compareIDs(a.member, b.member) })
	for _, m := range members {
		edges = append(edges, usecase.Output{"source": m.service.String(), "target": m.member.String(), "type": "group:" + m.key})
	}
	return usecase.Output{"hub_id": hub.String(), "nodes": nodes, "edges": edges, "truncated": len(order) >= maxMapNodes}, nil
}

func compareIDs(a, b shared.ID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
