// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CmdbAssuranceRepository stores the CMDB's assurance (migration 0267) through the unit of work's
// transaction (rule 3).
type CmdbAssuranceRepository struct{}

// NewCmdbAssuranceRepository answers the store.
func NewCmdbAssuranceRepository() CmdbAssuranceRepository { return CmdbAssuranceRepository{} }

var _ repository.Assurance = CmdbAssuranceRepository{}

func assuranceDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t.UTC(), Valid: true}
}

func assuranceDay(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time.UTC()
	return &t
}

func assuranceVersion(n int) int32 { return int32(n) } //nolint:gosec // a row's version

func (CmdbAssuranceRepository) AddAffected(ctx context.Context, a cmdb.Affected) (bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	var k cmdbKeys
	params := sqlc.InsertCmdbAffectedParams{ID: k.of(a.ID), ItemID: k.of(a.ItemID), CiID: k.of(a.CIID), AddedBy: k.of(a.AddedBy), At: timestampOf(a.AddedAt)}
	if k.err != nil {
		return false, k.err
	}
	n, err := q.InsertCmdbAffected(ctx, params)
	if err != nil {
		return false, cmdbFailed("naming an affected CI", err)
	}
	return n > 0, nil
}

func affectedFrom(id, item, ci, by pgtype.UUID, at pgtype.Timestamptz) (cmdb.Affected, error) {
	var k cmdbIDs
	a := cmdb.Affected{ID: k.of(id), ItemID: k.of(item), CIID: k.of(ci), AddedBy: k.of(by), AddedAt: timeFrom(at)}
	return a, k.err
}

func (CmdbAssuranceRepository) RemoveAffected(ctx context.Context, itemID, ciID shared.ID) (cmdb.Affected, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Affected{}, err
	}
	var k cmdbKeys
	params := sqlc.DeleteCmdbAffectedParams{ItemID: k.of(itemID), CiID: k.of(ciID)}
	if k.err != nil {
		return cmdb.Affected{}, shared.ErrNotFound.WithDetail("cmdb.affected_not_found")
	}
	r, err := q.DeleteCmdbAffected(ctx, params)
	if err != nil {
		return cmdb.Affected{}, cmdbRead(err, "cmdb.affected_not_found")
	}
	return affectedFrom(r.ID, r.ItemID, r.CiID, r.AddedBy, r.AddedAt)
}

