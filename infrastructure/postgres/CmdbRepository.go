// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CmdbRepository stores the assets and configuration module (migration 0236) through the unit of
// work's transaction (rule 3).
type CmdbRepository struct{}

// NewCmdbRepository answers the store.
func NewCmdbRepository() CmdbRepository { return CmdbRepository{} }

var _ repository.Store = CmdbRepository{}

func cmdbFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("cmdb %s: %w", what, err))
}

func cmdbRead(err error, notFound string) error {
	if IsNoRows(err) {
		return shared.ErrNotFound.WithDetail(notFound)
	}
	return cmdbFailed("reading", err)
}

// keys turns identifiers into UUIDs and keeps the first malformed one's error.
type cmdbKeys struct{ err error }

func (k *cmdbKeys) of(id shared.ID) pgtype.UUID {
	v, err := uuidOf(id)
	if err != nil && k.err == nil {
		k.err = err
	}
	return v
}

func (k *cmdbKeys) opt(id shared.ID) pgtype.UUID {
	if id.IsZero() {
		return pgtype.UUID{}
	}
	return k.of(id)
}

type cmdbIDs struct{ err error }

func (k *cmdbIDs) of(v pgtype.UUID) shared.ID {
	id, err := optionalID(v)
	if err != nil && k.err == nil {
		k.err = err
	}
	return id
}

// The JSON documents of the configuration columns, in the contract's names.
type attributeDoc struct {
	Key        string   `json:"key"`
	Kind       string   `json:"kind"`
	Need       string   `json:"need"`
	Identifier bool     `json:"identifier,omitempty"`
	Options    []string `json:"options,omitempty"`
}

type ruleDoc struct {
	Attributes []string `json:"attributes"`
}

type requiredDoc struct {
	Types    []string `json:"types"`
	Outgoing bool     `json:"outgoing"`
}

type pairDoc struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type mappingDoc struct {
	Class           string            `json:"class"`
	ClassColumn     string            `json:"class_column,omitempty"`
	NameColumn      string            `json:"name_column"`
	NativeKeyColumn string            `json:"native_key_column,omitempty"`
	Columns         map[string]string `json:"columns,omitempty"`
	Defaults        map[string]string `json:"defaults,omitempty"`
}

type provenanceDoc struct {
	Source string    `json:"source"`
	Run    string    `json:"run,omitempty"`
	At     time.Time `json:"at"`
}

