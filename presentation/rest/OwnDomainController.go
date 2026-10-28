// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// A workspace's own domain (UC-INS-15): the owner's set-up and removal, the operator's list and
// block. Each a thin translation onto its use case.

const (
	setUpOwnDomainUseCase          = "SetUpOwnDomain"
	removeOwnDomainUseCase         = "RemoveOwnDomain"
	listOwnDomainsUseCase          = "ListOwnDomains"
	blockOwnDomainUseCase          = "BlockOwnDomain"
	makeOwnDomainMainUseCase       = "MakeOwnDomainMain"
	switchBackFromOwnDomainUseCase = "SwitchBackFromOwnDomain"
)

// MakeOwnDomainMain answers POST /tenant/own-domain:make-main.
func (c *RestController) MakeOwnDomainMain(w http.ResponseWriter, r *http.Request, params openapi.MakeOwnDomainMainParams) {
	c.moveMainAddress(w, r, makeOwnDomainMainUseCase, params.XHubtaskStepUp)
}

// SwitchBackFromOwnDomain answers POST /tenant/own-domain:switch-back.
func (c *RestController) SwitchBackFromOwnDomain(
	w http.ResponseWriter, r *http.Request, params openapi.SwitchBackFromOwnDomainParams,
) {
	c.moveMainAddress(w, r, switchBackFromOwnDomainUseCase, params.XHubtaskStepUp)
}

func (c *RestController) moveMainAddress(w http.ResponseWriter, r *http.Request, useCase string, stepUp *string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.MainAddressSwitch
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"step_up_token": stepUpHeaderField(stepUp)}
	if body.DryRun != nil {
		in["dry_run"] = *body.DryRun
	}
	out, err := c.UseCases.Invoke(r.Context(), useCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	mailed, _ := out["members_mailed"].(int)
	told, _ := out["members_told_at_sign_in"].(int)
	dryRun, _ := out["dry_run"].(bool)
	answer := openapi.WorkspaceAddressChanged{
		From: out.String("from"), MembersMailed: mailed, MembersToldAtSignIn: told, DryRun: dryRun,
	}
	if to := out.String("to"); to != "" {
		answer.To = &to
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// SetUpOwnDomain answers PUT /tenant/own-domain.
func (c *RestController) SetUpOwnDomain(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.OwnDomainSetUp
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), setUpOwnDomainUseCase, actorOf(r), usecase.Input{"domain": body.Domain})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, ownDomainOf(out))
}

// RemoveOwnDomain answers DELETE /tenant/own-domain.
func (c *RestController) RemoveOwnDomain(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), removeOwnDomainUseCase, actorOf(r), usecase.Input{}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListOwnDomains answers GET /admin/own-domains.
func (c *RestController) ListOwnDomains(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listOwnDomainsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["items"].([]usecase.Output)
	listed := make([]openapi.AdminOwnDomain, 0, len(rows))
	for _, row := range rows {
		listed = append(listed, adminOwnDomainOf(row))
	}
	writeJSON(w, r, http.StatusOK, listed)
}

// BlockOwnDomain answers POST /admin/own-domains/{domain}:block.
func (c *RestController) BlockOwnDomain(w http.ResponseWriter, r *http.Request, domain string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.OwnDomainBlock
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), blockOwnDomainUseCase, actorOf(r),
		usecase.Input{"domain": domain, "reason": body.Reason})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, adminOwnDomainOf(out))
}

func ownDomainOf(out usecase.Output) *openapi.OwnDomain {
	if out == nil {
		return nil
	}
	answer := &openapi.OwnDomain{
		Domain: out.String("domain"), State: openapi.OwnDomainState(out.String("state")),
		Since: timeValue(out["since"]), IsMain: boolValue(out["is_main"]),
		CertificateAsked: boolValue(out["certificate_asked"]), Records: []openapi.OwnDomainRecord{},
	}
	if checked, ok := out["checked_at"].(time.Time); ok {
		answer.CheckedAt = &checked
	}
	if reason := out.String("blocked_reason"); reason != "" {
		answer.BlockedReason = &reason
	}
	if fell, ok := out["fell_back_at"].(time.Time); ok {
		answer.FellBackAt = &fell
	}
	records, _ := out["records"].([]usecase.Output)
	for _, record := range records {
		answer.Records = append(answer.Records, openapi.OwnDomainRecord{
			Type: openapi.OwnDomainRecordType(record.String("type")), Name: record.String("name"),
			Value: record.String("value"), Found: boolValue(record["found"]),
		})
	}
	return answer
}

func adminOwnDomainOf(row usecase.Output) openapi.AdminOwnDomain {
	listed := openapi.AdminOwnDomain{
		Domain: row.String("domain"), TenantName: row.String("tenant_name"),
		State: openapi.AdminOwnDomainState(row.String("state")), Since: timeValue(row["since"]),
		IsMain: boolValue(row["is_main"]),
	}
	if tenant := row.String("tenant_id"); tenant != "" {
		id := uuidValue(tenant)
		listed.TenantId = &id
	}
	if reason := row.String("blocked_reason"); reason != "" {
		listed.BlockedReason = &reason
	}
	if fell, ok := row["fell_back_at"].(time.Time); ok {
		listed.FellBackAt = &fell
	}
	if host := row.String("address_host"); host != "" {
		listed.AddressHost = &host
	}
	return listed
}
