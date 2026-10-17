// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-INS-09 check 5: a ceiling the installation locked reaches the client as locked, so the
// workspace's screen can say who set it.
func TestAQuotaStandingAnswersTheInstallationsLock(t *testing.T) {
	answered := quotaStandingsResponse(usecase.Output{"data": []usecase.Output{
		{"quota": "items", "limit": int64(1000), "configured": false, "locked": true},
		{"quota": "media_bytes", "limit": int64(0), "configured": true, "locked": false},
	}})
	if len(answered) != 2 {
		t.Fatalf("answered %d standings", len(answered))
	}
	if answered[0].Locked == nil || !*answered[0].Locked {
		t.Errorf("the locked ceiling answers locked %v", answered[0].Locked)
	}
	if answered[1].Locked != nil && *answered[1].Locked {
		t.Error("an open ceiling answers locked")
	}
}
