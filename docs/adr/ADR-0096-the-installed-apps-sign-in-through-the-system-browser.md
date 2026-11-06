# ADR-0096 — The installed apps sign in through the system browser

**Status:** accepted · **Date:** 2026-10-27 · **Decided:** at the cut of
milestone F7, by the worker; the shells themselves by
[ADR-0031](./ADR-0031-tauri-app-shell.md) and [ADR-0084](./ADR-0084-the-first-majors-shells-cost-nothing-to-ship.md)

**Rule lives in:** [identity.md](../architecture/identity.md) §15.3 and §23.3,
[UC-SYN-07](../usecases/sync/UC-SYN-07-install-the-app-on-my-computer-or-phone.md)

## Context

A passkey belongs to the workspace's host: its RP ID is the main address and the server admits only
that origin (identity.md §23.3). A Tauri webview runs at an origin of its own (`tauri://localhost`,
`http://tauri.localhost`), so a passkey ceremony inside the app is refused, and identity.md left the
shells' path to F7. A provider's sign-in has the same problem in reverse: providers warn against
embedded webviews. Building a second set of sign-in screens for the app would duplicate every way in
the web app already has, with their rules.

## Decision

1. **The app signs in as a client of the installation's own OAuth2 provider** (identity.md §15.3):
   authorization code with PKCE (`S256`), opened in the **system browser** at the workspace's own
   host, where the workspace's sign-in card offers every way in it allows - a passkey and a provider
   included - under the rules they already have.
2. **The client is built in, not registered**: every workspace knows the first-party public client
   of the Hubtask apps without an administrator registering it; it has no secret. Its redirect is a
   loopback address on any port on the desktop (RFC 8252 §7.3) and the private-use scheme
   `eu.hubtask.app:/oauth` on Android (RFC 8252 §7.1); nothing else is accepted for it.
3. **The person confirms the device**: the consent screen names the app and the device, so a
   program on the same machine that opens the flow cannot take a session unseen. The grant carries a
   session's scopes (§15.4) and is listed, and revocable, with the account's grants.
4. **The tokens live in the platform's keystore**, never in the webview's storage, and a refresh is
   the app's one exchange per refusal (§14.4).
5. **A step-up in the app** offers the account's methods as the web app does; a passkey step-up
   goes through the system browser the same way.

## Consequences

* No ceremony ever runs at the webview's origin; §23.3's origin rule stands as it is.
* One app installation holds one grant per workspace; *Forget this workspace* in the app revokes it.
* The browser's own session at the workspace's host is the person's; the app does not reuse it.

## Alternatives

* **Platform association** (Digital Asset Links, `webauthn` related origins) so the webview may use
  the host's passkeys: it needs a file on every workspace's host naming the app's signing key, and
  WebView2 and WebKitGTK offer no such association at all.
* **Sign-in screens in the app, a passkey through the platform's credential API**: still needs the
  association above for the RP ID, and duplicates every way in.
* **An administrator registers the app per workspace**: a person could not install the app without
  asking somebody (P-09).
