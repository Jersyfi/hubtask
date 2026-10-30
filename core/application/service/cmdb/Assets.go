// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names (UC-AST-12).
const (
	ListCiAssetsName    = "ListCiAssets"
	ReadCiAssetName     = "ReadCiAsset"
	RegisterCiAssetName = "RegisterCiAsset"
	MoveCiAssetName     = "MoveCiAsset"
)

// The audit codes of the asset register. A cost, a supplier and a certificate are named by the
// field only (rule 10).
const (
	AssetRegisteredAction audit.Action = "cmdb.asset_registered"
	AssetMovedAction      audit.Action = "cmdb.asset_moved"
)

const assetTarget = "cmdb_asset"

// AssetEntity is the change log's entity of a CI's asset record, filed under its hub.
const AssetEntity = "cmdb_asset"

// AssetOutput is an asset record (schema CiAsset).
func AssetOutput(a domain.Asset, now time.Time) usecase.Output {
	var disposal any
	if a.Disposal != nil {
		disposal = usecase.Output{"method": a.Disposal.Method, "certificate": a.Disposal.Certificate, "by": idOrNil(a.Disposal.By), "at": a.Disposal.At}
	}
	return usecase.Output{
		"ci_id": a.CIID.String(), "hub_id": a.HubID.String(), "asset_tag": a.AssetTag, "cost_minor": a.CostMinor,
		"currency": a.Currency, "purchased_on": dayOrNil(a.PurchasedOn), "supplier": a.Supplier,
		"warranty_until": dayOrNil(a.WarrantyUntil), "warranty_ended": a.WarrantyEnded(now),
		"contract_id": idOrNil(a.ContractID), "stockroom_id": idOrNil(a.StockroomID), "assigned_to": idOrNil(a.AssignedTo),
		"request_id": idOrNil(a.RequestID), "state": string(a.State), "disposal": disposal,
		"updated_at": a.UpdatedAt, "version": a.Version,
	}
}

func assetStateNames() []string {
	out := make([]string, 0, len(domain.AssetStates))
	for _, s := range domain.AssetStates {
		out = append(out, string(s))
	}
	return out
}

func (s Inventory) announceAsset(ctx context.Context, actor appshared.ActorContext, a domain.Asset) error {
	if s.Changes == nil {
		return nil
	}
	return s.Changes.Record(ctx, changelog.Change{
		TenantID: actor.TenantID, Entity: AssetEntity, EntityID: a.CIID, Op: changelog.Upsert,
		ContainerID: a.HubID, ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: AssetOutput(a, s.Clock.Now()),
	})
}

// ListCiAssets answers a hub's asset register.
type ListCiAssets struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ListCiAssets) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiAssetsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's asset register in CI order after a cursor: each CI's tag, cost, purchase, supplier, " +
			"warranty (and whether it ended), contract, stockroom, holder and state, with the CI's name and class; " +
			"narrowed to a state when given.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "state", Kind: usecase.KindString, Enum: assetStateNames(), Description: "Only assets in this state."},
			{Name: "after", Kind: usecase.KindID, Description: "The last CI of the previous page."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, 1 to 500; 100 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: assetTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			var after shared.ID
			if err := optionalID(in, "after", func(id shared.ID) { after = id }); err != nil {
				return nil, err
			}
			s := h.Inventory
			if err := s.authorizeHub(ctx, actor, hub, false, CIReadAction, assetTarget, hub); err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.hub(ctx, hub); err != nil {
					return err
				}
				assets, err := s.Assurance.Assets(ctx, hub, after, limitOf(in, 100, 500))
				if err != nil {
					return err
				}
				ids := make([]shared.ID, 0, len(assets))
				for _, a := range assets {
					ids = append(ids, a.CIID)
				}
				cis, err := s.Store.CIsByID(ctx, ids)
				if err != nil {
					return err
				}
				byID := map[shared.ID]domain.CI{}
				for _, ci := range cis {
					byID[ci.ID] = ci
				}
				now := s.Clock.Now()
				for _, a := range assets {
					if state := in.String("state"); state != "" && string(a.State) != state {
						continue
					}
					row := AssetOutput(a, now)
					row["name"], row["class"] = byID[a.CIID].Name, byID[a.CIID].Class
					data = append(data, row)
				}
				return nil
			})
			return usecase.Output{"data": data}, err
		}),
	})
}

// ReadCiAsset answers a CI's asset record.
type ReadCiAsset struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h ReadCiAsset) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadCiAssetName, Permission: service.PermissionRead,
		Summary:     "Reads a configuration item's asset record with its disposal, if any; not found when it has none.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."}},
		Audit: usecase.AuditDeclaration{Action: CIReadAction, TargetType: assetTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			ci, err := s.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, ci.HubID, false, CIReadAction, assetTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := s.Assurance.FindAsset(ctx, id)
				if err == nil {
					out = AssetOutput(a, s.Clock.Now())
				}
				return err
			})
			return out, err
		}),
	})
}

