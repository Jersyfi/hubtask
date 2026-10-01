---
id: UC-ID-06
title: See where I am signed in and end a session
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-11]
state: partial
tasks: [SI-08, SI-15, SC-09]
checked_by: [test/integration/session_test.go, presentation/rest/SessionProjection_test.go, apps/webapp/e2e/settings.test.mjs]
---

# See where I am signed in and end a session

## Goal

A person sees every device and browser their account is signed in on, how each one got in, and
can end any of them — or all others at once — when a device is lost.

## Story

The profile lists the sessions: device and browser, rough place (IP class), when it was last used,
and how it was opened ("Password and second factor", "Recovery code", "Microsoft"). This session is
marked. *End* ends one; *Sign out everywhere else* ends all others.

## How to check

1. Every open session of the account is listed with device, last use and the current one marked.
2. Each session says how it was opened, in words ("Password", "Password and second factor",
   "Recovery code", "Invitation", "Reset", the provider's name).
3. Ending a session makes its next request fail with the "your session ended" sentence.
4. *Sign out everywhere else* leaves exactly the current session.
5. A session that outlived the workspace's session rules (maximum age, idle time, a required new
   password) is not listed as open.

## Where it ends

* Administrators do not see or end other people's sessions here; removing a person is the
  administrator's tool.
* No location lookup beyond the IP class the session already stores.

## Today

Check 2 holds since SC-09: each session says how it was opened, the provider by its name
(`TestAProviderSessionNamesItsProviderInItsOwnTenantOnly`, the sessions walk). Found while checking
the rest, tracked in issue #1104:

* **Check 5 fails:** the list drops only revoked and expired sessions. One past the workspace's
  maximum age, its idle time, or a required new password is refused on its next request but still
  listed as open.
