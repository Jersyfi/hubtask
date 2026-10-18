// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// A rule a pass set aside carries why over REST; one that evaluated cleanly carries nothing
// (UC-LIF-04 check 10).
func TestARuleSetAsideAnswersItsLastFailure(t *testing.T) {
	at := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	failing := retentionPolicyResponse(usecase.Output{
		"id": "01936f2a-7c1e-7000-8000-0000000000d1", "data_kind": "COMPLETED_ITEM", "action": "ARCHIVE",
		"last_failure": map[string]any{"code": "expression.evaluation_failed", "at": at.Format(time.RFC3339Nano)},
	})
	if failing.LastFailure == nil || failing.LastFailure.Code != "expression.evaluation_failed" ||
		!failing.LastFailure.At.Equal(at) {
		t.Errorf("the failure answered %+v", failing.LastFailure)
	}

	clean := retentionPolicyResponse(usecase.Output{
		"id": "01936f2a-7c1e-7000-8000-0000000000d2", "data_kind": "COMPLETED_ITEM", "action": "ARCHIVE",
	})
	if clean.LastFailure != nil {
		t.Errorf("a clean rule answered %+v", clean.LastFailure)
	}
}
