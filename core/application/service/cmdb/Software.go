// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names (UC-AST-13).
const (
	ListSoftwareAliasesName  = "ListSoftwareAliases"
	DefineSoftwareAliasName  = "DefineSoftwareAlias"
	RemoveSoftwareAliasName  = "RemoveSoftwareAlias"
	ListSoftwareProductsName = "ListSoftwareProducts"
)

// The audit codes of the alias table.
const (
	AliasDefinedAction audit.Action = "cmdb.alias_defined"
	AliasRemovedAction audit.Action = "cmdb.alias_removed"
)

const aliasTarget = "cmdb_software_alias"

// AliasOutput is a software alias (schema SoftwareAlias).
func AliasOutput(a domain.SoftwareAlias) usecase.Output {
	return usecase.Output{
		"id": a.ID.String(), "match_publisher": a.MatchPublisher, "match_product": a.MatchProduct,
		"publisher": a.Publisher, "product": a.Product, "updated_at": a.UpdatedAt, "version": a.Version,
	}
}

// ListSoftwareAliases answers the workspace's software aliases.
type ListSoftwareAliases struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListSoftwareAliases) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSoftwareAliasesName, Permission: service.PermissionRead,
		Summary: "Lists the workspace's software aliases: what installations report - a publisher and a product, " +
			"folded - and the publisher and product it is counted as.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: ConfigReadAction, TargetType: aliasTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			s := h.Inventory
			if err := s.authorizeConfig(ctx, actor, false, ConfigReadAction, aliasTarget); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				aliases, err := s.Assurance.Aliases(ctx)
				for _, a := range aliases {
					data = append(data, AliasOutput(a))
				}
				return err
			})
			return usecase.Output{"data": data}, err
		}),
	})
}

// DefineSoftwareAlias maps what installations report to one product.
type DefineSoftwareAlias struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h DefineSoftwareAlias) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineSoftwareAliasName, Permission: service.PermissionStructure,
		Summary: "Defines a software alias, or changes the one of the same match: a product - and a publisher, or " +
			"any when empty - as installations report it, folded (lower case, without a version, an architecture " +
			"or a legal form), and the publisher and product it is counted as.",
		SideEffects: "Writes the alias and an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{
			{Name: "match_publisher", Kind: usecase.KindString, Description: "The publisher reported; empty matches any."},
			{Name: "match_product", Kind: usecase.KindString, Required: true, Description: "The product reported."},
			{Name: "publisher", Kind: usecase.KindString, Description: "The publisher it is counted as."},
			{Name: "product", Kind: usecase.KindString, Required: true, Description: "The product it is counted as."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; zero or absent for a new match."},
		},
		Audit: usecase.AuditDeclaration{Action: AliasDefinedAction, TargetType: aliasTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Inventory
			if err := s.authorizeConfig(ctx, actor, true, AliasDefinedAction, aliasTarget); err != nil {
				return nil, err
			}
			var out usecase.Output
			err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := domain.NewSoftwareAlias(s.IDs.NewID(), actor.TenantID, actor.AccountID, in.String("match_publisher"),
					in.String("match_product"), in.String("publisher"), in.String("product"), s.Clock.Now())
				if err != nil {
					return err
				}
				existing, err := s.Aliases(ctx)
				if err != nil {
					return err
				}
				version := 0
				for _, e := range existing {
					if e.MatchPublisher == a.MatchPublisher && e.MatchProduct == a.MatchProduct {
						a.ID, version = e.ID, e.Version
					}
				}
				if version == 0 && len(existing) >= domain.MaxSoftwareAliases {
					return invalid("/match_product", "cmdb.aliases_too_many")
				}
				if in.Present("expected_version") && in.Int("expected_version") != version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				if err := s.Assurance.PutAlias(ctx, a, version); err != nil {
					return err
				}
				a.Version = version + 1
				out = AliasOutput(a)
				return s.record(ctx, actor, AliasDefinedAction, aliasTarget, a.ID)
			})
			return out, err
		}),
	})
}

// Aliases reads the alias table inside the caller's transaction.
func (s Inventory) Aliases(ctx context.Context) ([]domain.SoftwareAlias, error) {
	return s.Assurance.Aliases(ctx)
}

// RemoveSoftwareAlias removes an alias; the products it named fold by their own names again.
type RemoveSoftwareAlias struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h RemoveSoftwareAlias) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveSoftwareAliasName, Permission: service.PermissionStructure,
		Summary:     "Removes a software alias; what it matched is counted by its own folded names again. No CI changes.",
		SideEffects: "Removes the alias and writes an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{{Name: "alias_id", Kind: usecase.KindID, Required: true, Description: "The alias."}},
		Audit: usecase.AuditDeclaration{Action: AliasRemovedAction, TargetType: aliasTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("alias_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeConfig(ctx, actor, true, AliasRemovedAction, aliasTarget); err != nil {
				return nil, err
			}
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Assurance.RemoveAlias(ctx, id); err != nil {
					return err
				}
				return s.record(ctx, actor, AliasRemovedAction, aliasTarget, id)
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"id": id.String(), "removed": true}, nil
		}),
	})
}

// ListSoftwareProducts answers a hub's software, normalised.
type ListSoftwareProducts struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListSoftwareProducts) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSoftwareProductsName, Permission: service.PermissionRead,
		Summary: "Answers a hub's software CIs grouped by product: the publisher and product an alias names, or the " +
			"folded names otherwise, each with its versions - how many CIs and how many installations (hosts it is " +
			"installed on) each - and whether an alias named it.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: ciTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, false, CIReadAction, ciTarget, hub); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
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
				aliases, err := s.Assurance.Aliases(ctx)
				if err != nil {
					return err
				}
				installs, err := s.Assurance.Installations(ctx, hub)
				if err != nil {
					return err
				}
				for _, p := range domain.Normalise(cis, reg.classes, aliases, installs) {
					versions := make([]usecase.Output, 0, len(p.Versions))
					installations := 0
					for _, v := range p.Versions {
						versions = append(versions, usecase.Output{"version": v.Version, "cis": v.CIs, "installations": v.Installations})
						installations += v.Installations
					}
					ids := make([]string, 0, len(p.CIs))
					for _, id := range p.CIs {
						ids = append(ids, id.String())
					}
					data = append(data, usecase.Output{
						"publisher": p.Publisher, "product": p.Product, "aliased": p.Aliased, "versions": versions,
						"installations": installations, "ci_ids": ids,
					})
				}
				return nil
			})
			return usecase.Output{"data": data}, err
		}),
	})
}
