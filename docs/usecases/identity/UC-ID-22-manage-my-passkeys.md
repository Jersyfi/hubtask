---
id: UC-ID-22
title: Manage my passkeys and security keys
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-11, P-13, P-16]
state: partial
tasks: [PK-01, PK-02, PK-04]
checked_by: [core/application/service/identity/Passkey_test.go, core/domain/model/identity/Passkey_test.go, test/integration/passkey_test.go, infrastructure/webauthn/Registration_test.go, infrastructure/webauthn/Captured_test.go, apps/webapp/e2e/passkeys.test.mjs, docs/evidence/PK-01-2026-10-16.md, core/application/service/identity/PasskeyManage_test.go, core/application/service/identity/PasskeyFactor_test.go, core/application/service/identity/WayIn_test.go, core/application/service/notification/WayInChanged_test.go, core/domain/model/identity/SecondFactor_test.go, test/integration/passkey_factor_test.go, presentation/rest/PasskeyController_test.go, apps/webapp/src/lib/data/passkeys.test.ts, docs/evidence/PK-02-2026-10-16.md]
---

# Manage my passkeys and security keys

## Goal

A person adds a passkey or a security key for each of their devices and a spare, recognises each by
a name they chose, sees when each was last used, and removes one they no longer have — without ever
removing the last thing their workspace requires of them.

## Story

On the profile under *Password and sign-in*, Clara chooses *Add a passkey*, proves it is her, and
confirms on her iPhone. "iPhone" appears in the list, added today. She adds her Windows laptop and a
security key she keeps in a drawer, and names them. A year later the list says the laptop was last
used eight months ago; she replaced it, so she removes it after proving it is her again, and a mail
tells her that a way into her account was removed.

## How to check

1. Under *Password and sign-in*, the profile lists the person's passkeys and security keys, each
   with its name, the day it was added and the day it was last used; with none, it says so in
   words.
2. *Add a passkey* asks for a fresh proof, then for the device; the new one appears in the list
   with a name the person can change at once.
3. An account holds up to ten; at ten, *Add a passkey* is not offered and the profile says why.
4. Renaming changes only the name; removing asks for a fresh proof, and the removed passkey stops
   working at once.
5. When the workspace requires a second factor of this person, the last passkey, security key or
   authenticator app the account holds is not offered for removal; the profile says the workspace
   requires it.
6. The first passkey or security key of an account without recovery codes shows the ten recovery
   codes once, as setting up an authenticator does (UC-ID-03 check 2).
7. Adding or removing a passkey tells the person by mail, as every change to a way into their
   account does; the name the person gave appears nowhere but on the profile.
8. A person routed into setup during sign-in because the workspace requires a second factor may add
   a passkey or a security key instead of an authenticator app.

## Where it ends

* At most one authenticator app per account (UC-ID-03); passkeys and security keys up to ten.
* No administrator sees, adds or removes a person's passkeys. Somebody who lost every factor and
  every code is removed and invited again (`NG-weaker-recovery`).
* No maker's name or picture for a key: the person names it.
* Passkeys belong to the workspace's address; another workspace of the same person has its own.

## Today

* Check 1: not met in full — the last use is written once a passkey signs in or proves; PK-03, PK-04.
* Check 4: not met in full — a removed passkey is gone at once; that it then fails to sign is PK-03.
* Check 8: not met — the setup during sign-in offers the authenticator app only; PK-04.
