// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package cmdb is the assets and configuration module's domain (M6): configuration item classes
// with single inheritance, typed relations with an impact direction, the service model, the
// compliance fields, the reconciliation every source writes through, health and impact.
//
// A configuration item (CI) sits in a hub, as a page does: the hub is the inventory's space and
// its memberships decide who reads and writes it. Classes, relation types and sources are the
// workspace's configuration. Hubtask ships a class hierarchy and a relation catalogue declared
// here and never stored; a workspace's own rows add classes beneath them.
package cmdb

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// The bounds of the configuration a workspace writes.
const (
	MaxClasses          = 500
	MaxAttributes       = 100
	MaxIdentifierRules  = 10
	MaxClassDepth       = 10
	MaxNameLength       = 200
	DefaultStalenessDay = 60
)

// AttributeKind is the shape of a class attribute's value.
type AttributeKind string

const (
	AttrText    AttributeKind = "TEXT"
	AttrNumber  AttributeKind = "NUMBER"
	AttrBoolean AttributeKind = "BOOLEAN"
	// AttrDate is a calendar day, `YYYY-MM-DD`.
	AttrDate AttributeKind = "DATE"
	// AttrChoice is one of the attribute's options.
	AttrChoice AttributeKind = "CHOICE"
	AttrURL    AttributeKind = "URL"
	// AttrGroup and AttrAccount name a group or a person of the workspace by identifier - an
	// offering's support and approval groups (the service model).
	AttrGroup   AttributeKind = "GROUP"
	AttrAccount AttributeKind = "ACCOUNT"
)

var attributeKinds = []AttributeKind{AttrText, AttrNumber, AttrBoolean, AttrDate, AttrChoice, AttrURL, AttrGroup, AttrAccount}

// Need says how much an attribute counts for a CI's completeness (health).
type Need string

const (
	NeedOptional    Need = "OPTIONAL"
	NeedRecommended Need = "RECOMMENDED"
	NeedRequired    Need = "REQUIRED"
)

// Attribute is one typed attribute a class declares; its descendants inherit it.
type Attribute struct {
	Key  string
	Kind AttributeKind
	Need Need
	// Identifier marks a value that identifies a CI: it is kept in the identifier index the
	// reconciliation looks up, normalised.
	Identifier bool
	Options    []string
}

// IdentifierRule is a set of attributes that together are unique among a class's CIs. A class's
// rules are tried in order, its own before those it inherits; `name` is the CI's name.
type IdentifierRule struct {
	Attributes []string
	// Class is the class that declared the rule: a match is looked for among its CIs and those of
	// its descendants. Set when a class is resolved.
	Class string
}

// RequiredRelation is a relation a CI of the class is an orphan without: an application service
// with nothing it runs on (health). Any of the types, in the direction given, counts.
type RequiredRelation struct {
	Types []string
	// Outgoing is true when the CI is the relation's source.
	Outgoing bool
}

// Class is a kind of configuration item.
type Class struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	// Parent is the class it inherits from; empty only for the root `ci`.
	Parent string
	// Name is empty on a shipped class: a client shows its own word for the key (rule 8).
	Name string
	Icon string
	// Abstract classes hold no CI of their own; they gather attributes and rules.
	Abstract   bool
	Attributes []Attribute
	Rules      []IdentifierRule
	Required   []RequiredRelation
	// StalenessDays is how long a CI may go unconfirmed by any source before it is stale; zero
	// inherits.
	StalenessDays int
	System        bool
	ArchivedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// RootClass is the key every class descends from.
const RootClass = "ci"

func attr(key string, kind AttributeKind, need Need, identifier bool, options ...string) Attribute {
	return Attribute{Key: key, Kind: kind, Need: need, Identifier: identifier, Options: options}
}

func rules(sets ...[]string) []IdentifierRule {
	out := make([]IdentifierRule, 0, len(sets))
	for _, set := range sets {
		out = append(out, IdentifierRule{Attributes: set})
	}
	return out
}