func (CmdbAssuranceRepository) AffectedOf(ctx context.Context, itemID shared.ID) ([]cmdb.Affected, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbAffectedOfItem(ctx, item)
	if err != nil {
		return nil, cmdbFailed("listing affected CIs", err)
	}
	out := make([]cmdb.Affected, 0, len(rows))
	for _, r := range rows {
		a, err := affectedFrom(r.ID, r.ItemID, r.CiID, r.AddedBy, r.AddedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (CmdbAssuranceRepository) AffectingAny(ctx context.Context, ciIDs []shared.ID, limit int) ([]cmdb.Affected, error) {
	q, err := queriesFrom(ctx)
	if err != nil || len(ciIDs) == 0 {
		return nil, err
	}
	var k cmdbKeys
	keys := make([]pgtype.UUID, 0, len(ciIDs))
	for _, id := range ciIDs {
		keys = append(keys, k.of(id))
	}
	if k.err != nil {
		return nil, k.err
	}
	rows, err := q.ListCmdbAffectedOfCis(ctx, sqlc.ListCmdbAffectedOfCisParams{CiIds: keys, Batch: int32(limit)}) //nolint:gosec // bounded by the caller
	if err != nil {
		return nil, cmdbFailed("listing entries on CIs", err)
	}
	out := make([]cmdb.Affected, 0, len(rows))
	for _, r := range rows {
		a, err := affectedFrom(r.ID, r.ItemID, r.CiID, r.AddedBy, r.AddedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

type askedDoc struct {
	CI    string  `json:"ci_id"`
	Owner *string `json:"owner_id"`
}

func (CmdbAssuranceRepository) InsertCampaign(ctx context.Context, c cmdb.Campaign, asked []cmdb.Attestation) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.InsertCmdbCampaignParams{
		ID: k.of(c.ID), HubID: k.of(c.HubID), Name: c.Name, Classes: append([]string{}, c.Classes...),
		DueBy: assuranceDate(c.DueBy), DueOn: pgtype.Date{Time: c.DueOn.UTC(), Valid: true}, ReviewMonths: int32(c.ReviewMonths), //nolint:gosec // bounded by the domain
		StartedBy: k.of(c.StartedBy), StartedAt: timestampOf(c.StartedAt),
	}
	if k.err != nil {
		return k.err
	}
	if err := q.InsertCmdbCampaign(ctx, params); err != nil {
		return cmdbFailed("starting a campaign", err)
	}
	docs := make([]askedDoc, 0, len(asked))
	for _, a := range asked {
		d := askedDoc{CI: a.CIID.String()}
		if !a.Owner.IsZero() {
			o := a.Owner.String()
			d.Owner = &o
		}
		docs = append(docs, d)
	}
	raw, err := json.Marshal(docs)
	if err != nil {
		return cmdbFailed("asking CIs", err)
	}
	if err := q.InsertCmdbAttestations(ctx, sqlc.InsertCmdbAttestationsParams{CampaignID: params.ID, Asked: raw}); err != nil {
		return cmdbFailed("asking CIs", err)
	}
	return nil
}

func campaignFrom(r sqlc.FindCmdbCampaignRow) (cmdb.Campaign, error) {
	var k cmdbIDs
	c := cmdb.Campaign{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Name: r.Name, Classes: append([]string{}, r.Classes...),
		DueBy: assuranceDay(r.DueBy), ReviewMonths: int(r.ReviewMonths), StartedBy: k.of(r.StartedBy),
		StartedAt: timeFrom(r.StartedAt), ClosedAt: optionalTime(r.ClosedAt), ClosedBy: k.of(r.ClosedBy), Version: int(r.Version),
	}
	if d := assuranceDay(r.DueOn); d != nil {
		c.DueOn = *d
	}
	return c, k.err
}

func (CmdbAssuranceRepository) FindCampaign(ctx context.Context, id shared.ID) (cmdb.Campaign, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Campaign{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Campaign{}, shared.ErrNotFound.WithDetail("cmdb.campaign_not_found")
	}
	r, err := q.FindCmdbCampaign(ctx, key)
	if err != nil {
		return cmdb.Campaign{}, cmdbRead(err, "cmdb.campaign_not_found")
	}
	return campaignFrom(r)
}

func (CmdbAssuranceRepository) Campaigns(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Campaign, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbCampaigns(ctx, sqlc.ListCmdbCampaignsParams{HubID: hub, Batch: int32(limit)}) //nolint:gosec // bounded by the caller
	if err != nil {
		return nil, cmdbFailed("listing campaigns", err)
	}
	out := make([]cmdb.Campaign, 0, len(rows))
	for _, r := range rows {
		c, err := campaignFrom(sqlc.FindCmdbCampaignRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (CmdbAssuranceRepository) CloseCampaign(ctx context.Context, id shared.ID, at time.Time, by shared.ID, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.CloseCmdbCampaignParams{ID: k.of(id), At: timestampOf(at), By: k.of(by), ExpectedVersion: assuranceVersion(expected)}
	if k.err != nil {
		return k.err
	}
	n, err := q.CloseCmdbCampaign(ctx, params)
	if err != nil {
		return cmdbFailed("closing a campaign", err)
	}
	if n == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

func attestationFrom(r sqlc.FindCmdbAttestationRow) (cmdb.Attestation, error) {
	var k cmdbIDs
	a := cmdb.Attestation{
		CampaignID: k.of(r.CampaignID), CIID: k.of(r.CiID), Owner: k.of(r.OwnerID), Verdict: cmdb.Verdict(r.Verdict),
		AttestedBy: k.of(r.AttestedBy), AttestedAt: optionalTime(r.AttestedAt),
	}
	return a, k.err
}

func (CmdbAssuranceRepository) Attestations(ctx context.Context, campaignID shared.ID) ([]cmdb.Attestation, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(campaignID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbAttestations(ctx, key)
	if err != nil {
		return nil, cmdbFailed("listing attestations", err)
	}
	out := make([]cmdb.Attestation, 0, len(rows))
	for _, r := range rows {
		a, err := attestationFrom(sqlc.FindCmdbAttestationRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (CmdbAssuranceRepository) Tallies(ctx context.Context, campaignIDs []shared.ID) (map[shared.ID]map[cmdb.Verdict]int, error) {
	out := map[shared.ID]map[cmdb.Verdict]int{}
	q, err := queriesFrom(ctx)
	if err != nil || len(campaignIDs) == 0 {
		return out, err
	}
	var k cmdbKeys
	keys := make([]pgtype.UUID, 0, len(campaignIDs))
	for _, id := range campaignIDs {
		keys = append(keys, k.of(id))
	}
	if k.err != nil {
		return nil, k.err
	}
	rows, err := q.CountCmdbAttestations(ctx, keys)
	if err != nil {
		return nil, cmdbFailed("counting attestations", err)
	}
	var ids cmdbIDs
	for _, r := range rows {
		c := ids.of(r.CampaignID)
		if out[c] == nil {
			out[c] = map[cmdb.Verdict]int{}
		}
		out[c][cmdb.Verdict(r.Verdict)] = int(r.N)
	}
	return out, ids.err
}

func (CmdbAssuranceRepository) FindAttestation(ctx context.Context, campaignID, ciID shared.ID) (cmdb.Attestation, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Attestation{}, err
	}
	var k cmdbKeys
	params := sqlc.FindCmdbAttestationParams{CampaignID: k.of(campaignID), CiID: k.of(ciID)}
	if k.err != nil {
		return cmdb.Attestation{}, shared.ErrNotFound.WithDetail("cmdb.attestation_not_found")
	}
	r, err := q.FindCmdbAttestation(ctx, params)
	if err != nil {
		return cmdb.Attestation{}, cmdbRead(err, "cmdb.attestation_not_found")
	}
	return attestationFrom(r)
}

func (CmdbAssuranceRepository) Answer(ctx context.Context, a cmdb.Attestation) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.AnswerCmdbAttestationParams{
		CampaignID: k.of(a.CampaignID), CiID: k.of(a.CIID), Verdict: string(a.Verdict), By: k.opt(a.AttestedBy),
		At: optionalTimestamp(a.AttestedAt),
	}
	if k.err != nil {
		return k.err
	}
	if err := q.AnswerCmdbAttestation(ctx, params); err != nil {
		return cmdbFailed("answering an attestation", err)
	}
	return nil
}

type filterDoc struct {
	Classes    []string          `json:"classes,omitempty"`
	Stages     []string          `json:"stages,omitempty"`
	NameHas    string            `json:"name_has,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

func (CmdbAssuranceRepository) PutGroup(ctx context.Context, g cmdb.Group, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	doc := filterDoc{Classes: g.Filter.Classes, NameHas: g.Filter.NameHas, Attributes: g.Filter.Attributes}
	for _, s := range g.Filter.Stages {
		doc.Stages = append(doc.Stages, string(s))
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbGroupParams{
		ID: k.of(g.ID), HubID: k.of(g.HubID), Key: g.Key, Name: g.Name, Filter: cmdbJSON(doc), ServiceID: k.opt(g.ServiceID),
		UpdatedBy: k.of(g.UpdatedBy), At: timestampOf(g.UpdatedAt), ExpectedVersion: assuranceVersion(expected),
	}
	if k.err != nil {
		return k.err
	}
	n, err := q.UpsertCmdbGroup(ctx, params)
	if err != nil {
		return cmdbFailed("writing a group", err)
	}
	if n == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

func groupFrom(r sqlc.FindCmdbGroupRow) (cmdb.Group, error) {
	var doc filterDoc
	if err := json.Unmarshal(r.Filter, &doc); err != nil {
		return cmdb.Group{}, cmdbFailed("reading a group", err)
	}
	f := cmdb.Filter{Classes: doc.Classes, NameHas: doc.NameHas, Attributes: doc.Attributes}
	for _, s := range doc.Stages {
		f.Stages = append(f.Stages, cmdb.Stage(s))
	}
	var k cmdbIDs
	g := cmdb.Group{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Key: r.Key, Name: r.Name, Filter: f, ServiceID: k.of(r.ServiceID),
		UpdatedBy: k.of(r.UpdatedBy), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}
	return g, k.err
}

func (CmdbAssuranceRepository) Groups(ctx context.Context, hubID shared.ID) ([]cmdb.Group, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbGroups(ctx, hub)
	if err != nil {
		return nil, cmdbFailed("listing groups", err)
	}
	out := make([]cmdb.Group, 0, len(rows))
	for _, r := range rows {
		g, err := groupFrom(sqlc.FindCmdbGroupRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (CmdbAssuranceRepository) FindGroup(ctx context.Context, id shared.ID) (cmdb.Group, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Group{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Group{}, shared.ErrNotFound.WithDetail("cmdb.group_not_found")
	}
	r, err := q.FindCmdbGroup(ctx, key)
	if err != nil {
		return cmdb.Group{}, cmdbRead(err, "cmdb.group_not_found")
	}
	return groupFrom(r)
}

type disposalDoc struct {
	Method      string    `json:"method"`
	Certificate string    `json:"certificate"`
	By          string    `json:"by"`
	At          time.Time `json:"at"`
}

func (CmdbAssuranceRepository) PutAsset(ctx context.Context, a cmdb.Asset, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var disposal []byte
	if a.Disposal != nil {
		disposal = cmdbJSON(disposalDoc{a.Disposal.Method, a.Disposal.Certificate, a.Disposal.By.String(), a.Disposal.At.UTC()})
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbAssetParams{
		CiID: k.of(a.CIID), HubID: k.of(a.HubID), AssetTag: a.AssetTag, CostMinor: a.CostMinor, Currency: a.Currency,
		PurchasedOn: assuranceDate(a.PurchasedOn), Supplier: a.Supplier, WarrantyUntil: assuranceDate(a.WarrantyUntil),
		ContractID: k.opt(a.ContractID), StockroomID: k.opt(a.StockroomID), AssignedTo: k.opt(a.AssignedTo),
		RequestID: k.opt(a.RequestID), State: string(a.State), Disposal: disposal, UpdatedBy: k.of(a.UpdatedBy),
		At: timestampOf(a.UpdatedAt), ExpectedVersion: assuranceVersion(expected),
	}
	if k.err != nil {
		return k.err
	}
	n, err := q.UpsertCmdbAsset(ctx, params)
	if err != nil {
		return cmdbFailed("writing an asset", err)
	}
	if n == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

func assetFrom(r sqlc.FindCmdbAssetRow) (cmdb.Asset, error) {
	var k cmdbIDs
	a := cmdb.Asset{
		CIID: k.of(r.CiID), HubID: k.of(r.HubID), AssetTag: r.AssetTag, CostMinor: r.CostMinor, Currency: r.Currency,
		PurchasedOn: assuranceDay(r.PurchasedOn), Supplier: r.Supplier, WarrantyUntil: assuranceDay(r.WarrantyUntil),
		ContractID: k.of(r.ContractID), StockroomID: k.of(r.StockroomID), AssignedTo: k.of(r.AssignedTo),
		RequestID: k.of(r.RequestID), State: cmdb.AssetState(r.State), UpdatedBy: k.of(r.UpdatedBy),
		UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}
	if len(r.Disposal) > 0 {
		var d disposalDoc
		if err := json.Unmarshal(r.Disposal, &d); err != nil {
			return cmdb.Asset{}, cmdbFailed("reading an asset", err)
		}
		by, _ := shared.ParseID(d.By)
		a.Disposal = &cmdb.Disposal{Method: d.Method, Certificate: d.Certificate, By: by, At: d.At}
	}
	return a, k.err
}

func (CmdbAssuranceRepository) FindAsset(ctx context.Context, ciID shared.ID) (cmdb.Asset, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Asset{}, err
	}
	key, err := uuidOf(ciID)
	if err != nil {
		return cmdb.Asset{}, shared.ErrNotFound.WithDetail("cmdb.asset_not_found")
	}
	r, err := q.FindCmdbAsset(ctx, key)
	if err != nil {
		return cmdb.Asset{}, cmdbRead(err, "cmdb.asset_not_found")
	}
	return assetFrom(r)
}

func (CmdbAssuranceRepository) Assets(ctx context.Context, hubID, after shared.ID, limit int) ([]cmdb.Asset, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	var k cmdbKeys
	params := sqlc.ListCmdbAssetsParams{HubID: k.of(hubID), Batch: int32(limit)} //nolint:gosec // bounded by the caller
	if after.IsZero() {
		params.After = pgtype.UUID{Valid: true}
	} else {
		params.After = k.of(after)
	}
	if k.err != nil {
		return nil, k.err
	}
	rows, err := q.ListCmdbAssets(ctx, params)
	if err != nil {
		return nil, cmdbFailed("listing assets", err)
	}
	out := make([]cmdb.Asset, 0, len(rows))
	for _, r := range rows {
		a, err := assetFrom(sqlc.FindCmdbAssetRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (CmdbAssuranceRepository) PutAlias(ctx context.Context, a cmdb.SoftwareAlias, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbSoftwareAliasParams{
		ID: k.of(a.ID), MatchPublisher: a.MatchPublisher, MatchProduct: a.MatchProduct, Publisher: a.Publisher,
		Product: a.Product, UpdatedBy: k.of(a.UpdatedBy), At: timestampOf(a.UpdatedAt), ExpectedVersion: assuranceVersion(expected),
	}
	if k.err != nil {
		return k.err
	}
	n, err := q.UpsertCmdbSoftwareAlias(ctx, params)
	if err != nil {
		return cmdbFailed("writing a software alias", err)
	}
	if n == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

func (CmdbAssuranceRepository) Aliases(ctx context.Context) ([]cmdb.SoftwareAlias, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbSoftwareAliases(ctx)
	if err != nil {
		return nil, cmdbFailed("listing software aliases", err)
	}
	out := make([]cmdb.SoftwareAlias, 0, len(rows))
	var k cmdbIDs
	for _, r := range rows {
		out = append(out, cmdb.SoftwareAlias{
			ID: k.of(r.ID), MatchPublisher: r.MatchPublisher, MatchProduct: r.MatchProduct, Publisher: r.Publisher,
			Product: r.Product, UpdatedBy: k.of(r.UpdatedBy), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
		})
	}
	return out, k.err
}

func (CmdbAssuranceRepository) RemoveAlias(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return shared.ErrNotFound.WithDetail("cmdb.alias_not_found")
	}
	n, err := q.DeleteCmdbSoftwareAlias(ctx, key)
	if err != nil {
		return cmdbFailed("removing a software alias", err)
	}
	if n == 0 {
		return shared.ErrNotFound.WithDetail("cmdb.alias_not_found")
	}
	return nil
}

func (CmdbAssuranceRepository) Installations(ctx context.Context, hubID shared.ID) (map[shared.ID]int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.CountCmdbInstallations(ctx, hub)
	if err != nil {
		return nil, cmdbFailed("counting installations", err)
	}
	out := make(map[shared.ID]int, len(rows))
	var k cmdbIDs
	for _, r := range rows {
		out[k.of(r.CiID)] = int(r.N)
	}
	return out, k.err
}
