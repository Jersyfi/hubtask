// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "context"

// PeopleLimit refuses a person past the workspace's people limit (multi-tenancy.md §4.2), counted
// in the caller's transaction under the workspace's people lock, so two creations at the last place
// admit one. The quota guard satisfies it; the identity service does not reach into the quota
// service for it. Nil enforces no limit - the composition root sets it on every door that creates a
// person, which its wiring test holds.
type PeopleLimit interface {
	// Accounts asks for room for that many more people.
	Accounts(ctx context.Context, tenant string, adding int64) error
	// Renewal asks for room to renew the invitation: none where its link is still open.
	Renewal(ctx context.Context, tenant, account string) error
}
