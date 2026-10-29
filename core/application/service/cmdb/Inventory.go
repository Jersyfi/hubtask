// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package cmdb is the assets and configuration module's use cases (M6): the workspace's classes,
// relation types and sources; a hub's configuration items with their relations and compliance
// fields; the reconciliation every source writes through - people, programs and files alike -;
// duplicates and holds; health and impact.
//
// A CI is judged at its hub, as a page is: READ to read, WRITE_ITEMS to write, so the hub's
// memberships decide and a private hub's inventory is its owner's alone (ADR-0073). Classes,
// relation types and sources are workspace configuration: STRUCTURE to define them. Every write
// of a CI - a person's, a payload's, a file's - goes through Inventory.reconcile or Inventory.save,
// which identify, apply the precedence and keep the identifier index (research itsm-cmdb-ai R1).
package cmdb

import (
	"context"
	"slices"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes: a CI is read and written with the entries' scopes, the configuration with the
// workspace's.
const (
	cisRead     = "items:read"
	cisWrite    = "items:write"
	configWrite = "workspace:manage"
)

const (
	ciTarget       = "cmdb_ci"
	classTarget    = "cmdb_class"
	relTypeTarget  = "cmdb_relation_type"
	sourceTarget   = "cmdb_source"
	relationTarget = "cmdb_relation"
	runTarget      = "cmdb_run"
)

// The stable audit codes (audit.md §2). A CI's name is content and goes to the trail only as a
// fingerprint; attribute values never go (rule 10).
const (
	CIReadAction              audit.Action = "cmdb.ci_read"
	CICreatedAction           audit.Action = "cmdb.ci_created"
	CIChangedAction           audit.Action = "cmdb.ci_changed"
	CIMergedAction            audit.Action = "cmdb.ci_merged"
	RelatedAction             audit.Action = "cmdb.related"
	UnrelatedAction           audit.Action = "cmdb.unrelated"
	ReconciledAction          audit.Action = "cmdb.reconciled"
	HoldDismissedAction       audit.Action = "cmdb.hold_dismissed"
	DuplicateResolvedAction   audit.Action = "cmdb.duplicate_resolved"
	ClassDefinedAction        audit.Action = "cmdb.class_defined"
	RelationTypeDefinedAction audit.Action = "cmdb.relation_type_defined"
	SourceDefinedAction       audit.Action = "cmdb.source_defined"
	ConfigReadAction          audit.Action = "cmdb.configuration_read"
)

const cisExempt = "a configuration item is not an entry; its history is the audit trail and its provenance."

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Inventory is what every CMDB use case shares.
type Inventory struct {
	Store      repository.Store
	Containers workrepo.Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Changes and HLC tell offline devices of a CI or a relation written, so that a hub's
	// inventory can be read without the server (offline-sync.md §13). A nil Changes records
	// nothing, a test's convenience.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

// The change log's entities: a CI and a relation, each filed under its hub.
const (
	CIEntity       = "cmdb_ci"
	RelationEntity = "cmdb_relation"
)

// announce records each CI as it now stands, filed under its hub. The protection it inherits is
// left out: it is a walk over the hub's relations, which a device computes from its own copy.
func (s Inventory) announce(ctx context.Context, actor appshared.ActorContext, cis ...domain.CI) error {
	if s.Changes == nil {
		return nil
	}
	for _, ci := range cis {
		if err := s.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: CIEntity, EntityID: ci.ID, Op: changelog.Upsert,
			ContainerID: ci.HubID, ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: CIOutput(ci, nil),
		}); err != nil {
			return err
		}
	}
	return nil
}

// announceRelations records relations as they now stand, filed under their hub.
func (s Inventory) announceRelations(ctx context.Context, actor appshared.ActorContext, rs ...domain.Relation) error {
	if s.Changes == nil {
		return nil
	}
	for _, r := range rs {
		if err := s.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: RelationEntity, EntityID: r.ID, Op: changelog.Upsert,
			ContainerID: r.HubID, ActorID: actor.AccountID, HLC: s.HLC.Next(), Payload: RelationOutput(r),
		}); err != nil {
			return err
		}
	}
	return nil
}

