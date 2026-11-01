// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// asSAML turns the fixture's one provider into a SAML provider, as its stored row.
func asSAML(t *testing.T, f *oidcFixture) {
	t.Helper()
	for i := range f.store.rows {
		if f.store.rows[i].ID == f.provider.ID {
			f.store.rows[i].Kind = domain.KindSAML
			return
		}
	}
	t.Fatal("the fixture's provider is not stored")
}

// UC-ID-30 check 4: a SAML answer reaches only a sign-in's flow, so a step-up at a SAML provider is
// refused with identity_provider.saml_sign_in_only.
func TestAStepUpAtASAMLProviderIsRefused(t *testing.T) {
	f := providerStepUpFixture(t)
	asSAML(t, f)

	_, err := StartProviderStepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor())
	refusedWith(t, err, "identity_provider.saml_sign_in_only")
}

// UC-ID-30 check 4: connecting an account through a SAML provider is refused the same way.
func TestConnectingAnAccountThroughASAMLProviderIsRefused(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	asSAML(t, c.f)

	refusedWith(t, c.start(t, signedInActor()), "identity_provider.saml_sign_in_only")
}
