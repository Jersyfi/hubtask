// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The reads the triage's routing makes, as the agent.
const (
	listEntryCisName   = "ListEntryCis"
	readCiImpactName   = "ReadCiImpact"
	getGroupName       = "GetGroup"
	searchItemsName    = "SearchItems"
	assignWorkItemName = "AssignWorkItem"

	// supportGroupAttribute is the offering's group that answers its tickets (cmdb SystemClasses).
	supportGroupAttribute = "support_group"

	// The routing's bounds: the CIs a ticket names that it follows, the services one CI's impact
	// may name, the members of a group it weighs, and the open entries it counts of one member - a
	// member carrying more is as loaded as one carrying that many.
	maxNamedCis       = 20
	maxReachedOffered = 50
	maxGroupMembers   = 50
	maxOpenCounted    = 1000
)

// route proposes, for a ticket nobody holds, AssignWorkItem to the member of its offering's
// support group with the fewest open entries in the hub (accepted.md, second answers 10). The
// offering is the one the ticket names, else the nearest one a CI it names affects - none where two
// are equally near, and none where it names two. A group holds no ticket, and skills are not
// weighed. A read the agent may not make leaves the ticket unrouted; it does not fail the job.
func (w *jobWork) route(ctx context.Context, item shared.ID, read ticketRead) error {
	if read.hub.IsZero() || read.entry.String("assignee_id") != "" || read.entry["is_completed"] == true {
		return nil
	}
	named, err := w.namedCis(ctx, item, read.named)
	if err != nil {
		return softened(err)
	}
	offering, err := w.offeringOf(ctx, named)
	if err != nil || offering == "" {
		return softened(err)
	}
	group, err := w.invoke(ctx, getGroupName, usecase.Input{"group_id": offering})
	if err != nil {
		return softened(err)
	}
	members, _ := group["members"].([]string)
	if len(members) > maxGroupMembers {
		members = members[:maxGroupMembers]
	}
	chosen, fewest := "", maxOpenCounted+1
	for _, member := range members {
		open, err := w.openEntries(ctx, read.hub, member)
		if err != nil {
			return softened(err)
		}
		if open < fewest {
			chosen, fewest = member, open
		}
	}
	if chosen == "" {
		return nil
	}
	return w.propose(ctx, assignWorkItemName, usecase.Input{"item_id": item.String(), "account_id": chosen})
}

// softened passes over a read the agent may not make, or of something gone; a halt, a spent
// budget or a failure ends the job as any other act's would.
func softened(err error) error {
	if err == nil || errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) ||
		errors.Is(err, shared.ErrValidation) {
		return nil
	}
	return err
}

// namedCis answers the CIs the ticket names: those it names as affected, then those its words
// name, each once, at most maxNamedCis.
func (w *jobWork) namedCis(ctx context.Context, item shared.ID, byWords []string) ([]string, error) {
	out, err := w.invoke(ctx, listEntryCisName, usecase.Input{"item_id": item.String()})
	if err != nil {
		return nil, err
	}
	var named []string
	for _, ci := range outputs(out["data"]) {
		if id := ci.String("ci_id"); id != "" && !slices.Contains(named, id) {
			named = append(named, id)
		}
	}
	for _, id := range byWords {
		if !slices.Contains(named, id) {
			named = append(named, id)
		}
	}
	if len(named) > maxNamedCis {
		named = named[:maxNamedCis]
	}
	return named, nil
}

// offeringOf answers the support group of the ticket's offering: the one offering among the CIs it
// names, else the nearest offering the impact of a named CI reaches. Two named, or two equally
// near, leave it unrouted: the triage does not guess between two groups.
func (w *jobWork) offeringOf(ctx context.Context, named []string) (string, error) {
	var groups []string
	for _, id := range named {
		group, err := w.supportGroup(ctx, id)
		if err != nil {
			return "", err
		}
		if group != "" && !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	switch len(groups) {
	case 1:
		return groups[0], nil
	case 0:
	default:
		return "", nil
	}
	depth := map[string]int{}
	for _, id := range named {
		out, err := w.invoke(ctx, readCiImpactName, usecase.Input{"ci_id": id, "direction": "DOWNSTREAM"})
		if err != nil {
			return "", err
		}
		services, _ := out["services"].([]string)
		for _, row := range outputs(out["reached"]) {
			reached := row.String("ci_id")
			if !slices.Contains(services, reached) || slices.Contains(named, reached) {
				continue
			}
			d := depthOf(row["depth"])
			if held, seen := depth[reached]; !seen || d < held {
				depth[reached] = d
			}
		}
	}
	order := make([]string, 0, len(depth))
	for id := range depth {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		if depth[order[i]] != depth[order[j]] {
			return depth[order[i]] < depth[order[j]]
		}
		return order[i] < order[j]
	})
	if len(order) > maxReachedOffered {
		order = order[:maxReachedOffered]
	}
	found, at := "", -1
	for _, id := range order {
		if at >= 0 && depth[id] > at {
			break
		}
		group, err := w.supportGroup(ctx, id)
		if err != nil {
			return "", err
		}
		if group == "" {
			continue
		}
		if at >= 0 && group != found {
			return "", nil
		}
		found, at = group, depth[id]
	}
	return found, nil
}

// supportGroup answers the group an offering names to support it; empty for any other CI.
func (w *jobWork) supportGroup(ctx context.Context, ci string) (string, error) {
	out, err := w.invoke(ctx, readCiName, usecase.Input{"ci_id": ci})
	if err != nil {
		return "", err
	}
	attributes, _ := out["attributes"].(map[string]any)
	group, _ := attributes[supportGroupAttribute].(string)
	if _, err := shared.ParseID(group); err != nil {
		return "", nil
	}
	return group, nil
}

// openEntries counts the open entries a member carries in the hub, as the agent may see them, up
// to maxOpenCounted.
func (w *jobWork) openEntries(ctx context.Context, hub shared.ID, member string) (int, error) {
	filter := map[string]any{"op": "AND", "nodes": []any{
		map[string]any{"field": "assignee_id", "op": "EQ", "value": member},
		map[string]any{"field": "is_completed", "op": "EQ", "value": false},
	}}
	count, cursor := 0, ""
	for count < maxOpenCounted {
		in := usecase.Input{"container_id": hub.String(), "mode": "LEXICAL", "filter": filter, "size": 200}
		if cursor != "" {
			in["cursor"] = cursor
		}
		out, err := w.invoke(ctx, searchItemsName, in)
		if err != nil {
			return 0, err
		}
		count += len(outputs(out["data"]))
		page, _ := out["page"].(map[string]any)
		next, _ := page["next_cursor"].(string)
		if more, _ := page["has_more"].(bool); !more || next == "" {
			break
		}
		cursor = next
	}
	return min(count, maxOpenCounted), nil
}

// depthOf reads an impact row's depth, an int in process and a number once it crossed JSON.
func depthOf(v any) int {
	switch d := v.(type) {
	case int:
		return d
	case float64:
		return int(d)
	}
	return 0
}
