---
id: UC-ID-01
title: Sign in with my address and password
context: identity
actors: [PE-person, PE-member, PE-guest, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-12, P-13]
state: built
tasks: [H-01, SI-13, SI-14]
checked_by: [apps/webapp/e2e/signin.test.mjs]
---

# Sign in with my address and password

## Goal

A person opens their workspace's address, types their address and password, and is in — on a
screen that shows nothing they cannot use, tells them in one sentence when something is wrong, and
never reveals whether an address has an account.

## Story

Signed out, there is no app bar and no menu: one card with the wordmark, the workspace's address
beside it (a phishing anchor, not a choice), the address and password fields, *Forgot your
password?*, and — only where the workspace has one — a button per sign-in provider. The footer
carries the legal links the operator or the workspace set. The password field has an eye to show
what was typed. A wrong address or password gets one sentence; the password field is emptied and
focused, the address stays.

## How to check

1. The signed-out screen has no app bar, no navigation and no account menu; the card shows the
   workspace's host beside the wordmark.
2. Focus is in the address field when the card opens; the address field carries
   `autocomplete="username webauthn"`.
3. The eye button shows and hides the password without changing it, and is announced as
   "Show password" / "Hide password" with its pressed state.
4. A wrong address, a wrong password, a suspended account and an address without an account all
   produce the same sentence and the same response; the password field is emptied and focused.
5. A provider button appears only when `/auth/sign-in-rules` names at least one provider; with
   none, there is no "or" and no provider button.
6. An ended session shows an information banner and restores the remembered page after signing in;
   an unreachable server shows a warning and the form stays usable.
7. Nothing on the card sends itself: pressing the button (or Enter) is the only thing that submits.

## Where it ends

* No "remember me" switch — the session rules of the workspace decide how long a session lives.
* The address is not remembered on the device; only the last used *method* is.
* Choosing a workspace on the sign-in screen is not offered: the host decides the workspace
  ([NG-global-identity](../../vision/non-goals.md)).
