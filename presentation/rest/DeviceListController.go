// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The device's list of workspaces (ADR-0095 §7, multi-tenancy.md §3.1): the entry a sign-in hands
// the browser, and the check the installation's own address asks before it keeps or shows one.

const (
	issueDeviceListEntryUseCase = "IssueDeviceListEntry"
	checkDeviceListUseCase      = "CheckDeviceList"
)

// IssueDeviceListEntry answers GET /tenant/device-list-entry.
func (c *RestController) IssueDeviceListEntry(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), issueDeviceListEntryUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, openapi.DeviceListEntry{
		Entry: out.String("entry"), Host: out.String("host"), DisplayName: out.String("display_name"),
	})
}

// CheckDeviceList answers POST /auth/device-list:check. Anonymous, and in the auth bucket
// (RateLimit.go).
func (c *RestController) CheckDeviceList(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.DeviceListCheck
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), checkDeviceListUseCase, actorOf(r),
		usecase.Input{"entries": body.Entries})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["entries"].([]usecase.Output)
	answer := openapi.DeviceListChecked{Entries: make([]openapi.DeviceListEntryState, 0, len(rows))}
	for _, row := range rows {
		answers, _ := row["answers"].(bool)
		answer.Entries = append(answer.Entries, openapi.DeviceListEntryState{
			Entry: row.String("entry"), Host: row.String("host"), DisplayName: row.String("display_name"),
			Answers: answers,
		})
	}
	writeJSON(w, r, http.StatusOK, answer)
}
