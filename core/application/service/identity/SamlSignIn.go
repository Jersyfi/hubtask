// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A SAML provider answers by posting the browser to this installation's assertion consumer service
// (identity.md §26). The answer is kept on the sign-in's flow unverified, and the browser goes on to
// the one callback every provider returns to, carrying a single-use handoff code: the sign-in then
// finishes exactly as an OpenID Connect provider's return does, where CompleteOidcSignIn verifies
// the kept answer. So a SAML provider shares the admission, the arrival and the session with every
// other way in, and the workspace's host keeps the session (multi-tenancy.md §3).

// samlHandoffBytes is the handoff code's entropy.
const samlHandoffBytes = 32

func samlSignInOnly() error {
	return shared.ErrValidation.WithDetail("identity_provider.saml_sign_in_only")
}

func samlFlowNotFound() error {
	return shared.ErrNotFound.WithDetail("auth.oidc_flow_not_found")
}

// relyingFor answers the port and its configuration for the provider: the SAML service provider
// for a SAML row, the OpenID Connect relying party for every other.
func (w OidcWriter) relyingFor(
	configured domain.IdentityProvider, opened secret.Secret,
) (provider.Port, provider.Config) {
	if !configured.IsSAML() {
		return w.Relying, relyingConfig(configured, opened, w.RedirectURL)
	}
	cfg := provider.Config{
		Issuer: configured.Issuer, ClientID: configured.ClientID, RedirectURL: w.SAMLACSURL,
		SSOURL: configured.SAMLSSOURL, Certificate: configured.SAMLCertificate,
	}
	if w.SAML == nil {
		return unwiredSAML{}, cfg
	}
	return w.SAML, cfg
}

// unwiredSAML refuses every SAML flow of a build wired without the service provider.
type unwiredSAML struct{}

func (unwiredSAML) Check(context.Context, string) error { return nil }
func (unwiredSAML) AuthorizationURL(context.Context, provider.Config, provider.Authorization) (string, error) {
	return "", shared.ErrValidation.WithDetail("identity_provider.not_configured")
}
func (unwiredSAML) Exchange(context.Context, provider.Config, provider.Exchange) (provider.Identity, error) {
	return provider.Identity{}, shared.ErrValidation.WithDetail("identity_provider.not_configured")
}

// ReceiveSAMLAnswer is the assertion consumer service's use case: keep the answer, mint the handoff.
type ReceiveSAMLAnswer struct {
	Session SessionWriter
	Answers repository.SAMLAnswers
}

// Execute keeps the answer on the live SAML sign-in the state names and answers the handoff code
// the callback presents. Unknown, spent, expired, another kind's flow, a second answer and an
// oversized one are one refusal, and keep nothing.
func (h ReceiveSAMLAnswer) Execute(ctx context.Context, rawState, answer string) (secret.Secret, error) {
	state, err := domain.ParseOidcFlowState(rawState)
	if err != nil || answer == "" || len(answer) > domain.MaxSAMLResponse {
		return secret.Secret{}, samlFlowNotFound()
	}
	material, err := h.Session.Entropy.Bytes(samlHandoffBytes)
	if err != nil {
		return secret.Secret{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	code := domain.SAMLHandoffPrefix + base64.RawURLEncoding.EncodeToString(material)
	kept := false
	if err := h.Session.UnitOfWork.Within(ctx, persistence.Scope{TenantID: state.TenantID()}, func(ctx context.Context) error {
		var err error
		kept, err = h.Answers.KeepSAMLAnswer(ctx, state, answer, domain.SAMLHandoffHash(code), h.Session.Clock.Now())
		return err
	}); err != nil {
		return secret.Secret{}, err
	}
	if !kept {
		return secret.Secret{}, samlFlowNotFound()
	}
	return secret.New(code), nil
}