// announceUnrelated records relations removed: a tombstone names the relation and its hub only.
func (s Inventory) announceUnrelated(ctx context.Context, actor appshared.ActorContext, rs ...domain.Relation) error {
	if s.Changes == nil {
		return nil
	}
	for _, r := range rs {
		if err := s.Changes.Record(ctx, changelog.Change{
			TenantID: actor.TenantID, Entity: RelationEntity, EntityID: r.ID, Op: changelog.Delete,
			ContainerID: r.HubID, ActorID: actor.AccountID, HLC: s.HLC.Next(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func hubPath(hubID shared.ID) []identity.Scope {
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
}

func (s Inventory) authorizeHub(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, write bool, action audit.Action, target string, id shared.ID) error {
	permission, scope := service.PermissionRead, cisRead
	if write {
		permission, scope = service.PermissionWriteItems, cisWrite
	}
	return s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: hubPath(hubID), Action: action, TokenScope: scope, TargetType: target, TargetID: id,
	})
}

func (s Inventory) authorizeConfig(ctx context.Context, actor appshared.ActorContext, write bool, action audit.Action, target string) error {
	permission, scope := service.PermissionRead, cisRead
	if write {
		permission, scope = service.PermissionStructure, configWrite
	}
	return s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: []identity.Scope{identity.TenantScope()}, Action: action, TokenScope: scope,
		TargetType: target, TargetID: actor.TenantID,
	})
}

// hub refuses anything that is not a living hub.
func (s Inventory) hub(ctx context.Context, hubID shared.ID) error {
	container, err := s.Containers.Find(ctx, hubID)
	if err != nil {
		return err
	}
	if container.Type != work.ContainerHub || container.DeletedAt != nil {
		return shared.ErrNotFound.WithDetail("cmdb.hub_not_found")
	}
	return nil
}

// registry is the workspace's configuration as one read.
type registry struct {
	classes  domain.Classes
	types    domain.RelationTypes
	sources  map[string]domain.Source
	ranks    domain.Precedence
	ownClass []domain.Class
	ownTypes []domain.RelationType
}

func (s Inventory) registry(ctx context.Context) (registry, error) {
	classes, err := s.Store.Classes(ctx)
	if err != nil {
		return registry{}, err
	}
	types, err := s.Store.RelationTypes(ctx)
	if err != nil {
		return registry{}, err
	}
	sources, err := s.Store.Sources(ctx)
	if err != nil {
		return registry{}, err
	}
	r := registry{
		classes: domain.NewClasses(classes), types: domain.NewRelationTypes(types),
		sources: map[string]domain.Source{}, ranks: domain.Precedence{}, ownClass: classes, ownTypes: types,
	}
	for _, src := range append(domain.SystemSources(), sources...) {
		r.sources[src.Key], r.ranks[src.Key] = src, src.Rank
	}
	return r, nil
}

// placeOf reads a CI's hub before the permission is asked, outside the write's transaction.
func (s Inventory) placeOf(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.CI, error) {
	var ci domain.CI
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		ci, err = s.Store.FindCI(ctx, id, false)
		return err
	})
	return ci, err
}

func (s Inventory) entry(actor appshared.ActorContext, action audit.Action, target string, id shared.ID, changes ...audit.Change) audit.Entry {
	return audit.Entry{
		TenantID: actor.TenantID, OccurredAt: s.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Changes: audit.Changes(changes...),
	}
}

func (s Inventory) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID, changes ...audit.Change) error {
	e := s.entry(actor, action, target, id, changes...)
	e.Context = audit.Context{RequestID: correlation.RequestIDFrom(ctx)}
	return s.Audit.Append(ctx, e)
}

// recordCI writes a CI's change by the names of what changed: the name as a fingerprint, the
// attributes by key only (rule 10).
func (s Inventory) recordCI(ctx context.Context, actor appshared.ActorContext, action audit.Action, ci domain.CI, fields []string) error {
	changes := make([]audit.Change, 0, len(fields))
	for _, f := range fields {
		if f == domain.FieldName {
			changes = append(changes, audit.Change{Field: f, Classification: audit.Sensitive, To: ci.Name})
			continue
		}
		changes = append(changes, audit.Change{Field: f, Classification: audit.Open})
	}
	return s.record(ctx, actor, action, ciTarget, ci.ID, changes...)
}

func descriptor(d usecase.Descriptor) usecase.Descriptor {
	d.Module = module.Assets
	if d.Activity.Exempt == "" && d.Activity.Verb == "" {
		d.Activity = usecase.ActivityDeclaration{Exempt: cisExempt}
	}
	return d
}