type protectionDoc struct {
	C      string   `json:"c,omitempty"`
	I      string   `json:"i,omitempty"`
	A      string   `json:"a,omitempty"`
	Fixed  []string `json:"fixed,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

type keptDoc struct {
	Attribute string `json:"attribute"`
	By        string `json:"by"`
}

type decisionDoc struct {
	Ref        string    `json:"ref"`
	Outcome    string    `json:"outcome"`
	CI         string    `json:"ci_id,omitempty"`
	Class      string    `json:"class,omitempty"`
	Rule       string    `json:"rule,omitempty"`
	Taken      []string  `json:"taken,omitempty"`
	Kept       []keptDoc `json:"kept,omitempty"`
	Candidates []string  `json:"candidates,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

type itemDoc struct {
	Ref        string         `json:"ref,omitempty"`
	Class      string         `json:"class"`
	Name       string         `json:"name"`
	NativeKey  string         `json:"native_key,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// EncodeCmdbMapping is a source's file mapping as the column stores it.
func EncodeCmdbMapping(m *cmdb.FileMapping) []byte {
	if m == nil {
		return nil
	}
	raw, _ := json.Marshal(mappingDoc{m.Class, m.ClassColumn, m.NameColumn, m.NativeKeyColumn, m.Columns, m.Defaults})
	return raw
}

func decodeCmdbMapping(raw []byte) (*cmdb.FileMapping, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var d mappingDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &cmdb.FileMapping{Class: d.Class, ClassColumn: d.ClassColumn, NameColumn: d.NameColumn,
		NativeKeyColumn: d.NativeKeyColumn, Columns: d.Columns, Defaults: d.Defaults}, nil
}

func cmdbJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return raw
}

// Classes answers the workspace's own classes.
func (CmdbRepository) Classes(ctx context.Context) ([]cmdb.Class, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbClasses(ctx)
	if err != nil {
		return nil, cmdbFailed("listing classes", err)
	}
	out := make([]cmdb.Class, 0, len(rows))
	for _, r := range rows {
		var attrs []attributeDoc
		var rules []ruleDoc
		var required []requiredDoc
		if err := json.Unmarshal(r.Attributes, &attrs); err != nil {
			return nil, cmdbFailed("decoding a class", err)
		}
		if err := json.Unmarshal(r.Rules, &rules); err != nil {
			return nil, cmdbFailed("decoding a class", err)
		}
		if err := json.Unmarshal(r.Required, &required); err != nil {
			return nil, cmdbFailed("decoding a class", err)
		}
		var k cmdbIDs
		c := cmdb.Class{
			ID: k.of(r.ID), Key: r.Key, Parent: r.ParentKey, Name: r.Name, Icon: r.Icon, Abstract: r.Abstract,
			StalenessDays: int(r.StalenessDays), ArchivedAt: optionalTime(r.ArchivedAt),
			CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
		}
		for _, a := range attrs {
			c.Attributes = append(c.Attributes, cmdb.Attribute{Key: a.Key, Kind: cmdb.AttributeKind(a.Kind), Need: cmdb.Need(a.Need), Identifier: a.Identifier, Options: a.Options})
		}
		for _, rule := range rules {
			c.Rules = append(c.Rules, cmdb.IdentifierRule{Attributes: rule.Attributes})
		}
		for _, req := range required {
			c.Required = append(c.Required, cmdb.RequiredRelation{Types: req.Types, Outgoing: req.Outgoing})
		}
		if k.err != nil {
			return nil, k.err
		}
		out = append(out, c)
	}
	return out, nil
}

// PutClass writes a class at the version expected.
func (CmdbRepository) PutClass(ctx context.Context, c cmdb.Class, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	attrs := make([]attributeDoc, 0, len(c.Attributes))
	for _, a := range c.Attributes {
		attrs = append(attrs, attributeDoc{a.Key, string(a.Kind), string(a.Need), a.Identifier, a.Options})
	}
	rules := make([]ruleDoc, 0, len(c.Rules))
	for _, r := range c.Rules {
		rules = append(rules, ruleDoc{r.Attributes})
	}
	required := make([]requiredDoc, 0, len(c.Required))
	for _, r := range c.Required {
		required = append(required, requiredDoc{r.Types, r.Outgoing})
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbClassParams{
		ID: k.of(c.ID), Key: c.Key, ParentKey: c.Parent, Name: c.Name, Icon: c.Icon, Abstract: c.Abstract,
		Attributes: cmdbJSON(attrs), Rules: cmdbJSON(rules), Required: cmdbJSON(required),
		StalenessDays: int32(c.StalenessDays), ArchivedAt: optionalTimestamp(c.ArchivedAt), //nolint:gosec // bounded by the domain
		At: timestampOf(c.UpdatedAt), ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	}
	if k.err != nil {
		return k.err
	}
	rows, err := q.UpsertCmdbClass(ctx, params)
	if err != nil {
		return cmdbFailed("writing a class", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

// RelationTypes answers the workspace's own relation types.
func (CmdbRepository) RelationTypes(ctx context.Context) ([]cmdb.RelationType, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbRelationTypes(ctx)
	if err != nil {
		return nil, cmdbFailed("listing relation types", err)
	}
	out := make([]cmdb.RelationType, 0, len(rows))
	for _, r := range rows {
		var pairs []pairDoc
		if err := json.Unmarshal(r.Pairs, &pairs); err != nil {
			return nil, cmdbFailed("decoding a relation type", err)
		}
		var k cmdbIDs
		t := cmdb.RelationType{
			ID: k.of(r.ID), Key: r.Key, Name: r.Name, ReverseName: r.ReverseName, Impact: cmdb.Impact(r.Impact),
			ArchivedAt: optionalTime(r.ArchivedAt), CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt),
			Version: int(r.Version),
		}
		for _, p := range pairs {
			t.Pairs = append(t.Pairs, cmdb.ClassPair{From: p.From, To: p.To})
		}
		if k.err != nil {
			return nil, k.err
		}
		out = append(out, t)
	}
	return out, nil
}

// PutRelationType writes a relation type at the version expected.
func (CmdbRepository) PutRelationType(ctx context.Context, t cmdb.RelationType, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	pairs := make([]pairDoc, 0, len(t.Pairs))
	for _, p := range t.Pairs {
		pairs = append(pairs, pairDoc{p.From, p.To})
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbRelationTypeParams{
		ID: k.of(t.ID), Key: t.Key, Name: t.Name, ReverseName: t.ReverseName, Impact: string(t.Impact),
		Pairs: cmdbJSON(pairs), ArchivedAt: optionalTimestamp(t.ArchivedAt), At: timestampOf(t.UpdatedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	}
	if k.err != nil {
		return k.err
	}
	rows, err := q.UpsertCmdbRelationType(ctx, params)
	if err != nil {
		return cmdbFailed("writing a relation type", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

// Sources answers the workspace's own sources.
func (CmdbRepository) Sources(ctx context.Context) ([]cmdb.Source, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbSources(ctx)
	if err != nil {
		return nil, cmdbFailed("listing sources", err)
	}
	out := make([]cmdb.Source, 0, len(rows))
	for _, r := range rows {
		mapping, err := decodeCmdbMapping(r.Mapping)
		if err != nil {
			return nil, cmdbFailed("decoding a source", err)
		}
		var k cmdbIDs
		s := cmdb.Source{
			ID: k.of(r.ID), Key: r.Key, Kind: cmdb.SourceKind(r.Kind), Rank: int(r.Rank), Classes: r.Classes,
			MayCreate: r.MayCreate, Mapping: mapping, LastRunAt: optionalTime(r.LastRunAt),
			CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
		}
		if k.err != nil {
			return nil, k.err
		}
		out = append(out, s)
	}
	return out, nil
}

// PutSource writes a source at the version expected.
func (CmdbRepository) PutSource(ctx context.Context, s cmdb.Source, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbSourceParams{
		ID: k.of(s.ID), Key: s.Key, Kind: string(s.Kind), Rank: int32(s.Rank), Classes: append([]string{}, s.Classes...), //nolint:gosec // bounded by the domain
		MayCreate: s.MayCreate, Mapping: EncodeCmdbMapping(s.Mapping), At: timestampOf(s.UpdatedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	}
	if k.err != nil {
		return k.err
	}
	rows, err := q.UpsertCmdbSource(ctx, params)
	if err != nil {
		return cmdbFailed("writing a source", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

// TouchSource records a source's last run.
func (CmdbRepository) TouchSource(ctx context.Context, key string, at time.Time) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if err := q.TouchCmdbSource(ctx, sqlc.TouchCmdbSourceParams{Key: key, At: timestampOf(at)}); err != nil {
		return cmdbFailed("touching a source", err)
	}
	return nil
}

type ciColumns struct {
	attributes, provenance, protection []byte
	reviewDue                          pgtype.Date
}

func ciColumnsOf(ci cmdb.CI) ciColumns {
	prov := map[string]provenanceDoc{}
	for k, p := range ci.Provenance {
		prov[k] = provenanceDoc{Source: p.Source, Run: p.Run.String(), At: p.At.UTC()}
	}
	attrs := ci.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	p := ci.Compliance.Protection
	out := ciColumns{
		attributes: cmdbJSON(attrs), provenance: cmdbJSON(prov),
		protection: cmdbJSON(protectionDoc{string(p.C), string(p.I), string(p.A), p.Fixed, p.Reason}),
	}
	if ci.Compliance.ReviewDue != nil {
		out.reviewDue = pgtype.Date{Time: *ci.Compliance.ReviewDue, Valid: true}
	}
	return out
}

// InsertCI writes a new CI.
func (CmdbRepository) InsertCI(ctx context.Context, ci cmdb.CI) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	cols := ciColumnsOf(ci)
	var k cmdbKeys
	params := sqlc.InsertCmdbCiParams{
		ID: k.of(ci.ID), HubID: k.of(ci.HubID), ClassKey: ci.Class, Name: ci.Name, Stage: string(ci.Stage),
		Status: string(ci.Status), Attributes: cols.attributes, Provenance: cols.provenance,
		OwnerAccountID: k.opt(ci.Compliance.OwnerAccount), OwnerGroupID: k.opt(ci.Compliance.OwnerGroup),
		LocationCiID: k.opt(ci.Compliance.Location), Protection: cols.protection, ReviewDue: cols.reviewDue,
		Quantity: int32(ci.Compliance.Quantity), LastSeenAt: optionalTimestamp(ci.LastSeen), //nolint:gosec // bounded by the domain
		MergedInto: k.opt(ci.MergedInto), CreatedBy: k.of(ci.CreatedBy), CreatedAt: timestampOf(ci.CreatedAt),
	}
	if k.err != nil {
		return k.err
	}
	if err := q.InsertCmdbCi(ctx, params); err != nil {
		return cmdbFailed("writing a CI", err)
	}
	return nil
}

// UpdateCI writes a CI at the version expected.
func (CmdbRepository) UpdateCI(ctx context.Context, ci cmdb.CI, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	cols := ciColumnsOf(ci)
	var k cmdbKeys
	params := sqlc.UpdateCmdbCiParams{
		ID: k.of(ci.ID), ClassKey: ci.Class, Name: ci.Name, Stage: string(ci.Stage), Status: string(ci.Status),
		Attributes: cols.attributes, Provenance: cols.provenance,
		OwnerAccountID: k.opt(ci.Compliance.OwnerAccount), OwnerGroupID: k.opt(ci.Compliance.OwnerGroup),
		LocationCiID: k.opt(ci.Compliance.Location), Protection: cols.protection, ReviewDue: cols.reviewDue,
		Quantity: int32(ci.Compliance.Quantity), LastSeenAt: optionalTimestamp(ci.LastSeen), //nolint:gosec // bounded by the domain
		MergedInto: k.opt(ci.MergedInto), UpdatedBy: k.of(ci.UpdatedBy), UpdatedAt: timestampOf(ci.UpdatedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	}
	if k.err != nil {
		return k.err
	}
	rows, err := q.UpdateCmdbCi(ctx, params)
	if err != nil {
		return cmdbFailed("writing a CI", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

// ciFrom maps every CI row shape (they have the same columns).
func cmdbCIFrom(r sqlc.FindCmdbCiRow) (cmdb.CI, error) {
	var k cmdbIDs
	ci := cmdb.CI{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Class: r.ClassKey, Name: r.Name, Stage: cmdb.Stage(r.Stage),
		Status: cmdb.Status(r.Status), LastSeen: optionalTime(r.LastSeenAt), MergedInto: k.of(r.MergedInto),
		CreatedBy: k.of(r.CreatedBy), CreatedAt: timeFrom(r.CreatedAt), UpdatedBy: k.of(r.UpdatedBy),
		UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
		Compliance: cmdb.Compliance{
			OwnerAccount: k.of(r.OwnerAccountID), OwnerGroup: k.of(r.OwnerGroupID), Location: k.of(r.LocationCiID),
			Quantity: int(r.Quantity),
		},
	}
	if r.ReviewDue.Valid {
		day := r.ReviewDue.Time.UTC()
		ci.Compliance.ReviewDue = &day
	}
	if err := json.Unmarshal(r.Attributes, &ci.Attributes); err != nil {
		return cmdb.CI{}, cmdbFailed("decoding a CI", err)
	}
	var prov map[string]provenanceDoc
	if err := json.Unmarshal(r.Provenance, &prov); err != nil {
		return cmdb.CI{}, cmdbFailed("decoding a CI", err)
	}
	ci.Provenance = map[string]cmdb.Provenance{}
	for key, p := range prov {
		run, _ := shared.ParseID(p.Run)
		ci.Provenance[key] = cmdb.Provenance{Source: p.Source, Run: run, At: p.At.UTC()}
	}
	var p protectionDoc
	if err := json.Unmarshal(r.Protection, &p); err != nil {
		return cmdb.CI{}, cmdbFailed("decoding a CI", err)
	}
	ci.Compliance.Protection = cmdb.OwnProtection{
		Protection: cmdb.Protection{C: cmdb.Level(p.C), I: cmdb.Level(p.I), A: cmdb.Level(p.A)}, Fixed: p.Fixed, Reason: p.Reason,
	}
	return ci, k.err
}

// FindCI answers a CI, merged ones included.
func (CmdbRepository) FindCI(ctx context.Context, id shared.ID, lock bool) (cmdb.CI, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.CI{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.CI{}, shared.ErrNotFound.WithDetail("cmdb.ci_not_found")
	}
	var row sqlc.FindCmdbCiRow
	if lock {
		var locked sqlc.FindCmdbCiForUpdateRow
		locked, err = q.FindCmdbCiForUpdate(ctx, key)
		row = sqlc.FindCmdbCiRow(locked)
	} else {
		row, err = q.FindCmdbCi(ctx, key)
	}
	if err != nil {
		return cmdb.CI{}, cmdbRead(err, "cmdb.ci_not_found")
	}
	return cmdbCIFrom(row)
}

// ListCIs answers a page of a hub's CIs.
func (CmdbRepository) ListCIs(ctx context.Context, hubID shared.ID, classes []string, after shared.ID, limit int) ([]cmdb.CI, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	var k cmdbKeys
	cursor := pgtype.UUID{Valid: true}
	if !after.IsZero() {
		cursor = k.of(after)
	}
	params := sqlc.ListCmdbCisParams{HubID: k.of(hubID), Classes: append([]string{}, classes...), After: cursor, Batch: int32(limit)} //nolint:gosec // bounded by the caller
	if k.err != nil {
		return nil, k.err
	}
	rows, err := q.ListCmdbCis(ctx, params)
	if err != nil {
		return nil, cmdbFailed("listing CIs", err)
	}
	out := make([]cmdb.CI, 0, len(rows))
	for _, r := range rows {
		ci, err := cmdbCIFrom(sqlc.FindCmdbCiRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, ci)
	}
	return out, nil
}

// CIsByID answers the CIs named, in no order.
func (CmdbRepository) CIsByID(ctx context.Context, list []shared.ID) ([]cmdb.CI, error) {
	if len(list) == 0 {
		return nil, nil
	}
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keysOf, err := uuidsOf(list)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbCisByIDs(ctx, keysOf)
	if err != nil {
		return nil, cmdbFailed("reading CIs", err)
	}
	out := make([]cmdb.CI, 0, len(rows))
	for _, r := range rows {
		ci, err := cmdbCIFrom(sqlc.FindCmdbCiRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, ci)
	}
	return out, nil
}

// CountCIs counts a hub's CIs.
func (CmdbRepository) CountCIs(ctx context.Context, hubID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountCmdbCis(ctx, key)
	if err != nil {
		return 0, cmdbFailed("counting CIs", err)
	}
	return int(n), nil
}

func cmdbPairs(identifiers map[string]string) ([]string, []string) {
	names := slices.Sorted(maps.Keys(identifiers))
	values := make([]string, len(names))
	for i, n := range names {
		values[i] = identifiers[n]
	}
	return names, values
}

// PutIdentifiers replaces the CI's identifiers.
func (CmdbRepository) PutIdentifiers(ctx context.Context, ci cmdb.CI, identifiers map[string]string) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	id, hub := k.of(ci.ID), k.of(ci.HubID)
	if k.err != nil {
		return k.err
	}
	if err := q.DeleteCmdbCiIdentifiers(ctx, id); err != nil {
		return cmdbFailed("clearing identifiers", err)
	}
	names, values := cmdbPairs(identifiers)
	if len(names) == 0 {
		return nil
	}
	if err := q.PutCmdbCiIdentifiers(ctx, sqlc.PutCmdbCiIdentifiersParams{CiID: id, HubID: hub, Names: names, IdentifierValues: values}); err != nil {
		return cmdbFailed("writing identifiers", err)
	}
	return nil
}

// FindIdentifiers looks the pairs up in a hub's index.
func (CmdbRepository) FindIdentifiers(ctx context.Context, hubID shared.ID, identifiers map[string]string) ([]cmdb.IdentifierRow, error) {
	if len(identifiers) == 0 {
		return nil, nil
	}
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	names, values := cmdbPairs(identifiers)
	rows, err := q.FindCmdbIdentifiers(ctx, sqlc.FindCmdbIdentifiersParams{HubID: hub, Names: names, IdentifierValues: values})
	if err != nil {
		return nil, cmdbFailed("looking identifiers up", err)
	}
	out := make([]cmdb.IdentifierRow, 0, len(rows))
	for _, r := range rows {
		var k cmdbIDs
		out = append(out, cmdb.IdentifierRow{CI: k.of(r.CiID), Class: r.ClassKey, Name: r.Name, Value: r.Value})
		if k.err != nil {
			return nil, k.err
		}
	}
	return out, nil
}

// MergeInto moves the duplicate's source keys and relations to the survivor.
func (CmdbRepository) MergeInto(ctx context.Context, survivor, duplicate shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	s, d := k.of(survivor), k.of(duplicate)
	if k.err != nil {
		return k.err
	}
	if err := q.MoveCmdbNativeKeys(ctx, sqlc.MoveCmdbNativeKeysParams{SurvivorID: s, DuplicateID: d}); err != nil {
		return cmdbFailed("moving source keys", err)
	}
	if err := q.DropCmdbCiIdentifiersAll(ctx, d); err != nil {
		return cmdbFailed("dropping identifiers", err)
	}
	if err := q.MoveCmdbRelations(ctx, sqlc.MoveCmdbRelationsParams{SurvivorID: s, DuplicateID: d}); err != nil {
		return cmdbFailed("moving relations", err)
	}
	if err := q.DropCmdbRelationsOfCi(ctx, d); err != nil {
		return cmdbFailed("dropping relations", err)
	}
	return nil
}

func cmdbRelationFrom(r sqlc.FindCmdbRelationRow) (cmdb.Relation, error) {
	var k cmdbIDs
	rel := cmdb.Relation{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Type: r.TypeKey, SourceCI: k.of(r.SourceCiID), TargetCI: k.of(r.TargetCiID),
		DataSource: r.DataSource, FirstSeen: timeFrom(r.FirstSeen), LastSeen: timeFrom(r.LastSeen), CreatedBy: k.of(r.CreatedBy),
	}
	return rel, k.err
}

// PutRelation writes a relation or sees it again.
func (CmdbRepository) PutRelation(ctx context.Context, r cmdb.Relation) (shared.ID, bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return "", false, err
	}
	var k cmdbKeys
	params := sqlc.PutCmdbRelationParams{
		ID: k.of(r.ID), HubID: k.of(r.HubID), TypeKey: r.Type, SourceCiID: k.of(r.SourceCI), TargetCiID: k.of(r.TargetCI),
		DataSource: r.DataSource, At: timestampOf(r.LastSeen), CreatedBy: k.of(r.CreatedBy),
	}
	if k.err != nil {
		return "", false, k.err
	}
	row, err := q.PutCmdbRelation(ctx, params)
	if err != nil {
		return "", false, cmdbFailed("writing a relation", err)
	}
	id, err := idFrom(row.ID)
	return id, row.Inserted, err
}

// FindRelation answers a relation.
func (CmdbRepository) FindRelation(ctx context.Context, id shared.ID) (cmdb.Relation, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Relation{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Relation{}, shared.ErrNotFound.WithDetail("cmdb.relation_not_found")
	}
	row, err := q.FindCmdbRelation(ctx, key)
	if err != nil {
		return cmdb.Relation{}, cmdbRead(err, "cmdb.relation_not_found")
	}
	return cmdbRelationFrom(row)
}

// DeleteRelation removes a relation.
func (CmdbRepository) DeleteRelation(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return shared.ErrNotFound.WithDetail("cmdb.relation_not_found")
	}
	rows, err := q.DeleteCmdbRelation(ctx, key)
	if err != nil {
		return cmdbFailed("removing a relation", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("cmdb.relation_not_found")
	}
	return nil
}

// RelationsOf answers a CI's relations both ways.
func (CmdbRepository) RelationsOf(ctx context.Context, ciID shared.ID) ([]cmdb.Relation, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(ciID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbRelationsOfCi(ctx, key)
	if err != nil {
		return nil, cmdbFailed("listing relations", err)
	}
	out := make([]cmdb.Relation, 0, len(rows))
	for _, r := range rows {
		rel, err := cmdbRelationFrom(sqlc.FindCmdbRelationRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, nil
}

// HubRelations answers every relation of a hub, page by page.
func (CmdbRepository) HubRelations(ctx context.Context, hubID shared.ID) ([]cmdb.Relation, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	var out []cmdb.Relation
	cursor := pgtype.UUID{Valid: true}
	for {
		rows, err := q.ListCmdbRelationsOfHub(ctx, sqlc.ListCmdbRelationsOfHubParams{HubID: hub, After: cursor, Batch: 5000})
		if err != nil {
			return nil, cmdbFailed("listing relations", err)
		}
		for _, r := range rows {
			rel, err := cmdbRelationFrom(sqlc.FindCmdbRelationRow(r))
			if err != nil {
				return nil, err
			}
			out = append(out, rel)
		}
		if len(rows) < 5000 {
			return out, nil
		}
		cursor = rows[len(rows)-1].ID
	}
}

// Walk follows the impact from a CI.
func (CmdbRepository) Walk(ctx context.Context, ciID shared.ID, downstream bool, sourceSuffers, targetSuffers []string, depth int) ([]repository.Reached, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(ciID)
	if err != nil {
		return nil, err
	}
	rows, err := q.WalkCmdbImpact(ctx, sqlc.WalkCmdbImpactParams{
		CiID: key, Downstream: downstream, SourceSuffers: append([]string{}, sourceSuffers...),
		TargetSuffers: append([]string{}, targetSuffers...), MaxDepth: int32(depth), //nolint:gosec // bounded by the caller
	})
	if err != nil {
		return nil, cmdbFailed("walking the impact", err)
	}
	out := make([]repository.Reached, 0, len(rows))
	for _, r := range rows {
		id, err := idFrom(r.CiID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.Reached{CI: id, Depth: int(r.Depth)})
	}
	return out, nil
}

// EncodeCmdbDecisions is a run's explanation as the column stores it.
func EncodeCmdbDecisions(decisions []cmdb.Decision) []byte {
	out := make([]decisionDoc, 0, len(decisions))
	for _, d := range decisions {
		doc := decisionDoc{Ref: d.Ref, Outcome: string(d.Outcome), CI: d.CI.String(), Class: d.Class, Rule: d.Rule, Taken: d.Taken, Reason: d.Reason}
		for _, kept := range d.Kept {
			doc.Kept = append(doc.Kept, keptDoc{kept.Attribute, kept.By})
		}
		for _, c := range d.Candidates {
			doc.Candidates = append(doc.Candidates, c.String())
		}
		out = append(out, doc)
	}
	return cmdbJSON(out)
}

func decodeCmdbDecisions(raw []byte) ([]cmdb.Decision, error) {
	var docs []decisionDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		return nil, err
	}
	out := make([]cmdb.Decision, 0, len(docs))
	for _, doc := range docs {
		ci, _ := shared.ParseID(doc.CI)
		d := cmdb.Decision{Ref: doc.Ref, Outcome: cmdb.Outcome(doc.Outcome), CI: ci, Class: doc.Class, Rule: doc.Rule, Taken: doc.Taken, Reason: doc.Reason}
		for _, kept := range doc.Kept {
			d.Kept = append(d.Kept, cmdb.Kept{Attribute: kept.Attribute, By: kept.By})
		}
		for _, c := range doc.Candidates {
			if id, err := shared.ParseID(c); err == nil {
				d.Candidates = append(d.Candidates, id)
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// InsertRun records a run.
func (CmdbRepository) InsertRun(ctx context.Context, run cmdb.Run) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.InsertCmdbRunParams{
		ID: k.of(run.ID), HubID: k.of(run.HubID), SourceKey: run.Source, ActorID: k.of(run.ActorID),
		StartedAt: timestampOf(run.StartedAt), Counts: cmdbJSON(run.Counts), Decisions: EncodeCmdbDecisions(run.Decisions),
	}
	if k.err != nil {
		return k.err
	}
	if err := q.InsertCmdbRun(ctx, params); err != nil {
		return cmdbFailed("recording a run", err)
	}
	return nil
}

// Runs answers a hub's last runs without their decisions.
func (CmdbRepository) Runs(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Run, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbRuns(ctx, sqlc.ListCmdbRunsParams{HubID: hub, Batch: int32(limit)}) //nolint:gosec // bounded by the caller
	if err != nil {
		return nil, cmdbFailed("listing runs", err)
	}
	out := make([]cmdb.Run, 0, len(rows))
	for _, r := range rows {
		var k cmdbIDs
		run := cmdb.Run{ID: k.of(r.ID), HubID: k.of(r.HubID), Source: r.SourceKey, ActorID: k.of(r.ActorID), StartedAt: timeFrom(r.StartedAt)}
		if err := json.Unmarshal(r.Counts, &run.Counts); err != nil {
			return nil, cmdbFailed("decoding a run", err)
		}
		if k.err != nil {
			return nil, k.err
		}
		out = append(out, run)
	}
	return out, nil
}

// FindRun answers a run with its decisions.
func (CmdbRepository) FindRun(ctx context.Context, id shared.ID) (cmdb.Run, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Run{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Run{}, shared.ErrNotFound.WithDetail("cmdb.run_not_found")
	}
	r, err := q.FindCmdbRun(ctx, key)
	if err != nil {
		return cmdb.Run{}, cmdbRead(err, "cmdb.run_not_found")
	}
	var k cmdbIDs
	run := cmdb.Run{ID: k.of(r.ID), HubID: k.of(r.HubID), Source: r.SourceKey, ActorID: k.of(r.ActorID), StartedAt: timeFrom(r.StartedAt)}
	if err := json.Unmarshal(r.Counts, &run.Counts); err != nil {
		return cmdb.Run{}, cmdbFailed("decoding a run", err)
	}
	if run.Decisions, err = decodeCmdbDecisions(r.Decisions); err != nil {
		return cmdb.Run{}, cmdbFailed("decoding a run", err)
	}
	return run, k.err
}

// EncodeCmdbItem is a held item as the column stores it.
func EncodeCmdbItem(item cmdb.Item) []byte {
	return cmdbJSON(itemDoc{item.Ref, item.Class, item.Name, item.NativeKey, item.Attributes})
}

func cmdbHoldFrom(r sqlc.FindCmdbHoldRow) (cmdb.Hold, error) {
	var doc itemDoc
	if err := json.Unmarshal(r.Item, &doc); err != nil {
		return cmdb.Hold{}, cmdbFailed("decoding a hold", err)
	}
	var k cmdbIDs
	h := cmdb.Hold{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Run: k.of(r.RunID), Source: r.SourceKey, Reason: r.Reason,
		Item:      cmdb.Item{Ref: doc.Ref, Class: doc.Class, Name: doc.Name, NativeKey: doc.NativeKey, Attributes: doc.Attributes},
		CreatedAt: timeFrom(r.CreatedAt), ResolvedAt: optionalTime(r.ResolvedAt), ResolvedBy: k.of(r.ResolvedBy),
	}
	return h, k.err
}

// InsertHold keeps an item for review.
func (CmdbRepository) InsertHold(ctx context.Context, h cmdb.Hold) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.InsertCmdbHoldParams{
		ID: k.of(h.ID), HubID: k.of(h.HubID), RunID: k.of(h.Run), SourceKey: h.Source, Item: EncodeCmdbItem(h.Item),
		Reason: h.Reason, At: timestampOf(h.CreatedAt),
	}
	if k.err != nil {
		return k.err
	}
	if err := q.InsertCmdbHold(ctx, params); err != nil {
		return cmdbFailed("holding an item", err)
	}
	return nil
}

// Holds answers a hub's open holds.
func (CmdbRepository) Holds(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Hold, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbHolds(ctx, sqlc.ListCmdbHoldsParams{HubID: hub, Batch: int32(limit)}) //nolint:gosec // bounded by the caller
	if err != nil {
		return nil, cmdbFailed("listing holds", err)
	}
	out := make([]cmdb.Hold, 0, len(rows))
	for _, r := range rows {
		h, err := cmdbHoldFrom(sqlc.FindCmdbHoldRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// FindHold answers a hold.
func (CmdbRepository) FindHold(ctx context.Context, id shared.ID) (cmdb.Hold, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Hold{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Hold{}, shared.ErrNotFound.WithDetail("cmdb.hold_not_found")
	}
	r, err := q.FindCmdbHold(ctx, key)
	if err != nil {
		return cmdb.Hold{}, cmdbRead(err, "cmdb.hold_not_found")
	}
	return cmdbHoldFrom(r)
}

// ResolveHold closes a hold.
func (CmdbRepository) ResolveHold(ctx context.Context, id shared.ID, at time.Time, by shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.ResolveCmdbHoldParams{ID: k.of(id), At: timestampOf(at), By: k.of(by)}
	if k.err != nil {
		return k.err
	}
	rows, err := q.ResolveCmdbHold(ctx, params)
	if err != nil {
		return cmdbFailed("resolving a hold", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("cmdb.hold_not_found")
	}
	return nil
}

func cmdbDuplicateFrom(r sqlc.FindCmdbDuplicateRow) (cmdb.Duplicate, error) {
	var k cmdbIDs
	d := cmdb.Duplicate{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Rule: r.Rule, CreatedAt: timeFrom(r.CreatedAt),
		ResolvedAt: optionalTime(r.ResolvedAt), ResolvedBy: k.of(r.ResolvedBy), Survivor: k.of(r.SurvivorID),
	}
	for _, c := range r.CiIds {
		d.CIs = append(d.CIs, k.of(c))
	}
	return d, k.err
}

// OpenDuplicate opens a duplicate unless one is open for the same CIs.
func (CmdbRepository) OpenDuplicate(ctx context.Context, d cmdb.Duplicate) (bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	list, err := uuidsOf(d.CIs)
	if err != nil {
		return false, err
	}
	var k cmdbKeys
	params := sqlc.OpenCmdbDuplicateParams{ID: k.of(d.ID), HubID: k.of(d.HubID), CiIds: list, Rule: d.Rule, At: timestampOf(d.CreatedAt)}
	if k.err != nil {
		return false, k.err
	}
	rows, err := q.OpenCmdbDuplicate(ctx, params)
	if err != nil {
		return false, cmdbFailed("opening a duplicate", err)
	}
	return rows > 0, nil
}

// Duplicates answers a hub's open duplicates.
func (CmdbRepository) Duplicates(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Duplicate, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCmdbDuplicates(ctx, sqlc.ListCmdbDuplicatesParams{HubID: hub, Batch: int32(limit)}) //nolint:gosec // bounded by the caller
	if err != nil {
		return nil, cmdbFailed("listing duplicates", err)
	}
	out := make([]cmdb.Duplicate, 0, len(rows))
	for _, r := range rows {
		d, err := cmdbDuplicateFrom(sqlc.FindCmdbDuplicateRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// FindDuplicate answers a duplicate.
func (CmdbRepository) FindDuplicate(ctx context.Context, id shared.ID) (cmdb.Duplicate, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Duplicate{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Duplicate{}, shared.ErrNotFound.WithDetail("cmdb.duplicate_not_found")
	}
	r, err := q.FindCmdbDuplicate(ctx, key)
	if err != nil {
		return cmdb.Duplicate{}, cmdbRead(err, "cmdb.duplicate_not_found")
	}
	return cmdbDuplicateFrom(r)
}

// ResolveDuplicate closes a duplicate, with its survivor when merged.
func (CmdbRepository) ResolveDuplicate(ctx context.Context, id shared.ID, at time.Time, by, survivor shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.ResolveCmdbDuplicateParams{ID: k.of(id), At: timestampOf(at), By: k.of(by), SurvivorID: k.opt(survivor)}
	if k.err != nil {
		return k.err
	}
	rows, err := q.ResolveCmdbDuplicate(ctx, params)
	if err != nil {
		return cmdbFailed("resolving a duplicate", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("cmdb.duplicate_not_found")
	}
	return nil
}