// SystemClasses is the hierarchy Hubtask ships, in the spirit of a common service data model in
// its own names: hardware, virtual and cloud resources, software, the service model (business and
// technical services with their offerings, application services), locations, contracts, business
// processes and information objects (research itsm-cmdb-ai §6.1, D1, D3).
func SystemClasses() []Class {
	host := []Attribute{
		attr("hostname", AttrText, NeedRecommended, true), attr("domain", AttrText, NeedOptional, false),
		attr("ip_address", AttrText, NeedOptional, true), attr("mac_address", AttrText, NeedOptional, true),
		attr("operating_system", AttrText, NeedRecommended, false),
	}
	offering := []Attribute{
		attr("service_level", AttrText, NeedRecommended, false),
		attr("support_group", AttrGroup, NeedRequired, false),
		attr("approval_group", AttrGroup, NeedOptional, false),
	}
	cs := []Class{
		{Key: RootClass, Abstract: true, Icon: "box", StalenessDays: DefaultStalenessDay},
		{Key: "hardware", Parent: RootClass, Abstract: true, Icon: "cpu", Attributes: []Attribute{
			attr("manufacturer", AttrText, NeedRecommended, true), attr("model", AttrText, NeedRecommended, false),
			attr("serial_number", AttrText, NeedRecommended, true), attr("asset_tag", AttrText, NeedOptional, true),
		}, Rules: rules([]string{"serial_number", "manufacturer"}, []string{"asset_tag"})},
		{Key: "computer", Parent: "hardware", Abstract: true, Icon: "monitor", Attributes: host,
			Rules: rules([]string{"mac_address"}, []string{"hostname", "domain"})},
		{Key: "server", Parent: "computer", Icon: "server"},
		{Key: "workstation", Parent: "computer", Icon: "monitor"},
		{Key: "laptop", Parent: "computer", Icon: "laptop"},
		{Key: "network_device", Parent: "hardware", Icon: "router", Attributes: host,
			Rules: rules([]string{"mac_address"}, []string{"hostname"})},
		{Key: "storage_device", Parent: "hardware", Icon: "hard-drive"},
		{Key: "virtual_machine", Parent: RootClass, Icon: "layers", Attributes: append([]Attribute{
			attr("instance_id", AttrText, NeedOptional, true)}, host...),
			Rules:    rules([]string{"instance_id"}, []string{"hostname", "domain"}),
			Required: []RequiredRelation{{Types: []string{RelHostedOn, RelRunsOn}, Outgoing: true}}},
		{Key: "cloud_resource", Parent: RootClass, Icon: "cloud", Attributes: []Attribute{
			attr("provider", AttrText, NeedRequired, true), attr("resource_id", AttrText, NeedRequired, true),
			attr("region", AttrText, NeedRecommended, false),
		}, Rules: rules([]string{"provider", "resource_id"})},
		{Key: "virtualization_cluster", Parent: RootClass, Icon: "server-cog", Attributes: []Attribute{
			attr("cluster_id", AttrText, NeedOptional, true), attr("cluster_type", AttrText, NeedOptional, false),
		}, Rules: rules([]string{"cluster_id"}, []string{"name"})},
		{Key: "container_cluster", Parent: RootClass, Icon: "boxes", Attributes: []Attribute{
			attr("cluster_id", AttrText, NeedOptional, true), attr("distribution", AttrText, NeedOptional, false),
		}, Rules: rules([]string{"cluster_id"}, []string{"name"})},
		{Key: "database_instance", Parent: RootClass, Icon: "database", Attributes: []Attribute{
			attr("engine", AttrText, NeedRecommended, false), attr("version", AttrText, NeedOptional, false),
			attr("instance_id", AttrText, NeedOptional, true),
		}, Rules: rules([]string{"instance_id"}, []string{"name"}),
			Required: []RequiredRelation{{Types: []string{RelRunsOn, RelHostedOn}, Outgoing: true}}},
		{Key: "software", Parent: RootClass, Icon: "package", Attributes: []Attribute{
			attr("publisher", AttrText, NeedRecommended, false), attr("version", AttrText, NeedRecommended, true),
			attr("purl", AttrText, NeedOptional, true), attr("cpe", AttrText, NeedOptional, true),
		}, Rules: rules([]string{"purl"}, []string{"cpe"}, []string{"name", "version"})},
		{Key: "service", Parent: RootClass, Abstract: true, Icon: "workflow", Rules: rules([]string{"name"})},
		{Key: "business_service", Parent: "service", Icon: "briefcase"},
		{Key: "business_offering", Parent: "service", Icon: "tag", Attributes: offering,
			Required: []RequiredRelation{{Types: []string{RelOfferingOf}, Outgoing: true}}},
		{Key: "technical_service", Parent: "service", Icon: "wrench"},
		{Key: "technical_offering", Parent: "service", Icon: "tag", Attributes: offering,
			Required: []RequiredRelation{{Types: []string{RelOfferingOf}, Outgoing: true}}},
		{Key: "application_service", Parent: "service", Icon: "app-window", Attributes: []Attribute{
			attr("url", AttrURL, NeedOptional, false)},
			Required: []RequiredRelation{{Types: []string{RelRunsOn, RelDependsOn}, Outgoing: true}}},
		{Key: "location", Parent: RootClass, Icon: "map-pin", Attributes: []Attribute{
			attr("address", AttrText, NeedRecommended, false)}, Rules: rules([]string{"name"})},
		{Key: "site", Parent: "location", Icon: "map-pin"},
		{Key: "building", Parent: "location", Icon: "building"},
		{Key: "room", Parent: "location", Icon: "door-open"},
		{Key: "contract", Parent: RootClass, Icon: "file-signature", Attributes: []Attribute{
			attr("number", AttrText, NeedRequired, true), attr("supplier", AttrText, NeedRecommended, false),
			attr("ends_on", AttrDate, NeedRecommended, false),
		}, Rules: rules([]string{"number"})},
		{Key: "business_process", Parent: RootClass, Icon: "git-branch", Rules: rules([]string{"name"})},
		{Key: "information_object", Parent: RootClass, Icon: "file-lock", Rules: rules([]string{"name"})},
	}
	for i := range cs {
		cs[i].System = true
	}
	return cs
}

