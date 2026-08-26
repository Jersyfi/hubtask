// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"encoding/hex"
	"sort"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
)

// The projection every channel answers with, and the two conversions it needs. In the application
// layer rather than in the REST adapter, because MCP and an automation rule read the same entry -
// a shape decided in one adapter is a shape the other two do not have (arc42 §4).

// EntryOutput is one stored entry as it leaves the boundary.
//
// What is absent is deliberate. There is no tenant identifier - the reader is inside one - and no
// `prev_hash`: the chain is checked by `:verify`, which walks it server-side, and handing a client
// the neighbouring digest would invite a verifier that re-implements the canonical serialisation
// and proves that two implementations agree (audit.md §3).
func EntryOutput(record repository.Record) usecase.Output {
	entry := record.Entry

	out := usecase.Output{
		"id":          record.ID.String(),
		"seq":         record.Seq,
		"occurred_at": entry.OccurredAt.UTC(),
		"action":      string(entry.Action),
		"outcome":     string(entry.Outcome),
		"severity":    string(entry.Severity),
		"actor":       actorOutput(entry),
		"target":      targetOutput(entry),
		"changes":     changesOutput(entry.Changes),
		"context":     contextOutput(entry),
		"hash":        hex.EncodeToString(record.Hash),
	}
	if entry.LegalBasis != "" {
		out["legal_basis"] = entry.LegalBasis
	}
	return out
}

func actorOutput(entry port.Entry) map[string]any {
	actor := map[string]any{"type": string(entry.ActorKind)}
	if !entry.ActorID.IsZero() {
		actor["id"] = entry.ActorID.String()
	}
	if entry.ActorLabel != "" {
		// The label as it was at the time, which is why an entry stays readable after the account
		// is deleted (audit.md §2, test AT-7). It is personal data and it is here because the
		// alternative - a trail that loses its meaning through a deletion - does not do its job.
		actor["label"] = entry.ActorLabel
	}
	if !entry.OnBehalfOf.IsZero() {
		actor["on_behalf_of"] = entry.OnBehalfOf.String()
	}
	return actor
}

func targetOutput(entry port.Entry) map[string]any {
	target := map[string]any{}
	if entry.TargetType != "" {
		target["type"] = entry.TargetType
	}
	if !entry.TargetID.IsZero() {
		target["id"] = entry.TargetID.String()
	}
	if entry.TargetLabel != "" {
		target["label"] = entry.TargetLabel
	}
	return target
}

func contextOutput(entry port.Entry) map[string]any {
	out := map[string]any{}
	for name, value := range map[string]string{
		"request_id": entry.Context.RequestID,
		"trace_id":   entry.Context.TraceID,
		// The address is already truncated where the entry was written - IPv4 /24, IPv6 /48 - and
		// the contract calls the field `ip_prefix`, which is what it is (audit.md §4).
		"ip_prefix":        entry.Context.IPTruncated,
		"user_agent_class": entry.Context.UserAgentClass,
		"api_client":       entry.Context.APIClient,
	} {
		if value != "" {
			out[name] = value
		}
	}
	if !entry.Context.RuleID.IsZero() {
		out["rule_id"] = entry.Context.RuleID.String()
	}
	return out
}

// changesOutput turns the stored map into the array the contract carries.
//
// Sorted by field name, because a map has no order and a client diffing two entries would
// otherwise see a change where there was none. The masking itself is not undone and cannot be: a
// `SENSITIVE` field was written as "changed" with two hashes and there is no value to recover, a
// `SECRET` one was never written at all.
func changesOutput(changes map[string]any) []map[string]any {
	fields := make([]string, 0, len(changes))
	for field := range changes {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	out := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		change := map[string]any{"field": field}
		if values, ok := changes[field].(map[string]any); ok {
			for _, key := range []string{"from", "to", "changed", "from_hash", "to_hash"} {
				if value, present := values[key]; present {
					change[key] = value
				}
			}
		}
		out = append(out, change)
	}
	return out
}

// pageOutput is the shape of a paged answer: `{ "data": [...], "page": {...} }`
// (api-guidelines.md §4). The REST adapter renders the audit list's own older spelling -
// `items` and `next_cursor` - out of it, because that is what the contract has declared for this
// path since phase 0.
func pageOutput(data []usecase.Output, info repository.PageInfo) usecase.Output {
	page := map[string]any{"next_cursor": nil, "has_more": info.HasMore}
	if info.NextCursor != "" {
		page["next_cursor"] = info.NextCursor
	}
	return usecase.Output{"data": data, "page": page}
}

// parseInstant reads the one spelling the contract declares, RFC 3339, and refuses anything else
// with the field the caller sent.
func parseInstant(raw, field string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		code := "audit." + field + "_malformed"
		return time.Time{}, shared.ErrValidation.
			WithDetail(code).
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{Path: "/" + field, Code: code})
	}
	return at, nil
}
