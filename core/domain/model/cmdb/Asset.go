// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AssetState is where an asset is in its financial life (research A2): requested, ordered,
// received into stock, deployed, retired and disposed of.
type AssetState string

const (
	AssetRequested AssetState = "REQUESTED"
	AssetOrdered   AssetState = "ORDERED"
	AssetInStock   AssetState = "IN_STOCK"
	AssetDeployed  AssetState = "DEPLOYED"
	AssetRetired   AssetState = "RETIRED"
	AssetDisposed  AssetState = "DISPOSED"
)

// AssetStates are the states in their order.
var AssetStates = []AssetState{AssetRequested, AssetOrdered, AssetInStock, AssetDeployed, AssetRetired, AssetDisposed}

// assetMoves is where each state may go: forward, a deployed asset back to stock, and a retired
// one back into use. Disposed is the end.
var assetMoves = map[AssetState][]AssetState{
	AssetRequested: {AssetOrdered, AssetInStock},
	AssetOrdered:   {AssetInStock},
	AssetInStock:   {AssetDeployed, AssetRetired},
	AssetDeployed:  {AssetInStock, AssetRetired},
	AssetRetired:   {AssetInStock, AssetDisposed},
}

// Stage is the CI's lifecycle stage the state carries: the asset leads, the CI follows.
func (s AssetState) Stage() Stage {
	switch s {
	case AssetRequested:
		return StagePlanned
	case AssetOrdered:
		return StageOrdered
	case AssetInStock:
		return StageInStock
	case AssetDeployed:
		return StageInUse
	case AssetRetired:
		return StageRetired
	default:
		return StageDisposed
	}
}

// DisposalMethods are how an asset leaves (ISO 27001 A.7.14: secure disposal or re-use).
var DisposalMethods = []string{"WIPED", "DESTROYED", "RECYCLED", "RETURNED", "SOLD"}

// Disposal is the record that an asset was disposed of securely: how, the certificate's reference
// that proves it, and who and when.
type Disposal struct {
	Method      string
	Certificate string
	By          shared.ID
	At          time.Time
}

// Asset is a CI's financial record (research A1), one per CI: its tag, cost, purchase, supplier,
// warranty and contract, where it is stocked and who holds it.
type Asset struct {
	CIID          shared.ID
	TenantID      shared.ID
	HubID         shared.ID
	AssetTag      string
	CostMinor     int64
	Currency      string
	PurchasedOn   *time.Time
	Supplier      string
	WarrantyUntil *time.Time
	ContractID    shared.ID
	StockroomID   shared.ID
	AssignedTo    shared.ID
	RequestID     shared.ID
	State         AssetState
	Disposal      *Disposal
	UpdatedBy     shared.ID
	UpdatedAt     time.Time
	Version       int
}

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// Check refuses a malformed register entry: a tag or supplier too long, a negative cost, a cost
// without a currency, a warranty ending before the purchase, an unknown state.
func (a Asset) Check() error {
	if len([]rune(a.AssetTag)) > 100 || strings.ContainsAny(a.AssetTag, "\n\r") {
		return invalid("/asset_tag", "cmdb.asset_tag_invalid")
	}
	if len([]rune(a.Supplier)) > MaxNameLength || strings.ContainsAny(a.Supplier, "\n\r") {
		return invalid("/supplier", "cmdb.asset_supplier_invalid")
	}
	if a.CostMinor < 0 {
		return invalid("/cost_minor", "cmdb.asset_cost_invalid")
	}
	if a.Currency != "" && !currencyPattern.MatchString(a.Currency) || a.CostMinor > 0 && a.Currency == "" {
		return invalid("/currency", "cmdb.asset_currency_invalid")
	}
	if a.PurchasedOn != nil && a.WarrantyUntil != nil && a.WarrantyUntil.Before(*a.PurchasedOn) {
		return invalid("/warranty_until", "cmdb.asset_warranty_before_purchase")
	}
	if !slices.Contains(AssetStates, a.State) {
		return invalid("/state", "cmdb.asset_state_unknown")
	}
	return nil
}

// Move takes the asset to a state it may reach; disposing needs a method and the certificate's
// reference, and a disposed asset moves no more.
func (a Asset) Move(to AssetState, disposal *Disposal) (Asset, error) {
	if !slices.Contains(AssetStates, to) {
		return a, invalid("/state", "cmdb.asset_state_unknown")
	}
	if !slices.Contains(assetMoves[a.State], to) {
		return a, shared.ErrConflict.WithDetail("cmdb.asset_move_refused").WithParams(map[string]string{"from": string(a.State), "to": string(to)})
	}
	if to == AssetDisposed {
		if disposal == nil || !slices.Contains(DisposalMethods, disposal.Method) {
			return a, invalid("/disposal/method", "cmdb.disposal_method_unknown")
		}
		cert := strings.TrimSpace(disposal.Certificate)
		if cert == "" || len([]rune(cert)) > 200 || strings.ContainsAny(cert, "\n\r") {
			return a, invalid("/disposal/certificate", "cmdb.disposal_certificate_required")
		}
		d := *disposal
		d.Certificate = cert
		a.Disposal = &d
	}
	if to == AssetDeployed && a.AssignedTo.IsZero() && a.StockroomID.IsZero() {
		return a, invalid("/assigned_to", "cmdb.asset_deploy_unassigned")
	}
	a.State = to
	return a, nil
}

// WarrantyEnded says whether the warranty is over on a day.
func (a Asset) WarrantyEnded(now time.Time) bool {
	return a.WarrantyUntil != nil && a.WarrantyUntil.Before(now)
}
