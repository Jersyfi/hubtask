// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// UC-INS-01 check 1, the service's half: a code is issued on an empty installation, valid for thirty
// minutes, and none once a workspace exists.
func TestACodeIsIssuedOnlyOnAnEmptyInstallation(t *testing.T) {
	codes := &setupCodes{}
	work := &unitOfWork{}
	issuer := IssueSetupCode{
		Codes: codes, UnitOfWork: work, Clock: clock.Fixed(now), IDs: &countingIDs{},
		Entropy: clock.FixedEntropy{},
	}

	issued, ok, err := issuer.Execute(t.Context())
	if err != nil || !ok {
		t.Fatalf("an empty installation issued nothing: %v %v", ok, err)
	}
	if !issued.ExpiresAt.Equal(now.Add(domain.SetupCodeLifetime)) {
		t.Errorf("expires %v, want thirty minutes from now", issued.ExpiresAt)
	}
	if len(codes.issued) != 1 || codes.issued[0].Canonical() != issued.Code.Canonical() {
		t.Errorf("stored %v, answered %q", codes.issued, issued.Code.Canonical())
	}
	if !work.scopes[0].System {
		t.Errorf("issued under %+v, want the system scope", work.scopes[0])
	}

	codes.done = true
	if _, ok, err := issuer.Execute(t.Context()); err != nil || ok {
		t.Errorf("an installation with a workspace issued a code: %v %v", ok, err)
	}
}
