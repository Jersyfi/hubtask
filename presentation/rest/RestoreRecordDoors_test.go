// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

const restoredTenant = "0192f000-0000-7000-8000-0000000000aa"

// After a restore the operator hands the legal holds and the erasure cases back as records: the
// generated doors pass them on as the objects they arrived as, with the recovery point and the
// step-up proof, and the use case parses each instant (backup-restore.md §8.5).
func TestTheRestoreRecordsReachTheirUseCasesAsSent(t *testing.T) {
	for name, tc := range map[string]struct {
		path, body, useCase, list string
		stepUp                    bool
	}{
		"legal holds": {
			path: ":replace-legal-holds", useCase: "ReplaceLegalHolds", list: "holds",
			body: `{"recovery_point":"2026-10-01T12:00:00+02:00","holds":[{"id":"0192f000-0000-7000-8000-0000000000b1",` +
				`"scope":{"kind":"TENANT"},"reason":"litigation","placed_by":"0192f000-0000-7000-8000-0000000000b2",` +
				`"placed_at":"2026-09-01T08:00:00Z"}]}`,
		},
		"erasures": {
			path: ":reapply-erasures", useCase: "ReapplyErasures", list: "erasures", stepUp: true,
			body: `{"recovery_point":"2026-10-01T12:00:00+02:00","erasures":[{"id":"0192f000-0000-7000-8000-0000000000b3",` +
				`"erasure_mode":"ANONYMIZE","received_at":"2026-09-01T08:00:00Z","due_at":"2026-10-01T08:00:00Z",` +
				`"completed_at":"2026-09-02T08:00:00Z"}]}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cat := &catalogue{out: usecase.Output{tc.list: []any{}}}
			controller := NewRestController()
			controller.UseCases = cat
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				APIBasePath+"/admin/tenants/"+restoredTenant+tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(stepUpHeader, "proof")
			response := httptest.NewRecorder()
			controller.Routes().ServeHTTP(response, request)

			if response.Code != http.StatusOK || cat.name != tc.useCase {
				t.Fatalf("status %d, use case %q: %s", response.Code, cat.name, response.Body)
			}
			if cat.in.String("tenant_id") != restoredTenant {
				t.Errorf("tenant %v", cat.in["tenant_id"])
			}
			at, err := time.Parse(time.RFC3339Nano, cat.in.String("recovery_point"))
			if err != nil || !at.Equal(time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)) {
				t.Errorf("recovery point %v", cat.in["recovery_point"])
			}
			records, _ := cat.in[tc.list].([]any)
			record, _ := records[0].(map[string]any)
			if len(records) != 1 || record["id"] == nil {
				t.Errorf("%s %v", tc.list, cat.in[tc.list])
			}
			if _, passed := cat.in["step_up_token"]; passed != tc.stepUp {
				t.Errorf("step-up passed %t, want %t", passed, tc.stepUp)
			}
		})
	}
}

// An unknown member of a record is refused before the use case runs, as the typed body refused it.
func TestARestoreRecordWithAnUnknownMemberIsRefused(t *testing.T) {
	cat := &catalogue{}
	controller := NewRestController()
	controller.UseCases = cat
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/admin/tenants/"+restoredTenant+":replace-legal-holds",
		strings.NewReader(`{"recovery_point":"2026-10-01T12:00:00Z","holds":[],"extra":1}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || cat.invoked {
		t.Errorf("status %d, invoked %t", response.Code, cat.invoked)
	}
}
