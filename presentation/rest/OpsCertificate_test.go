// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type certificatesFake map[string]bool

func (f certificatesFake) Allowed(_ context.Context, host string) (bool, error) {
	if host == "failing.example" {
		return false, errors.New("the database is away")
	}
	return f[host], nil
}

// UC-INS-15 check 5: the ingress asks the operations port with `?domain=` and reads the status
// alone - 200 for a host that may have a certificate, 404 for every other, 503 when the answer
// cannot be had - so a failure never reads as a yes.
func TestTheOperationsPortAnswersTheIngress(t *testing.T) {
	ops := OpsController{Certificates: certificatesFake{"tasks.acme.com": true}}.Routes()
	for domain, want := range map[string]int{
		"tasks.acme.com":    http.StatusOK,
		"elsewhere.example": http.StatusNotFound,
		"":                  http.StatusNotFound,
		"failing.example":   http.StatusServiceUnavailable,
	} {
		recorder := httptest.NewRecorder()
		ops.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/tls/allowed?domain="+domain, nil))
		if recorder.Code != want {
			t.Errorf("%q: %d, want %d", domain, recorder.Code, want)
		}
	}
	recorder := httptest.NewRecorder()
	OpsController{}.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/tls/allowed?domain=tasks.acme.com", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("unwired: %d", recorder.Code)
	}
}
