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

// Data subject rights over REST (E-10, data-protection.md §4). No rules here: which permission a
// step needs depends on the case, and that decision belongs to the application layer where the
// case can be read (ADR-0005).

const (
	createDataSubjectRequestUseCase = "CreateDataSubjectRequest"
	listDataSubjectRequestsUseCase  = "ListDataSubjectRequests"
	updateDataSubjectRequestUseCase = "UpdateDataSubjectRequest"
	restrictProcessingUseCase       = "RestrictProcessing"
	withdrawConsentUseCase          = "WithdrawConsent"
)

// ListDataSubjectRequests answers GET /privacy/requests.
func (c *RestController) ListDataSubjectRequests(
	w http.ResponseWriter, r *http.Request, params openapi.ListDataSubjectRequestsParams,
) {
	in := usecase.Input{
		"cursor": optionalStringField(params.Cursor),
		"size":   optionalIntField(params.Size),
	}
	if params.Status != nil {
		in["status"] = string(*params.Status)
	}
	if params.Kind != nil {
		in["kind"] = string(*params.Kind)
	}
	if params.DueWithinDays != nil {
		in["due_within_days"] = *params.DueWithinDays
	}
	if params.IncludeClosed != nil {
		in["include_closed"] = *params.IncludeClosed
	}

	out, ok := c.read(w, r, listDataSubjectRequestsUseCase, in)
	if !ok {
		return
	}

	page := struct {
		Data []openapi.DataSubjectRequest `json:"data"`
		Page openapi.PageInfo             `json:"page"`
	}{Data: []openapi.DataSubjectRequest{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, dataSubjectRequestResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

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

// UpdateDataSubjectRequest answers PATCH /privacy/requests/{requestId}.
func (c *RestController) UpdateDataSubjectRequest(
	w http.ResponseWriter, r *http.Request, requestID openapi_types.UUID,
) {
	var body openapi.DataSubjectRequestUpdate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"request_id": requestID.String()}
	if body.Status != nil {
		in["status"] = string(*body.Status)
	}
	if body.ErasureMode != nil {
		in["erasure_mode"] = string(*body.ErasureMode)
	}
	if body.HandledBy != nil {
		in["handled_by"] = body.HandledBy.String()
	}
	if body.RejectionReason != nil {
		in["rejection_reason"] = *body.RejectionReason
	}
	if body.TargetId != nil {
		in["target_id"] = body.TargetId.String()
	}
	if body.Notes != nil {
		in["notes"] = *body.Notes
	}

	out, ok := c.read(w, r, updateDataSubjectRequestUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, dataSubjectRequestResponse(out))
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
	return request
}

// RestrictProcessing answers POST /accounts/{accountId}:restrict.
func (c *RestController) RestrictProcessing(
	w http.ResponseWriter, r *http.Request, accountID openapi.AccountId,
) {
	var body openapi.ProcessingRestriction
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"account_id": accountID.String(), "restricted": body.Restricted}
	if body.Reason != nil {
		in["reason"] = *body.Reason
	}

	out, ok := c.read(w, r, restrictProcessingUseCase, in)
	if !ok {
		return
	}

	// What the call set, rather than the person it was set on. A restriction touches nothing else
	// about an account, and answering their name and address here would be answering more than the
	// caller asked about.
	writeJSON(w, r, http.StatusOK, openapi.ProcessingState{
		AccountId: uuidValue(out.String("id")),
		Status:    openapi.ProcessingStateStatus(out.String("status")),
	})
}

// WithdrawConsent answers POST /privacy/consents:withdraw.
func (c *RestController) WithdrawConsent(w http.ResponseWriter, r *http.Request) {
	var body openapi.ConsentWithdrawal
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}

	in := usecase.Input{"purpose": body.Purpose}
	if body.AccountId != nil {
		in["account_id"] = body.AccountId.String()
	}
	if body.Reason != nil {
		in["reason"] = *body.Reason
	}

	out, ok := c.read(w, r, withdrawConsentUseCase, in)
	if !ok {
		return
	}

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
	writeJSON(w, r, http.StatusOK, record)
}

// timePointer is timeValue for a field the generated type carries as a pointer.
func timePointer(value any) *time.Time {
	at, ok := value.(time.Time)
	if !ok {
		return nil
	}
	return &at
}