func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

func levelOrNil(l domain.Level) any {
	if l == domain.LevelNone {
		return nil
	}
	return string(l)
}

func protectionOutput(p domain.Protection) usecase.Output {
	return usecase.Output{"c": levelOrNil(p.C), "i": levelOrNil(p.I), "a": levelOrNil(p.A)}
}

// CIOutput is the shape every door returns (api/openapi.yaml, schema ConfigurationItem). The
// inherited protection is given when it was computed.
func CIOutput(ci domain.CI, effective *domain.Protection) usecase.Output {
	attrs := ci.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	prov := map[string]any{}
	for k, p := range ci.Provenance {
		prov[k] = usecase.Output{"source": p.Source, "run_id": idOrNil(p.Run), "at": p.At}
	}
	own := protectionOutput(ci.Compliance.Protection.Protection)
	own["fixed"] = append([]string{}, ci.Compliance.Protection.Fixed...)
	own["reason"] = ci.Compliance.Protection.Reason
	var review any
	if ci.Compliance.ReviewDue != nil {
		review = ci.Compliance.ReviewDue.Format("2006-01-02")
	}
	var lastSeen any
	if ci.LastSeen != nil {
		lastSeen = *ci.LastSeen
	}
	out := usecase.Output{
		"id": ci.ID.String(), "hub_id": ci.HubID.String(), "class": ci.Class, "name": ci.Name,
		"stage": string(ci.Stage), "status": string(ci.Status), "attributes": attrs, "provenance": prov,
		"owner_account_id": idOrNil(ci.Compliance.OwnerAccount), "owner_group_id": idOrNil(ci.Compliance.OwnerGroup),
		"location_id": idOrNil(ci.Compliance.Location), "protection": own, "review_due": review,
		"quantity": ci.Compliance.Quantity, "last_seen_at": lastSeen, "merged_into": idOrNil(ci.MergedInto),
		"created_by": ci.CreatedBy.String(), "created_at": ci.CreatedAt,
		"updated_by": ci.UpdatedBy.String(), "updated_at": ci.UpdatedAt, "version": ci.Version,
	}
	if effective != nil {
		out["effective_protection"] = protectionOutput(*effective)
	}
	return out
}

// RelationOutput is a relation as every door returns it (schema CiRelation).
func RelationOutput(r domain.Relation) usecase.Output {
	return usecase.Output{
		"id": r.ID.String(), "type": r.Type, "source_ci_id": r.SourceCI.String(), "target_ci_id": r.TargetCI.String(),
		"data_source": r.DataSource, "first_seen": r.FirstSeen, "last_seen": r.LastSeen,
	}
}

// DecisionOutput explains one item (schema CiDecision).
func DecisionOutput(d domain.Decision) usecase.Output {
	kept := make([]usecase.Output, 0, len(d.Kept))
	for _, k := range d.Kept {
		kept = append(kept, usecase.Output{"attribute": k.Attribute, "by": k.By})
	}
	candidates := make([]string, 0, len(d.Candidates))
	for _, c := range d.Candidates {
		candidates = append(candidates, c.String())
	}
	return usecase.Output{
		"ref": d.Ref, "outcome": string(d.Outcome), "ci_id": idOrNil(d.CI), "class": d.Class, "rule": d.Rule,
		"taken": append([]string{}, d.Taken...), "kept": kept, "candidates": candidates, "reason": d.Reason,
	}
}

// CountsOutput is a run's figures (schema CiRunCounts).
func CountsOutput(c domain.Counts) usecase.Output {
	return usecase.Output{
		"items": c.Items, "created": c.Created, "updated": c.Updated, "unchanged": c.Unchanged, "held": c.Held,
		"duplicates": c.Duplicates, "refused": c.Refused, "relations_added": c.RelationsAdded,
		"relations_seen": c.RelationsSeen, "relations_refused": c.RelationsRefused,
	}
}

func limitOf(in usecase.Input, fallback, maximum int) int {
	n := in.Int("limit")
	if n <= 0 {
		return fallback
	}
	return min(n, maximum)
}

func sortedIDs(ids []shared.ID) []shared.ID {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

func itoa(n int) string { return strconv.Itoa(n) }
