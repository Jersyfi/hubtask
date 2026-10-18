// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
)

// A consent hands an app exactly what a session carries: the two rules have their own reasons
// (identity's Consentable, catalogue.SessionScopes), and this is where they are held to one answer.
func TestSGAConsentHandsOutWhatASessionCarriesAndNoMore(t *testing.T) {
	for _, scope := range catalogue.Scopes() {
		if identityservice.Consentable(scope) != slices.Contains(catalogue.SessionScopes(), scope) {
			t.Errorf("%s: consentable %v, carried by a session %v", scope,
				identityservice.Consentable(scope), slices.Contains(catalogue.SessionScopes(), scope))
		}
	}
}
