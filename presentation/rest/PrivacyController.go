// SPDX-License-Identifier: Apache-2.0
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

// Data subject rights over REST (data-protection.md §4). No rules here: which permission a
// step needs depends on the case, and that decision belongs to the application layer where the
// case can be read (ADR-0005).

const (
	createDataSubjectRequestUseCase = "CreateDataSubjectRequest"
	updateDataSubjectRequestUseCase = "UpdateDataSubjectRequest"
	restrictProcessingUseCase       = "RestrictProcessing"
	withdrawConsentUseCase          = "WithdrawConsent"
	listConsentsUseCase             = "ListConsents"
)

// CreateDataSubjectRequest answers POST /privacy/requests.
func (c *RestController) CreateDataSubjectRequest(
	w http.ResponseWriter, r *http.Request, _ openapi.CreateDataSubjectRequestParams,
) {
	var body openapi.DataSubjectRequestCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"kind": string(body.Kind)}
	if body.Scope != nil {
		in["scope"] = string(*body.Scope)
	}
	if body.SubjectAccountId != nil {
		in["subject_account_id"] = body.SubjectAccountId.String()
	}
	if body.SubjectEmail != nil {
		in["subject_email"] = string(*body.SubjectEmail)
	}
	if body.ReceivedOn != nil {
		in["received_on"] = body.ReceivedOn.String()
	}
	if body.DueAt != nil {
		in["due_at"] = body.DueAt.UTC().Format(time.RFC3339Nano)
	}
	if body.TargetId != nil {
		in["target_id"] = body.TargetId.String()
	}
	if body.Notes != nil {
		in["notes"] = *body.Notes
	}

	out, ok := c.read(w, r, createDataSubjectRequestUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, dataSubjectRequestResponse(out))
}

// dataSubjectRequestResponse maps one case onto the contract's schema.
func dataSubjectRequestResponse(row usecase.Output) openapi.DataSubjectRequest {
	request := openapi.DataSubjectRequest{
		Id:         uuidValue(row.String("id")),
		Kind:       openapi.DataSubjectRequestKind(row.String("kind")),
		Status:     openapi.DataSubjectRequestStatus(row.String("status")),
		ReceivedAt: timeValue(row["received_at"]),
		DueAt:      timeValue(row["due_at"]),
	}
	if scope := row.String("scope"); scope != "" {
		value := openapi.DataSubjectRequestScope(scope)
		request.Scope = &value
	}
	if mode := row.String("erasure_mode"); mode != "" {
		value := openapi.ErasureMode(mode)
		request.ErasureMode = &value
	}
	request.SubjectAccountId = uuidOrNil(row.String("subject_account_id"))
	request.HandledBy = uuidOrNil(row.String("handled_by"))
	request.ResultTargetId = uuidOrNil(row.String("result_target_id"))
	request.SubjectEmail = textOrNil(row.String("subject_email"))
	request.RejectionReason = textOrNil(row.String("rejection_reason"))
	request.ResultArchive = textOrNil(row.String("result_archive"))
	request.Notes = textOrNil(row.String("notes"))
	if completed, ok := row["completed_at"].(time.Time); ok {
		request.CompletedAt = &completed
	}
	if original, ok := row["original_due_at"].(time.Time); ok {
		request.OriginalDueAt = &original
	}
	if reason := row.String("extension_reason"); reason != "" {
		value := openapi.DataSubjectRequestExtensionReason(reason)
		request.ExtensionReason = &value
	}
	request.ReceivedOn = dateOrNil(row.String("received_on"))
	request.InformedOn = dateOrNil(row.String("informed_on"))
	request.ExtendableUntil = dateOrNil(row.String("extendable_until"))
	if parts, ok := row["kept"].([]map[string]any); ok && len(parts) > 0 {
		kept := erasureKeptResponse(parts)
		request.Kept = &kept
		basis := openapi.DataSubjectRequestKeptLegalBasis(row.String("kept_legal_basis"))
		request.KeptLegalBasis = &basis
	}
	return request
}

// erasurePreviewResponse maps what an erasure in a mode would keep.
func erasurePreviewResponse(out usecase.Output) openapi.ErasurePreview {
	parts, _ := out["kept"].([]map[string]any)
	return openapi.ErasurePreview{Mode: openapi.ErasureMode(out.String("mode")), Kept: erasureKeptResponse(parts)}
}

