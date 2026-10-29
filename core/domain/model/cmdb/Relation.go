// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Impact says which end of a relation suffers when the other fails. It is what impact analysis
// walks and what protection need is inherited against (BSI 200-2's maximum principle).
type Impact string

const (
	// ImpactNone carries nothing: two things connected are not hurt by each other.
	ImpactNone Impact = "NONE"
	// ImpactSource: the source suffers when the target fails - an application service that runs
	// on a server.
	ImpactSource Impact = "SOURCE"
	// ImpactTarget: the target suffers when the source fails - a room a building contains.
	ImpactTarget Impact = "TARGET"
	// ImpactBoth: either end suffers when the other fails.
	ImpactBoth Impact = "BOTH"
)

// The shipped relation types, read from their source: "A depends on B". Their words both ways
// are the client's, by key (rule 8).
const (
	RelDependsOn  = "depends_on"
	RelRunsOn     = "runs_on"
	RelHostedOn   = "hosted_on"
	RelInstalled  = "installed_on"
	RelContains   = "contains"
	RelConnects   = "connects_to"
	RelMemberOf   = "member_of"
	RelOfferingOf = "offering_of"
)

// ClassPair is one pair of classes a relation type may join; each side matches the class and its
// descendants.
type ClassPair struct {
	From string
	To   string
}

// RelationType is a kind of relation between CIs.
type RelationType struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	// Name and ReverseName are empty on a shipped type (rule 8).
	Name        string
	ReverseName string
	Impact      Impact
	// Pairs are the classes the type may join; empty joins any two.
	Pairs      []ClassPair
	System     bool
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int
}

// SystemRelationTypes is the relation catalogue Hubtask ships (research itsm-cmdb-ai §6.1, D2).
func SystemRelationTypes() []RelationType {
	ts := []RelationType{
		{Key: RelDependsOn, Impact: ImpactSource},
		{Key: RelRunsOn, Impact: ImpactSource, Pairs: []ClassPair{
			{From: "application_service", To: "computer"}, {From: "application_service", To: "virtual_machine"},
			{From: "application_service", To: "cloud_resource"}, {From: "application_service", To: "container_cluster"},
			{From: "database_instance", To: "computer"}, {From: "database_instance", To: "virtual_machine"},
			{From: "database_instance", To: "cloud_resource"}, {From: "database_instance", To: "container_cluster"},
			{From: "virtual_machine", To: "cloud_resource"}, {From: "software", To: "computer"},
			{From: "software", To: "virtual_machine"}, {From: "software", To: "container_cluster"},
		}},
		{Key: RelHostedOn, Impact: ImpactSource, Pairs: []ClassPair{
			{From: "virtual_machine", To: "server"}, {From: "container_cluster", To: "server"},
			{From: "container_cluster", To: "virtual_machine"}, {From: "container_cluster", To: "cloud_resource"},
		}},
		{Key: RelInstalled, Impact: ImpactSource, Pairs: []ClassPair{
			{From: "software", To: "computer"}, {From: "software", To: "virtual_machine"},
		}},
		{Key: RelContains, Impact: ImpactTarget},
		{Key: RelConnects, Impact: ImpactNone},
		{Key: RelMemberOf, Impact: ImpactTarget},
		{Key: RelOfferingOf, Impact: ImpactTarget, Pairs: []ClassPair{
			{From: "business_offering", To: "business_service"}, {From: "technical_offering", To: "technical_service"},
		}},
	}
	for i := range ts {
		ts[i].System = true
	}
	return ts
}

// RelationTypeInput is what a workspace writes for a relation type of its own.
type RelationTypeInput struct {
	ID          shared.ID
	TenantID    shared.ID
	Key         string
	Name        string
	ReverseName string
	Impact      Impact
	Pairs       []ClassPair
	Now         time.Time
}

// NewRelationType checks a workspace's relation type.
func NewRelationType(in RelationTypeInput, types RelationTypes, classes Classes) (RelationType, error) {
	if !keyPattern.MatchString(in.Key) {
		return RelationType{}, invalid("/key", "cmdb.relation_type_key_malformed")
	}
	if existing, ok := types.Find(in.Key); ok && existing.System {
		return RelationType{}, shared.ErrConflict.WithDetail("cmdb.relation_type_key_taken").WithParams(map[string]string{"key": in.Key})
	}
	t := RelationType{ID: in.ID, TenantID: in.TenantID, Key: in.Key, CreatedAt: in.Now, Version: 1}
	return t.Changed(in, classes)
}

