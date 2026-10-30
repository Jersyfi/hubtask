// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxGroupsPerHub bounds a hub's CI groups; MaxGroupConditions a group's attribute conditions.
const (
	MaxGroupsPerHub    = 200
	MaxGroupConditions = 10
)

var groupKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Filter is a group's query (research I4): CIs of these classes or below, in these stages, whose
// name holds a text and whose attributes equal the values given. An empty part narrows nothing;
// text compares without case.
type Filter struct {
	Classes    []string
	Stages     []Stage
	NameHas    string
	Attributes map[string]string
}

// Group is a hub's dynamic set of CIs, defined by a filter and never by listing them: "every
// virtual machine whose attribute app is crm". When it names a service, the members are what that
// service rests on beside its relations.
type Group struct {
	ID        shared.ID
	TenantID  shared.ID
	HubID     shared.ID
	Key       string
	Name      string
	Filter    Filter
	ServiceID shared.ID
	UpdatedBy shared.ID
	UpdatedAt time.Time
	Version   int
}

// GroupInput is what a person defines a group with.
type GroupInput struct {
	Key       string
	Name      string
	Filter    Filter
	ServiceID shared.ID
}

// NewGroup checks a definition against the workspace's classes; the service, when named, is
// checked by the caller to be a service CI of the hub.
func NewGroup(id, tenant, hub, by shared.ID, in GroupInput, classes Classes, now time.Time) (Group, error) {
	if !groupKeyPattern.MatchString(in.Key) {
		return Group{}, invalid("/key", "cmdb.group_key_invalid")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > MaxNameLength || strings.ContainsAny(name, "\n\r") {
		return Group{}, invalid("/name", "cmdb.group_name_invalid")
	}
	f := in.Filter
	for _, c := range f.Classes {
		if _, ok := classes.Find(c); !ok {
			return Group{}, invalid("/filter/classes", "cmdb.class_unknown", c)
		}
	}
	for _, s := range f.Stages {
		if !slices.Contains(Stages, s) {
			return Group{}, invalid("/filter/stages", "cmdb.stage_unknown")
		}
	}
	if len(f.Attributes) > MaxGroupConditions {
		return Group{}, invalid("/filter/attributes", "cmdb.group_conditions_too_many")
	}
	for k, v := range f.Attributes {
		if k == "" || len(k) > 64 || len([]rune(v)) > 500 {
			return Group{}, invalid("/filter/attributes", "cmdb.group_condition_invalid")
		}
	}
	if len([]rune(f.NameHas)) > MaxNameLength {
		return Group{}, invalid("/filter/name_has", "cmdb.group_condition_invalid")
	}
	if len(f.Classes) == 0 && len(f.Stages) == 0 && len(f.Attributes) == 0 && strings.TrimSpace(f.NameHas) == "" {
		return Group{}, invalid("/filter", "cmdb.group_filter_empty")
	}
	f.Classes = slices.Compact(slices.Sorted(slices.Values(f.Classes)))
	f.NameHas = strings.TrimSpace(f.NameHas)
	return Group{
		ID: id, TenantID: tenant, HubID: hub, Key: in.Key, Name: name, Filter: f, ServiceID: in.ServiceID,
		UpdatedBy: by, UpdatedAt: now, Version: 1,
	}, nil
}

// Matches says whether a CI belongs. A merged CI never does.
func (f Filter) Matches(ci CI, classes Classes) bool {
	if ci.Status == StatusMerged {
		return false
	}
	if len(f.Classes) > 0 && !slices.ContainsFunc(f.Classes, func(k string) bool { return classes.IsA(ci.Class, k) }) {
		return false
	}
	if len(f.Stages) > 0 && !slices.Contains(f.Stages, ci.Stage) {
		return false
	}
	if f.NameHas != "" && !strings.Contains(strings.ToLower(ci.Name), strings.ToLower(f.NameHas)) {
		return false
	}
	for k, want := range f.Attributes {
		got, ok := ci.Attributes[k]
		if !ok || !strings.EqualFold(strings.TrimSpace(fmt.Sprint(got)), strings.TrimSpace(want)) {
			return false
		}
	}
	return true
}

// Members answers the CIs of a hub's inventory that belong, in the order given.
func (g Group) Members(cis []CI, classes Classes) []CI {
	var out []CI
	for _, ci := range cis {
		if g.Filter.Matches(ci, classes) {
			out = append(out, ci)
		}
	}
	return out
}
