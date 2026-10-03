---
id: UC-ID-02
title: Prove it is me with a second factor when I sign in
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-11, P-12, P-13]
state: built
tasks: [H-02, SI-13, SI-14, SI-15, SC-03, SC-09, SC-17, SC-18]
checked_by: [apps/webapp/e2e/signin.test.mjs, apps/webapp/src/lib/signin/expiry.test.ts, apps/webapp/src/lib/data/recoverynote.test.ts, locales/Terms_test.go]
---

# Prove it is me with a second factor when I sign in

## Goal

A person whose account has a second factor finishes signing in with the code from their
authenticator — or, when the authenticator is gone, with one of their recovery codes — without
retyping anything and without guessing how long they have.

## Story

After a correct password the card moves to *Second factor*: "Signing in as jerome@example.eu —
Not you?", a code field with six places, and the time this step still waits. The code can be
typed or pasted. *I do not have my authenticator* switches the field to a recovery code: four
groups of four letters and digits, pasted with or without dashes. After a recovery code, the first
page says how many codes remain and leads to setting the authenticator up again.

## How to check

1. The step shows the identity line with *Not you?*, which returns to step one with the address
   kept.
2. The remaining time is visible and counts down; when it runs out the card says so and returns to
   step one.
3. The authenticator code field accepts six digits typed or pasted, shows a numeric keyboard on a
   phone, and does not submit by itself.
4. The recovery code field accepts the code **as it was shown** — 16 letters and digits, with or
   without dashes and spaces, pasted in one go — and shows a text keyboard on a phone.
5. A wrong code gets one sentence and an emptied field; the step does not restart.
6. After signing in with a recovery code, the first page shows a banner with the number of codes
   left and a link to set the authenticator up again.
7. The step is titled with the same words the profile uses for the feature ("Second factor").

## Where it ends

* No "trust this device for 30 days": the workspace's session rules decide how long a session lives.
* No SMS or mail codes ([NG-sms](../../vision/non-goals.md), [NG-magic-link](../../vision/non-goals.md)).
* No second factor after a provider sign-in: the provider is trusted as a whole, or not configured.
