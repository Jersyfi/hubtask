// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The audit surface (E-09, audit.md §5). No rules here: who may read what is decided in the
// application layer, and a refusal is recorded there - an adapter that narrowed the filter itself
// would be an adapter deciding a permission (ADR-0005).

const (
	listAuditEntriesUseCase = "ListAuditEntries"
	verifyAuditChainUseCase = "VerifyAuditChain"
	exportAuditTrailUseCase = "ExportAuditTrail"
)

// ListAuditEntries answers GET /audit.
//
// The page is spelled `items` and `next_cursor` here rather than the `data`/`page` shape every
// other list uses, because that is what the contract has declared for this path since phase 0.
// The catalogue answers the ordinary shape and this is the one place it is renamed; renaming it in
// the contract instead would break whatever already reads it.
func (c *RestController) ListAuditEntries(
	w http.ResponseWriter, r *http.Request, params openapi.ListAuditEntriesParams,
) {
	in := usecase.Input{
		"action":      optionalStringField(params.Action),
		"actor_id":    optionalUUIDField(params.ActorId),
		"target_type": optionalStringField(params.TargetType),
		"target_id":   optionalUUIDField(params.TargetId),
		"cursor":      optionalStringField(params.Cursor),
		"size":        optionalIntField(params.Limit),
	}
	if params.From != nil {
		in["from"] = params.From.UTC().Format(time.RFC3339Nano)
	}
	if params.To != nil {
		in["to"] = params.To.UTC().Format(time.RFC3339Nano)
	}
	if params.Outcome != nil {
		in["outcome"] = string(*params.Outcome)
	}

	out, ok := c.read(w, r, listAuditEntriesUseCase, in)
	if !ok {
		return
	}

	page := auditPage{Items: []openapi.AuditEntry{}}
	for _, row := range rowsOf(out) {
		page.Items = append(page.Items, auditEntryResponse(row))
	}
	page.NextCursor = pageResponse(out).NextCursor
	writeJSON(w, r, http.StatusOK, page)
}

// VerifyAuditChain answers POST /audit:verify.
func (c *RestController) VerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	var body openapi.VerifyAuditChainJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	out, ok := c.read(w, r, verifyAuditChainUseCase, usecase.Input{
		"from": body.From.UTC().Format(time.RFC3339Nano),
		"to":   body.To.UTC().Format(time.RFC3339Nano),
	})
	if !ok {
		return
	}

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
	writeJSON(w, r, http.StatusOK, answer)
}

// ExportAuditTrail answers POST /audit:export.
//
// A `202` with the job to poll. The archive lands at the backup target the caller named rather than
// at a URL this server can hand back, so the reference carries no `result_url`: an export is at
// somebody else's machine, which is where the caller asked for it.
func (c *RestController) ExportAuditTrail(w http.ResponseWriter, r *http.Request) {
	var body openapi.AuditExport
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{
		"from":      body.From.UTC().Format(time.RFC3339Nano),
		"to":        body.To.UTC().Format(time.RFC3339Nano),
		"target_id": body.TargetId.String(),
	}
	if body.Format != nil {
		in["format"] = string(*body.Format)
	}

	out, ok := c.read(w, r, exportAuditTrailUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusAccepted, openapi.JobRef{
		JobId:  uuidValue(out.String("job_id")),
		Status: openapi.JobStatusQUEUED,
	})
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
