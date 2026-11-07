// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "strings"

// returnStateSeparator is outside the base64url alphabet the handle is drawn in, so the split is
// unambiguous.
const returnStateSeparator = "."

// ReturnState is the `state` a connector's authorization hands a provider: the workspace, then the
// unguessable handle. A provider returns to the installation's one address, and in multi-tenant
// operation the workspace part is what lets that return be located on the workspace's own host
// (multi-tenancy.md §3). The workspace part proves nothing; the handle's hash is what is looked up,
// in that workspace only.
func ReturnState(tenantID ID, handle string) string {
	return tenantID.String() + returnStateSeparator + handle
}

// TenantOfReturnState reads the workspace a ReturnState names. False for anything else, a state
// minted before the workspace was part of it included.
func TenantOfReturnState(state string) (ID, bool) {
	tenant, handle, found := strings.Cut(state, returnStateSeparator)
	if !found || handle == "" {
		return "", false
	}
	id, err := ParseID(tenant)
	if err != nil {
		return "", false
	}
	return id, true
}