// erasureKeptResponse maps what each hold kept onto the contract's schema.
func erasureKeptResponse(parts []map[string]any) []openapi.ErasureKept {
	kept := make([]openapi.ErasureKept, 0, len(parts))
	for _, part := range parts {
		row := usecase.Output(part)
		out := openapi.ErasureKept{
			HoldId: uuidValue(row.String("hold_id")), Account: part["account"] == true,
			Entries: intOf(part["entries"]), Comments: intOf(part["comments"]),
			Assignments: intOf(part["assignments"]), Intake: intOf(part["intake"]),
		}
		if scope, ok := part["hold_scope"].(map[string]any); ok {
			kind, _ := scope["kind"].(string)
			out.HoldScope.Kind = openapi.ErasureKeptHoldScopeKind(kind)
			if id, ok := scope["id"].(string); ok {
				out.HoldScope.Id = uuidOrNil(id)
			}
		}
		if erased, ok := part["erased_at"].(time.Time); ok {
			out.ErasedAt = &erased
		}
		if blocked, ok := part["blocked"].(map[string]any); ok {
			code, _ := blocked["code"].(string)
			out.Blocked = &struct {
				Code   string             `json:"code"`
				Params *map[string]string `json:"params,omitempty"`
			}{Code: code}
			if params, ok := blocked["params"].(map[string]string); ok {
				out.Blocked.Params = &params
			}
		}
		kept = append(kept, out)
	}
	return kept
}

// intOf reads a count the use case answered as an int.
func intOf(value any) int {
	count, _ := value.(int)
	return count
}

// dateOrNil is a calendar day the use case answered as YYYY-MM-DD, or nil where it answered none.
func dateOrNil(value string) *openapi_types.Date {
	day, err := time.Parse(openapi_types.DateFormat, value)
	if err != nil {
		return nil
	}
	return &openapi_types.Date{Time: day}
}

// processingStateResponse maps what a restriction set, rather than the person it was set on: a
// restriction touches nothing else about an account, and their name and address would answer more
// than the caller asked about.
func processingStateResponse(out usecase.Output) openapi.ProcessingState {
	return openapi.ProcessingState{
		AccountId: uuidValue(out.String("id")),
		Status:    openapi.ProcessingStateStatus(out.String("status")),
	}
}

// ListConsents answers GET /privacy/consents: the register, the one place who withdrew is listed.
func (c *RestController) ListConsents(w http.ResponseWriter, r *http.Request, params openapi.ListConsentsParams) {
	out, ok := c.read(w, r, listConsentsUseCase, usecase.Input{"purpose": params.Purpose})
	if !ok {
		return
	}
	rows, _ := out["items"].([]usecase.Output)
	answer := struct {
		Data []openapi.ConsentRegisterEntry `json:"data"`
	}{Data: make([]openapi.ConsentRegisterEntry, 0, len(rows))}
	for _, row := range rows {
		entry := openapi.ConsentRegisterEntry{
			AccountId: uuidValue(row.String("account_id")), DisplayName: row.String("display_name"),
		}
		if since, held := row["since"].(time.Time); held {
			entry.Since = since
		}
		if source := row.String("source"); source != "" {
			value := openapi.ConsentRegisterEntrySource(source)
			entry.Source = &value
		}
		answer.Data = append(answer.Data, entry)
	}
	writeJSON(w, r, http.StatusOK, answer)
}

func ownAiPositionOf(out usecase.Output) openapi.OwnAiPosition {
	answer := openapi.OwnAiPosition{
		Shown:         boolAt(out, "shown"),
		PersonOptOut:  openapi.AiPersonOptOut(out.String("person_opt_out")),
		KeepingOut:    boolAt(out, "keeping_out"),
		InEffect:      boolAt(out, "in_effect"),
		NoticePending: boolAt(out, "notice_pending"),
	}
	if basis := out.String("basis"); basis != "" {
		value := openapi.AiOptOutBasis(basis)
		answer.Basis = &value
	}
	if contact := out.String("contact"); contact != "" {
		answer.Contact = &contact
	}
	answer.Since = timePointer(out["since"])
	answer.KeepingOutSince = timePointer(out["keeping_out_since"])
	return answer
}

func consentRecordOf(out usecase.Output) openapi.ConsentRecord {
	record := openapi.ConsentRecord{
		Id:        uuidValue(out.String("id")),
		Purpose:   out.String("purpose"),
		Granted:   boolAt(out, "granted"),
		GrantedAt: timePointer(out["granted_at"]),
	}
	record.AccountId = uuidOrNil(out.String("account_id"))
	if revoked, ok := out["revoked_at"].(time.Time); ok {
		record.RevokedAt = &revoked
	}
	if source := out.String("source"); source != "" {
		value := openapi.ConsentRecordSource(source)
		record.Source = &value
	}
	return record
}

// timePointer is timeValue for a field the generated type carries as a pointer.
func timePointer(value any) *time.Time {
	at, ok := value.(time.Time)
	if !ok {
		return nil
	}
	return &at
}
