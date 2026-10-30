---
id: UC-ID-30
title: Sign in through our SAML directory
context: identity
actors: [PE-admin, PE-member]
deployments: [D3, D4, D6, D7]
serves: [P-01, P-02, P-12, P-16]
state: verified
tasks: []
checked_by: [core/domain/model/identity/SamlProvider_test.go, infrastructure/saml/Provider_test.go, core/application/service/identity/SamlSignIn_test.go, presentation/rest/SamlController_test.go]
---

# Sign in through our SAML directory

## Goal

A company whose directory speaks SAML 2.0 and not OpenID Connect - an older ADFS, a Shibboleth, an
Okta or Entra ID application set up for SAML - lets its people sign in to the workspace through it,
with the same admission, the same first arrival and the same session as any other provider, and
without an assertion that was changed, replayed, meant for another service or signed by somebody
else ever opening an account.

## Story

The IT administrator chooses *A SAML 2.0 provider* among the workspace's ways in, registers Hubtask
at the directory from the metadata address the form shows, and pastes back the directory's entity
ID, its single sign-on address and its signing certificate. Lena presses the button, signs in at
her company's page and arrives in the workspace; the next time, the same account is found again.

## How to check

1. A workspace's provider of kind `SAML` is saved with an entity ID compared exactly (an `http:`
   or `urn:` one included, its trailing slash kept), an https single sign-on address and one or
   two certificates valid now; anything else is refused with `identity_provider.issuer_invalid`,
   `identity_provider.saml_sso_url_invalid`, `identity_provider.saml_certificate_invalid` or
   `identity_provider.saml_certificate_expired`, a secret sent with it with
   `identity_provider.saml_no_secret`, the two fields on another kind with
   `identity_provider.saml_only`, and offering one from the installation with
   `identity_provider.saml_workspace_only`.
2. An answer becomes an identity only when its response or assertion carries a signature by a
   configured certificate over SHA-256 or stronger, answers this sign-in's request, names this
   workspace's entity ID as its audience and the assertion consumer service as its recipient,
   comes from the configured entity ID, reports success and is inside its time with a minute's
   skew. A changed, unsigned, SHA-1, encrypted, foreign-key, second-assertion or expired answer,
   and one with a document type declaration, is the one refusal `auth.identity_token_invalid`.
3. `POST /auth/saml:acs` keeps an answer once on the live SAML sign-in its `RelayState` names and
   sends the browser with `303` to the installation's `/auth/callback` with a single-use code and the
   state; a second answer, an unknown or spent state is refused with `auth.oidc_flow_not_found`.
   The callback signs in only with that code, and the person arrives as through any provider.
4. A step-up or a connection of an account through a SAML provider is refused with
   `identity_provider.saml_sign_in_only`.

## Where it ends

* Authentication requests are not signed; encrypted assertions, single logout and a sign-in begun
  at the directory (IdP-initiated) are not served.
* An address from a SAML provider is never authoritative: an existing account with its own
  credential is connected only after that credential, as with `GENERIC` (UC-ID-10).
* The installation does not offer a SAML provider to every workspace.
