---
id: UC-ID-03
title: Set up my second factor and keep my recovery codes
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-11, P-13]
state: partial
tasks: [H-02, SI-09, SI-15, SC-03, SC-06, SC-09, SC-16, SC-17]
checked_by: [core/application/service/identity/Mfa_test.go, core/application/service/identity/FactorRule_test.go, apps/webapp/e2e/signin.test.mjs, apps/webapp/e2e/settings.test.mjs]
---

# Set up my second factor and keep my recovery codes

## Goal

A person sets up an authenticator app, stores ten recovery codes somewhere safe, sees how many are
left, gets new ones when they run low — and is never offered to turn the factor off while their
workspace requires it.

## Story

On the profile under *Password and sign-in*, *Set up an authenticator* shows a QR code and the
secret, asks for one code to confirm, then shows the ten recovery codes once, with *Copy all ten*
and a box to tick before continuing. Later the profile says "7 of 10 recovery codes left" with
*New codes*. When the workspace requires a second factor, a person without one is led through the
same setup during sign-in, told why.

## How to check

1. Setup shows the QR code and the secret as text, and confirms with a code in the same code field
   the sign-in uses.
2. The ten codes are shown once, can be copied as one block (where the browser has a clipboard),
   and *Continue* stays unavailable until the person ticks that they stored them — both on the
   profile and when setup happens during sign-in.
3. The profile shows how many recovery codes remain; zero is shown in words, not only in colour.
4. *New codes* asks for a fresh proof, replaces all ten, and the old ones stop working at once.
5. When the workspace's rule requires a second factor of this person, the profile does **not**
   offer to turn it off; it says the workspace requires it.
6. When nothing requires it, turning it off asks for a fresh proof and removes the recovery codes
   with it.
7. A person routed into setup during sign-in sees why ("Your workspace requires a second factor")
   and the identity line.

## Where it ends

* One authenticator per account. Several authenticators, hardware keys and passkeys are the passkey
  milestone's.
* No download button for the codes; the clipboard is enough for a password manager.

## Today

Checks 1, 2 and 7 hold since SC-03: the same walk on the profile and during a forced sign-in
(`apps/webapp/e2e/secondfactor.mjs`, used by `settings.test.mjs` and `signin.test.mjs`). Check 5
holds since SC-06: the profile reads the workspace's rule and offers no way to turn off a factor it
requires (`FactorRule_test.go`, `settings.test.mjs`).

* **Check 6 fails for an account without a password:** turning the factor off asks for the
  password, so a person who signs in only through a provider cannot prove themselves for it. Since
  SC-09 the profile says so instead of offering a field they cannot fill; SC-16 makes the step-up
  accept whatever the account holds.
