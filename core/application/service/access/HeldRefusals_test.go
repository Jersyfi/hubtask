// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// A refusal recorded under a held context is written only when the holder records it - after the
// caller's transaction, which the refusal rolls back - and a holder taken under another one leaves
// the writing to the outer. The PostgreSQL half is test/integration's
// TestARefusedAcceptanceIsInTheTrail.
func TestAHeldRefusalIsWrittenWhenTheHolderRecordsIt(t *testing.T) {
	authorize, _, trail, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleMember},
	})

	outer, held := HoldRefusals(context.Background())
	inner, nested := HoldRefusals(outer)
	if err := authorize.Authorize(inner, actorWithScopes("containers:write"), request()); err == nil {
		t.Fatal("a member was let in")
	}
	if len(trail.entries) != 0 {
		t.Fatalf("%d entries written while held, want none yet", len(trail.entries))
	}

	nested.Record(inner)
	if len(trail.entries) != 0 {
		t.Fatalf("the inner holder wrote %d entries, want the outer to write them", len(trail.entries))
	}

	held.Record(context.Background())
	if len(trail.entries) != 1 {
		t.Fatalf("%d entries after recording, want the one refusal", len(trail.entries))
	}
	held.Record(context.Background())
	if len(trail.entries) != 1 {
		t.Errorf("recording twice wrote %d entries, want the one", len(trail.entries))
	}
}
