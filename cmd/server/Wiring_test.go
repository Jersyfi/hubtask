// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
)

// The writers are values, so a copy taken before the sign-in path learns the rule never learns it.
// The password writer's copy (which a reset opens its session through) and the provider's copy
// (which a provider sign-in and the LINK step open theirs through) were both taken before, so a
// reset read the old boolean instead of the rule and a provider session answered to no session
// bound in production - while every test, which wires its own fixtures, passed (SC-03, SC-06).
func TestTheRuleReachesEveryWriterThatOpensASession(t *testing.T) {
	session := identity.SessionWriter{}
	passwords := identity.PasswordWriter{Session: session}
	oidc := identity.OidcWriter{Session: session}

	teachTheRule(&session, &passwords, &oidc)

	for name, rule := range map[string]identity.SignInRuleReader{
		"the sign-in path":         session.Rule,
		"the reset's session copy": passwords.Session.Rule,
		"the provider's copy":      oidc.Session.Rule,
	} {
		if rule == nil {
			t.Errorf("%s opens sessions without the rule", name)
		}
	}
}