// assetRef checks a CI an asset names: in the same hub and of the class asked.
func (s Inventory) assetRef(ctx context.Context, reg registry, hub, id shared.ID, class, path string) error {
	if id.IsZero() {
		return nil
	}
	ci, err := s.Store.FindCI(ctx, id, false)
	if err != nil || ci.HubID != hub || !reg.classes.IsA(ci.Class, class) {
		return invalid(path, "cmdb.asset_reference_invalid")
	}
	return nil
}

// RegisterCiAsset writes a CI's asset record, its financial facts; the state moves by MoveCiAsset.
type RegisterCiAsset struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h RegisterCiAsset) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RegisterCiAssetName, Permission: service.PermissionWriteItems,
		Summary: "Writes a configuration item's asset record: its tag, cost in minor units with its currency, " +
			"purchase day, supplier, warranty end, contract (a contract CI), stockroom (a location CI), the person " +
			"holding it and the request it came from. A new record starts in the state given - REQUESTED when " +
			"none -; an existing one keeps its state, which MoveCiAsset moves. A field left out stays; null clears it.",
		SideEffects: "Writes the asset record, an audit entry and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "asset_tag", Kind: usecase.KindString, Description: "The tag on the object."},
			{Name: "cost_minor", Kind: usecase.KindInt, Description: "What it cost, in the currency's minor unit."},
			{Name: "currency", Kind: usecase.KindString, Description: "The cost's currency, ISO 4217."},
			{Name: "purchased_on", Kind: usecase.KindString, Description: "The purchase day, YYYY-MM-DD; null clears."},
			{Name: "supplier", Kind: usecase.KindString, Description: "Who supplied it."},
			{Name: "warranty_until", Kind: usecase.KindString, Description: "The warranty's last day, YYYY-MM-DD; null clears."},
			{Name: "contract_id", Kind: usecase.KindID, Description: "A contract CI of the hub; null clears."},
			{Name: "stockroom_id", Kind: usecase.KindID, Description: "A location CI of the hub where it is kept; null clears."},
			{Name: "assigned_to", Kind: usecase.KindID, Description: "The person holding it; null clears."},
			{Name: "request_id", Kind: usecase.KindID, Description: "The request entry it came from; null clears."},
			{Name: "state", Kind: usecase.KindString, Enum: assetStateNames(), Description: "A new record's first state."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; zero or absent for a new record."},
		},
		Audit: usecase.AuditDeclaration{Action: AssetRegisteredAction, TargetType: assetTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			placed, err := s.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, placed.HubID, true, AssetRegisteredAction, assetTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := s.Assurance.FindAsset(ctx, id)
				version := a.Version
				if err != nil {
					if !errors.Is(err, shared.ErrNotFound) {
						return err
					}
					a, version = domain.Asset{CIID: id, HubID: placed.HubID, State: domain.AssetRequested}, 0
					if st := in.String("state"); st != "" {
						a.State = domain.AssetState(st)
					}
				}
				if in.Present("expected_version") && in.Int("expected_version") != version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				fields, err := assetFields(&a, in)
				if err != nil {
					return err
				}
				reg, err := s.registry(ctx)
				if err != nil {
					return err
				}
				if err := s.assetRef(ctx, reg, a.HubID, a.ContractID, "contract", "/contract_id"); err != nil {
					return err
				}
				if err := s.assetRef(ctx, reg, a.HubID, a.StockroomID, "location", "/stockroom_id"); err != nil {
					return err
				}
				if err := a.Check(); err != nil {
					return err
				}
				a.UpdatedBy, a.UpdatedAt = actor.AccountID, s.Clock.Now()
				if err := s.Assurance.PutAsset(ctx, a, version); err != nil {
					return err
				}
				a.Version = version + 1
				if version == 0 {
					if err := s.followStage(ctx, actor, a); err != nil {
						return err
					}
				}
				out = AssetOutput(a, a.UpdatedAt)
				if err := s.announceAsset(ctx, actor, a); err != nil {
					return err
				}
				changes := make([]audit.Change, 0, len(fields))
				for _, f := range fields {
					changes = append(changes, audit.Change{Field: f, Classification: audit.Open})
				}
				return s.record(ctx, actor, AssetRegisteredAction, assetTarget, id, changes...)
			})
			return out, err
		}),
	})
}