// Changed is the type with a new definition.
func (t RelationType) Changed(in RelationTypeInput, classes Classes) (RelationType, error) {
	next := t
	next.Name, next.ReverseName = strings.TrimSpace(in.Name), strings.TrimSpace(in.ReverseName)
	next.Impact, next.Pairs, next.UpdatedAt = in.Impact, in.Pairs, in.Now
	if !t.CreatedAt.Equal(in.Now) {
		next.Version++
	}
	for path, name := range map[string]string{"/name": next.Name, "/reverse_name": next.ReverseName} {
		if name == "" || len([]rune(name)) > 120 || strings.ContainsAny(name, "\n\r") {
			return RelationType{}, invalid(path, "cmdb.relation_type_name_invalid")
		}
	}
	if !slices.Contains([]Impact{ImpactNone, ImpactSource, ImpactTarget, ImpactBoth}, next.Impact) {
		return RelationType{}, invalid("/impact", "cmdb.impact_unknown")
	}
	for i, p := range next.Pairs {
		if _, ok := classes.Find(p.From); !ok {
			return RelationType{}, invalid("/pairs/"+strconv.Itoa(i)+"/from", "cmdb.class_unknown", "key", p.From)
		}
		if _, ok := classes.Find(p.To); !ok {
			return RelationType{}, invalid("/pairs/"+strconv.Itoa(i)+"/to", "cmdb.class_unknown", "key", p.To)
		}
	}
	return next, nil
}

// RelationTypes is a workspace's relation catalogue: the shipped types and its own.
type RelationTypes struct{ byKey map[string]RelationType }

// NewRelationTypes lays the workspace's rows over the shipped catalogue.
func NewRelationTypes(own []RelationType) RelationTypes {
	r := RelationTypes{byKey: map[string]RelationType{}}
	for _, t := range SystemRelationTypes() {
		r.byKey[t.Key] = t
	}
	for _, t := range own {
		if existing, ok := r.byKey[t.Key]; ok && existing.System {
			continue
		}
		r.byKey[t.Key] = t
	}
	return r
}

// Find answers a type by key.
func (r RelationTypes) Find(key string) (RelationType, bool) {
	t, ok := r.byKey[key]
	return t, ok
}

// All answers every type, shipped first.
func (r RelationTypes) All() []RelationType {
	out := make([]RelationType, 0, len(r.byKey))
	for _, t := range r.byKey {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b RelationType) int {
		if a.System != b.System {
			if a.System {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

// Suffering answers the keys whose source suffers and those whose target suffers - the two
// directions an impact walk follows.
func (r RelationTypes) Suffering() (source, target []string) {
	for _, t := range r.All() {
		if t.Impact == ImpactSource || t.Impact == ImpactBoth {
			source = append(source, t.Key)
		}
		if t.Impact == ImpactTarget || t.Impact == ImpactBoth {
			target = append(target, t.Key)
		}
	}
	return source, target
}

// Allows reports whether the type may join a CI of one class to one of another.
func (t RelationType) Allows(from, to string, classes Classes) bool {
	if len(t.Pairs) == 0 {
		return true
	}
	for _, p := range t.Pairs {
		if classes.IsA(from, p.From) && classes.IsA(to, p.To) {
			return true
		}
	}
	return false
}

// Relation joins two CIs of one hub by a type. It carries the source that reported it and when it
// was first and last seen, so that a relation no source confirms any more can be found.
type Relation struct {
	ID         shared.ID
	TenantID   shared.ID
	HubID      shared.ID
	Type       string
	SourceCI   shared.ID
	TargetCI   shared.ID
	DataSource string
	FirstSeen  time.Time
	LastSeen   time.Time
	CreatedBy  shared.ID
}

// CheckRelation refuses a relation the catalogue does not allow between the two CIs.
func CheckRelation(t RelationType, source, target CI, classes Classes) error {
	if source.ID == target.ID {
		return invalid("/target_ci_id", "cmdb.relation_to_itself")
	}
	if source.HubID != target.HubID {
		return invalid("/target_ci_id", "cmdb.relation_across_hubs")
	}
	if t.ArchivedAt != nil {
		return invalid("/type", "cmdb.relation_type_unknown", "key", t.Key)
	}
	if !t.Allows(source.Class, target.Class, classes) {
		return invalid("/type", "cmdb.relation_not_allowed", "key", t.Key)
	}
	return nil
}
