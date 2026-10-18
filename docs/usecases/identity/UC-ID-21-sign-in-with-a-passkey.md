---
id: UC-ID-21
title: Sign in with a passkey
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-02, P-05, P-11, P-13]
state: verified
tasks: [PK-03, PK-04]
checked_by: [core/application/service/identity/PasskeySignIn_test.go, core/application/service/identity/SignInRules_test.go, core/application/service/identity/LastWayIn_test.go, core/domain/model/identity/SignInPolicy_test.go, infrastructure/webauthn/Assertion_test.go, test/integration/passkey_test.go, test/integration/passkey_method_migration_test.go, apps/webapp/e2e/passkeysignin.test.mjs, apps/webapp/e2e/signinsettings.test.mjs, docs/evidence/PK-03-2026-10-17.md, core/application/service/identity/KeyFactor_test.go, core/application/service/identity/StepUpPasskey_test.go, test/integration/passkey_proof_test.go, apps/webapp/e2e/keyfactor.test.mjs, docs/evidence/PK-04-2026-10-17.md]
---

# Sign in with a passkey

## Goal

A person who holds a passkey signs in with it alone — no password, no code — confirmed on their own
device, at their workspace's address; a security key without its own PIN serves as the second
factor after the password, and either one gives the fresh proof a protected action asks for.

## Story

Ada opens her workspace's address. As she focuses the address field, the browser offers "Ada —
Passkey"; she picks it, touches the fingerprint sensor, and is signed in, although her workspace
requires a second factor of everyone. On a borrowed laptop without her passkey she types her
password and is asked for the second factor; she plugs in her security key, which has no PIN, and
touches it. Later, changing her workspace's sign-in rules asks her to prove it is her again; she
confirms with her passkey.

## How to check

1. Where the browser can use passkeys, the address field offers this workspace's passkeys when it
   is focused, and a *Sign in with a passkey* button asks the device directly; where the browser
   cannot, neither appears.
2. A passkey confirmed on the device (fingerprint, face or device PIN) opens a session without a
   password and without a code — also when the workspace requires a second factor.
3. Only passkeys made at this workspace's address are offered and accepted: a passkey the same
   person made in another workspace of the same installation is not, in single and in multi mode.
4. An unknown or removed passkey, one of a disabled or suspended account, and a failed confirmation
   produce the same sentence as a wrong password (UC-ID-01 check 4) and count as a failed attempt; a
   device prompt the person cancels shows no error and leaves focus on the card.
5. A security key confirmed without a PIN or fingerprint does not open the account alone; after the
   password, the second-factor step offers *Use my security key* beside the code, and the key
   completes the sign-in.
6. A fresh proof for a protected action can be given with a passkey or a security key the account
   holds.
7. The session list says that a session was opened with a passkey.
8. The workspace's sign-in rules carry a switch for signing in with a passkey; switched off, the
   card offers no passkey and the server refuses one, while security keys still serve as the second
   factor and as the fresh proof.
9. The passkey button and the security-key choice are reachable by keyboard, carry a name a screen
   reader announces, and the waiting for the device is announced once.

## Where it ends

* The web app in a browser only. The installed shells reach a passkey another way, because a
  shell's origin is not the workspace's address; that is milestone F7's.
* No account without a password or a provider: a passkey is added to an account that already has a
  way in (UC-ID-22), and the reset and the recovery codes stay as they are.
* No check of the authenticator's maker and no list of allowed models.
* A workspace cannot require passkeys and refuse codes (decided 2026-10-15, for later).
* A workspace that changes its address loses the passkeys made at the old one (UC-INS-15).
* Scripts, `hubctl` and MCP clients sign in with tokens, not passkeys.
