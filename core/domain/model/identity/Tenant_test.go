// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The workspace's standing gates every credential (H-06, multi-tenancy.md §5): active proceeds,
// everything else refuses with the code the lifecycle table names, and a standing this build
// does not know fails closed.
func TestTenantStandingGatesTheCredential(t *testing.T) {
	cases := []struct {
		name     string
		status   identity.TenantStatus
		wantCode string
	}{
		{"active proceeds", identity.TenantActive, ""},
		{"suspended refuses", identity.TenantSuspended, "access.tenant_suspended"},
		{"pending deletion refuses", identity.TenantPendingDeletion, "access.tenant_pending_deletion"},
		{"the zero value fails closed", identity.TenantStatus(""), "access.tenant_suspended"},
		{"an unknown standing fails closed", identity.TenantStatus("MIGRATING"), "access.tenant_suspended"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.status.Verify()
			if c.wantCode == "" {
				if err != nil {
					t.Fatalf("an active workspace refused: %v", err)
				}
				return
			}
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("not a domain error: %v", err)
			}
			if !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("category %v, want forbidden", err)
			}
			if domainErr.DetailCode != c.wantCode {
				t.Errorf("code %q, want %q", domainErr.DetailCode, c.wantCode)
			}
		})
	}
}