// ClassInput is what a workspace writes for a class of its own.
type ClassInput struct {
	ID            shared.ID
	TenantID      shared.ID
	Key           string
	Parent        string
	Name          string
	Icon          string
	Abstract      bool
	Attributes    []Attribute
	Rules         []IdentifierRule
	Required      []RequiredRelation
	StalenessDays int
	Now           time.Time
}

func invalid(path, code string, params ...string) error {
	err := shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
	if len(params) == 2 {
		err = err.WithParams(map[string]string{params[0]: params[1]})
	}
	return err
}

// NewClass checks a workspace's class against the classes it would join. A shipped class's key
// is taken; its parent must exist and not be archived.
func NewClass(in ClassInput, classes Classes) (Class, error) {
	if !keyPattern.MatchString(in.Key) {
		return Class{}, invalid("/key", "cmdb.class_key_malformed")
	}
	if existing, ok := classes.Find(in.Key); ok && existing.System {
		return Class{}, shared.ErrConflict.WithDetail("cmdb.class_key_taken").WithParams(map[string]string{"key": in.Key})
	}
	c := Class{
		ID: in.ID, TenantID: in.TenantID, Key: in.Key, Parent: in.Parent, Name: strings.TrimSpace(in.Name),
		Icon: in.Icon, Abstract: in.Abstract, Attributes: in.Attributes, Rules: in.Rules, Required: in.Required,
		StalenessDays: in.StalenessDays, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}
	return c, c.check(classes)
}

// Changed is the class with a workspace's new definition; its key and system mark stay.
func (c Class) Changed(in ClassInput, classes Classes) (Class, error) {
	next := c
	next.Parent, next.Name, next.Icon, next.Abstract = in.Parent, strings.TrimSpace(in.Name), in.Icon, in.Abstract
	next.Attributes, next.Rules, next.Required, next.StalenessDays = in.Attributes, in.Rules, in.Required, in.StalenessDays
	next.UpdatedAt = in.Now
	next.Version++
	return next, next.check(classes)
}

