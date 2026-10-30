// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The audit surface (audit.md §5). No rules here: who may read what is decided in the
// application layer, and a refusal is recorded there - an adapter that narrowed the filter itself
// would be an adapter deciding a permission (ADR-0005).

// auditPageResponse draws GET /audit's page (x-hubtask-rest-list: whole).
//
// The page is spelled `items` and `next_cursor` here rather than the `data`/`page` shape every
// other list uses, because that is what the contract declares for this path. The catalogue
// answers the ordinary shape and this is the one place it is renamed.
func auditPageResponse(out usecase.Output) auditPage {
	page := auditPage{Items: []openapi.AuditEntry{}}
	for _, row := range rowsOf(out) {
		page.Items = append(page.Items, auditEntryResponse(row))
	}
	page.NextCursor = pageResponse(out).NextCursor
	return page
}

// auditVerificationResponse draws POST /audit:verify's answer.
func auditVerificationResponse(out usecase.Output) auditVerification {
	answer := auditVerification{
		Valid:    boolAt(out, "valid"),
		Checked:  out.Int("checked"),
		Gaps:     []int64{},
		GapCount: out.Int("gap_count"),
	}
	if gaps, ok := out["gaps"].([]int64); ok {
		answer.Gaps = gaps
	}
	if broken, ok := out["first_broken_seq"].(int64); ok {
		answer.FirstBrokenSeq = &broken
	}
	if sealed, ok := out["sealed_until"].(time.Time); ok {
		answer.SealedUntil = &sealed
	}
	answer.AnchoringConfigured = boolAt(out, "anchoring_configured")
	if at, ok := out["anchored_until"].(time.Time); ok {
		answer.AnchoredUntil = &at
	}
	if seq, ok := out["anchor_seq"].(int64); ok {
		answer.AnchorSeq = &seq
	}
	if agrees, ok := out["anchor_agrees"].(bool); ok {
		answer.AnchorAgrees = &agrees
	}
	if code, ok := out["anchor_error_code"].(string); ok && code != "" {
		answer.AnchorErrorCode = &code
	}
	return answer
}

// auditVerification is the answer of POST /audit:verify, which the contract declares inline. The
// two nulls are explicit: a client reading "no break" out of a missing key would read the same
// thing out of a key it forgot.
type auditVerification struct {
	Valid          bool       `json:"valid"`
	Checked        int        `json:"checked"`
	FirstBrokenSeq *int64     `json:"first_broken_seq"`
	Gaps           []int64    `json:"gaps"`
	GapCount       int        `json:"gap_count"`
	SealedUntil    *time.Time `json:"sealed_until"`
	// The anchor half: explicit nulls, for the same reason `sealed_until` is one.
	AnchoringConfigured bool       `json:"anchoring_configured"`
	AnchoredUntil       *time.Time `json:"anchored_until"`
	AnchorSeq           *int64     `json:"anchor_seq"`
	AnchorAgrees        *bool      `json:"anchor_agrees"`
	AnchorErrorCode     *string    `json:"anchor_error_code"`
}

// auditPage is the response of GET /audit, which the contract declares inline rather than as a
// named schema - so there is no generated type for it and this is the one place it is written out.
type auditPage struct {
	Items      []openapi.AuditEntry `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

// auditEntryResponse maps one entry of the catalogue's answer onto the contract's schema.
func auditEntryResponse(row usecase.Output) openapi.AuditEntry {
	entry := openapi.AuditEntry{
		Id:         uuidValue(row.String("id")),
		OccurredAt: timeValue(row["occurred_at"]),
		Action:     row.String("action"),
		Outcome:    openapi.AuditEntryOutcome(row.String("outcome")),
	}
	if seq := row.Int("seq"); seq != 0 {
		entry.Seq = &seq
	}
	if severity := row.String("severity"); severity != "" {
		value := openapi.AuditEntrySeverity(severity)
		entry.Severity = &value
	}
	entry.Hash = textOrNil(row.String("hash"))
	entry.LegalBasis = textOrNil(row.String("legal_basis"))

	actor, _ := row["actor"].(map[string]any)
	entry.Actor.Id = uuidOrNil(stringAt(actor, "id"))
	entry.Actor.Label = textOrNil(stringAt(actor, "label"))
	entry.Actor.OnBehalfOf = uuidOrNil(stringAt(actor, "on_behalf_of"))
	if kind := stringAt(actor, "type"); kind != "" {
		value := openapi.AuditActorType(kind)
		entry.Actor.Type = &value
	}

	if target, ok := row["target"].(map[string]any); ok && len(target) > 0 {
		entry.Target = &openapi.AuditTarget{
			Id:    uuidOrNil(stringAt(target, "id")),
			Label: textOrNil(stringAt(target, "label")),
			Type:  textOrNil(stringAt(target, "type")),
		}
	}

	if context, ok := row["context"].(map[string]any); ok && len(context) > 0 {
		entry.Context = &openapi.AuditContext{
			ApiClient:      textOrNil(stringAt(context, "api_client")),
			IpPrefix:       textOrNil(stringAt(context, "ip_prefix")),
			RequestId:      textOrNil(stringAt(context, "request_id")),
			RuleId:         uuidOrNil(stringAt(context, "rule_id")),
			TraceId:        textOrNil(stringAt(context, "trace_id")),
			UserAgentClass: textOrNil(stringAt(context, "user_agent_class")),
		}
	}

	entry.Changes = auditChangesResponse(row["changes"])
	return entry
}

// auditChangesResponse renders the masked fields. A `SENSITIVE` one carries `changed` and its two
// hashes and no value, because there is no value to recover - the masking happened where the entry
// was written, and this is a projection of what is stored rather than of what happened.
func auditChangesResponse(value any) *[]openapi.AuditChange {
	rows, ok := value.([]map[string]any)
	if !ok || len(rows) == 0 {
		return nil
	}

	changes := make([]openapi.AuditChange, 0, len(rows))
	for _, row := range rows {
		change := openapi.AuditChange{
			Field:    textOrNil(stringAt(row, "field")),
			From:     row["from"],
			To:       row["to"],
			FromHash: textOrNil(stringAt(row, "from_hash")),
			ToHash:   textOrNil(stringAt(row, "to_hash")),
		}
		if changed, ok := row["changed"].(bool); ok {
			change.Changed = &changed
		}
		changes = append(changes, change)
	}
	return &changes
}

// boolAt reads one flag out of a result. An absent field is false, which is what an absent flag
// means everywhere else in this layer.
func boolAt(out usecase.Output, key string) bool {
	value, _ := out[key].(bool)
	return value
}

// stringAt reads one string out of a projection, and answers the empty string for anything that
// is not one - a missing key and a value of another type are the same to a mapper.
func stringAt(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

// textOrNil keeps an absent value absent. The generated fields are pointers with `omitempty`, so
// an empty string would appear as an empty field rather than as no field.
func textOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func uuidOrNil(value string) *openapi_types.UUID {
	if value == "" {
		return nil
	}
	id := uuidValue(value)
	return &id
}
