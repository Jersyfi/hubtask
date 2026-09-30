---
id: UC-ID-09
title: Sign in with my Google or Microsoft account
context: identity
actors: [PE-person, PE-member, PE-guest]
deployments: [D3, D5, D6]
serves: [P-02, P-05, P-10]
state: built
tasks: [SI-10, SI-11, SI-14]
checked_by: [core/domain/model/identity/IdentityProviderPreset_test.go, core/domain/model/identity/ProviderAdmission_test.go]
---

# Sign in with my Google or Microsoft account

## Goal

A person who was invited — or who bought access through a provider's platform — signs in with the
personal Google or Microsoft account they already have, and nobody else with such an account gets
in just because they have one.

## Story

The card shows *Sign in with Google* with Google's mark in the neutral style every brand guideline
allows. The person picks their account at Google and is back, signed in. A stranger with a Google
account is told they were not invited here.

## How to check

1. A public provider (Google accounts, personal Microsoft accounts) can only be set to *Only
   invited people*; the setting shows it and cannot be changed.
2. A person whose invited address matches the address the provider vouches for is signed in and
   linked; the link is recorded in the trail.
3. A person with no account here is refused with the "not invited" sentence, and nothing is created.
4. The provider's mark is shown in its published neutral form, never recoloured, loaded from the
   bundle and not from the provider's servers.
5. Where the workspace or installation turned the provider off, no button appears.

## Where it ends

* Apple sign-in waits for its own task (its client secret is a signed, expiring key).
* Linking the provider to an account that already has a password is UC-ID-10.