func (c Class) check(classes Classes) error {
	if c.Name == "" || len([]rune(c.Name)) > 120 || strings.ContainsAny(c.Name, "\n\r") {
		return invalid("/name", "cmdb.class_name_invalid")
	}
	parent, ok := classes.Find(c.Parent)
	if !ok || parent.ArchivedAt != nil {
		return invalid("/parent", "cmdb.class_parent_unknown", "key", c.Parent)
	}
	if slices.Contains(classes.Ancestry(c.Parent), c.Key) {
		return invalid("/parent", "cmdb.class_cycle")
	}
	if len(classes.Ancestry(c.Parent)) >= MaxClassDepth {
		return invalid("/parent", "cmdb.class_too_deep")
	}
	if c.StalenessDays < 0 || c.StalenessDays > 3650 {
		return invalid("/staleness_days", "cmdb.class_staleness_invalid")
	}
	if len(c.Attributes) > MaxAttributes {
		return invalid("/attributes", "cmdb.class_too_many_attributes")
	}
	inherited := classes.Resolve(c.Parent)
	seen := map[string]bool{"name": true}
	for _, a := range inherited.Attributes {
		seen[a.Key] = true
	}
	for i, a := range c.Attributes {
		path := "/attributes/" + strconv.Itoa(i)
		if !keyPattern.MatchString(a.Key) || seen[a.Key] {
			return invalid(path+"/key", "cmdb.attribute_key_invalid", "key", a.Key)
		}
		seen[a.Key] = true
		if !slices.Contains(attributeKinds, a.Kind) {
			return invalid(path+"/kind", "cmdb.attribute_kind_unknown")
		}
		if a.Need != NeedOptional && a.Need != NeedRecommended && a.Need != NeedRequired {
			return invalid(path+"/need", "cmdb.attribute_need_unknown")
		}
		if (a.Kind == AttrChoice) != (len(a.Options) > 0) || len(a.Options) > 100 {
			return invalid(path+"/options", "cmdb.attribute_options_invalid")
		}
	}
	if len(c.Rules) > MaxIdentifierRules {
		return invalid("/identifier_rules", "cmdb.class_too_many_rules")
	}
	for i, r := range c.Rules {
		if len(r.Attributes) == 0 || len(r.Attributes) > 5 {
			return invalid("/identifier_rules/"+strconv.Itoa(i), "cmdb.rule_invalid")
		}
		for _, key := range r.Attributes {
			if !seen[key] {
				return invalid("/identifier_rules/"+strconv.Itoa(i), "cmdb.rule_attribute_unknown", "key", key)
			}
		}
	}
	for i, r := range c.Required {
		if len(r.Types) == 0 {
			return invalid("/required_relations/"+strconv.Itoa(i), "cmdb.required_relation_invalid")
		}
	}
	return nil
}

// Classes is a workspace's class registry: the shipped hierarchy with its own rows over it.
type Classes struct{ byKey map[string]Class }

// NewClasses lays the workspace's rows over the shipped hierarchy. A row with a shipped key is
// ignored: a shipped class is not changed by a workspace.
func NewClasses(own []Class) Classes {
	r := Classes{byKey: map[string]Class{}}
	for _, c := range SystemClasses() {
		r.byKey[c.Key] = c
	}
	for _, c := range own {
		if existing, ok := r.byKey[c.Key]; ok && existing.System {
			continue
		}
		r.byKey[c.Key] = c
	}
	return r
}

// Find answers a class by key.
func (r Classes) Find(key string) (Class, bool) {
	c, ok := r.byKey[key]
	return c, ok
}

// All answers every class, shipped first, each in key order.
func (r Classes) All() []Class {
	out := make([]Class, 0, len(r.byKey))
	for _, c := range r.byKey {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Class) int {
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

// Ancestry answers the key and its ancestors, nearest first, up to the root.
func (r Classes) Ancestry(key string) []string {
	var out []string
	for key != "" && len(out) <= MaxClassDepth+1 {
		c, ok := r.byKey[key]
		if !ok || slices.Contains(out, key) {
			break
		}
		out = append(out, key)
		key = c.Parent
	}
	return out
}

// IsA reports whether a class is the other or descends from it.
func (r Classes) IsA(key, ancestor string) bool { return slices.Contains(r.Ancestry(key), ancestor) }

// Resolved is a class with what it inherits: every attribute, its rules nearest first, its orphan
// rules and its staleness.
type Resolved struct {
	Class
	Lineage []string
}

// Resolve answers the class with its inheritance folded in. An unknown key resolves to nothing.
func (r Classes) Resolve(key string) Resolved {
	lineage := r.Ancestry(key)
	out := Resolved{Lineage: lineage}
	if len(lineage) == 0 {
		return out
	}
	out.Class = r.byKey[key]
	out.Attributes, out.Rules, out.Required = nil, nil, nil
	out.StalenessDays = 0
	for i := len(lineage) - 1; i >= 0; i-- {
		c := r.byKey[lineage[i]]
		out.Attributes = append(out.Attributes, c.Attributes...)
		out.Required = append(out.Required, c.Required...)
		if c.StalenessDays > 0 {
			out.StalenessDays = c.StalenessDays
		}
	}
	for _, k := range lineage {
		for _, rule := range r.byKey[k].Rules {
			out.Rules = append(out.Rules, IdentifierRule{Attributes: rule.Attributes, Class: k})
		}
	}
	if out.StalenessDays == 0 {
		out.StalenessDays = DefaultStalenessDay
	}
	return out
}

// Attribute answers a resolved class's attribute by key.
func (c Resolved) Attribute(key string) (Attribute, bool) {
	for _, a := range c.Attributes {
		if a.Key == key {
			return a, true
		}
	}
	return Attribute{}, false
}