// assetFields applies what the input names to the record and answers the fields it named.
func assetFields(a *domain.Asset, in usecase.Input) ([]string, error) {
	var fields []string
	texts := map[string]*string{"asset_tag": &a.AssetTag, "currency": &a.Currency, "supplier": &a.Supplier}
	for _, f := range []string{"asset_tag", "currency", "supplier"} {
		if in.Present(f) {
			*texts[f] = in.String(f)
			fields = append(fields, f)
		}
	}
	if in.Present("cost_minor") {
		a.CostMinor = int64(in.Int("cost_minor"))
		fields = append(fields, "cost_minor")
	}
	for f, at := range map[string]**time.Time{"purchased_on": &a.PurchasedOn, "warranty_until": &a.WarrantyUntil} {
		if !in.Present(f) {
			continue
		}
		d, err := dayOf(in, f)
		if err != nil {
			return nil, err
		}
		*at = d
		fields = append(fields, f)
	}
	for f, at := range map[string]*shared.ID{"contract_id": &a.ContractID, "stockroom_id": &a.StockroomID, "assigned_to": &a.AssignedTo, "request_id": &a.RequestID} {
		if !in.Present(f) {
			continue
		}
		*at = ""
		if err := optionalID(in, f, func(id shared.ID) { *at = id }); err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

// followStage carries the asset's state to its CI's lifecycle stage: the asset leads.
func (s Inventory) followStage(ctx context.Context, actor appshared.ActorContext, a domain.Asset) error {
	ci, err := s.Store.FindCI(ctx, a.CIID, true)
	if err != nil {
		return err
	}
	if ci.Stage == a.State.Stage() {
		return nil
	}
	next := ci
	next.Stage = a.State.Stage()
	next.UpdatedBy, next.UpdatedAt = actor.AccountID, s.Clock.Now()
	if err := s.Store.UpdateCI(ctx, next, ci.Version); err != nil {
		return err
	}
	next.Version++
	if err := s.announce(ctx, actor, next); err != nil {
		return err
	}
	return s.recordCI(ctx, actor, CIChangedAction, next, []string{domain.FieldLifecycle})
}

// MoveCiAsset moves an asset through its life; its CI's stage follows.
type MoveCiAsset struct{ Inventory Inventory }

// Descriptor is the catalogue entry.
func (h MoveCiAsset) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: MoveCiAssetName, Permission: service.PermissionWriteItems,
		Summary: "Moves an asset to the next state of its life - REQUESTED, ORDERED, IN_STOCK, DEPLOYED (to a person " +
			"or a stockroom), RETIRED, DISPOSED -, a deployed one back to stock and a retired one back into it; the " +
			"CI's lifecycle stage follows. Disposing needs the method (WIPED, DESTROYED, RECYCLED, RETURNED, SOLD) " +
			"and the reference of the certificate that proves it; a disposed asset moves no more.",
		SideEffects: "Writes the asset record, the CI's stage, audit entries and the change log.", TokenScope: cisWrite,
		Input: []usecase.Field{
			{Name: "ci_id", Kind: usecase.KindID, Required: true, Description: "The configuration item."},
			{Name: "state", Kind: usecase.KindString, Required: true, Enum: assetStateNames(), Description: "Where it moves."},
			{Name: "disposal", Kind: usecase.KindObject, Description: "method and certificate, to dispose."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: AssetMovedAction, TargetType: assetTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("ci_id")
			if err != nil {
				return nil, err
			}
			s := h.Inventory
			placed, err := s.placeOf(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.authorizeHub(ctx, actor, placed.HubID, true, AssetMovedAction, assetTarget, id); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := s.Assurance.FindAsset(ctx, id)
				if err != nil {
					return err
				}
				if in.Present("expected_version") && in.Int("expected_version") != a.Version {
					return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
				}
				now := s.Clock.Now()
				var disposal *domain.Disposal
				if m, ok := in["disposal"].(map[string]any); ok {
					disposal = &domain.Disposal{Method: text(m, "method"), Certificate: text(m, "certificate"), By: actor.AccountID, At: now}
				}
				from := a.State
				moved, err := a.Move(domain.AssetState(in.String("state")), disposal)
				if err != nil {
					return err
				}
				moved.UpdatedBy, moved.UpdatedAt = actor.AccountID, now
				if err := s.Assurance.PutAsset(ctx, moved, a.Version); err != nil {
					return err
				}
				moved.Version = a.Version + 1
				if err := s.followStage(ctx, actor, moved); err != nil {
					return err
				}
				if err := s.announceAsset(ctx, actor, moved); err != nil {
					return err
				}
				out = AssetOutput(moved, now)
				return s.record(ctx, actor, AssetMovedAction, assetTarget, id,
					audit.Change{Field: "state", Classification: audit.Open, From: string(from), To: string(moved.State)})
			})
			return out, err
		}),
	})
}
