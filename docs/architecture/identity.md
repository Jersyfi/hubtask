# Identity and sign-in

The current rules for accounts and for every way into one: passwords over their lifetime, the
second factor and its recovery codes, sign-in providers and who they admit, invitations, sessions
and tokens, the step-up, the ways back in, and the levels that decide all of it. Where this document
and an ADR disagree, this document holds; the ADRs record why and when.

Two principles stand above every section:
[P-02](../vision/principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof) — an
account is opened only by its own strongest proof — and
[P-16](../vision/principles.md#p-16-nobody-is-locked-out) — nobody is locked out. Where they pull
against each other, P-02 wins: a lost second factor with lost recovery codes is not answered by a
weaker proof (§17).

Threats and their gates are in [security.md](./security.md) §4 and §13; authorisation (roles,
scopes, the one place permissions are checked) is in [security.md](./security.md) §5 and
[domain-model.md](./domain-model.md) §3.2; how a tenant is resolved is in
[multi-tenancy.md](./multi-tenancy.md) §3.

The section numbers are stable: code cites `identity.md §N`. A section that stops holding a rule
keeps its heading with a line saying where the rule went.

---

## 1. What this document holds

| Section | Topic |
|---|---|
| §2 | Accounts: kinds and states |
| §3 | The workspace is found before the credential |
| §4 | Ways in, and the steps of a sign-in |
| §5 | The password rule |
| §6 | Setting and changing a password |
| §7 | Forgetting a password |
| §8 | The second factor |
| §9 | Recovery codes |
| §10 | Sign-in providers and who they admit |
| §11 | Connecting a provider to an existing account |
| §12 | Invitations and activation |
| §13 | Managed accounts |
| §14 | Sessions |
| §15 | Credentials for machines and apps |
| §16 | The step-up |
| §17 | Nobody is locked out: the ways back in |
| §18 | The levels of the sign-in settings |
| §19 | Operators and the elevated session |
| §20 | What is recorded, and what never travels |
| §21 | Open points |

Decision records: [ADR-0005](../adr/ADR-0005-authn-authz.md) (local accounts, OIDC, tokens),
[ADR-0036](../adr/ADR-0036-oidc-token-verification.md) (the token verification library),
[ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md) (the sign-in rule and the
password lifetime), [ADR-0070](../adr/ADR-0070-the-instance-layer.md) (operators and the elevated
session), [ADR-0071](../adr/ADR-0071-provider-admission.md) (provider admission),
[ADR-0074](../adr/ADR-0074-managed-accounts.md) (managed accounts),
[ADR-0075](../adr/ADR-0075-step-up-with-what-the-account-holds.md) (step-up methods),
[ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md),
[ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) and
[ADR-0078](../adr/ADR-0078-the-ways-back-in.md) (withdrawal and the ways back in).

---

## 2. Accounts

| Rule | Detail |
|---|---|
| Accounts belong to one workspace | An account is a row of one tenant. The address is unique per tenant (`account_email_uq` on `(tenant_id, lower(email))`); the same address in two workspaces is two accounts. There is no global identity ([non-goals](../vision/non-goals.md)). |
| Two kinds | `USER` (a person) and `SERVICE_ACCOUNT` (a machine). The kind is recorded in every audit entry. A service account has no password, no second factor and no sign-in: it holds tokens only (§15), is bound to one tenant and has its own role. |
| States | `INVITED` (created, not yet activated, §12), `ACTIVE`, `DISABLED`, `RESTRICTED` (Art. 18: the person works; automatic processing of their data stops — not a lockout, [data-protection.md](./data-protection.md) §4) and `ANONYMIZED` (erased, authorship kept). Only `ACTIVE` and `RESTRICTED` may act. A disabled account, or any account of a suspended workspace, is refused at every door, including a provider's token exchange, not only at the first sign-in. |
| What an account holds | Any of: a password, one second factor with its recovery codes, and connected provider identities (`account_identity`, unique per provider and subject). `GET /accounts/me` answers `has_password` and `recovery_codes_remaining`, so a client offers only what applies. |
| A provider-only account | Holds no password. The password doors refuse it the way they refuse a service account; it never meets a password change or a password step-up. |

---

## 3. The workspace is found before the credential

Accounts are per tenant, so a sign-in resolves its workspace **before** any credential is checked:
the subdomain first, then the `X-Hubtask-Tenant` header, and in single mode the one row
([multi-tenancy.md](./multi-tenancy.md) §3). The header may confirm a tenant and never overrule one;
a contradiction between token and host is `403 tenant_mismatch`. Single mode is the case with one
row, not a second code path.

* Resolution answers one identifier or none, never a listing (`resolve_tenant`, a narrow
  `SECURITY DEFINER` function).
* A host no workspace answers at learns nothing: `/auth/sign-in-rules` answers it byte for byte
  what a workspace that has decided nothing answers, and the sign-in and the reset give their one
  answer (§4.3, §7), because which hosts hold workspaces is what a probe is after.
* A non-interactive credential carries its tenant inside itself (§15), because the lookup needs a
  tenant context before it can happen.
* A sign-in identifier is resolved to an account by address within that workspace — and, once
  managed accounts exist, by address or sign-in name (§13).

---

## 4. Ways in, and the steps of a sign-in

### 4.1 The ways in

A workspace's rule names its **methods**: `PASSWORD` and `OIDC` (each switched-on provider is one
way in). The list is the one control: a provider is switched on or off in the workspace's list of
ways to sign in and nowhere else (§10.6). Which methods resolve, and when the password opens anyway,
is §17.

| Rule | Detail |
|---|---|
| The password switch is honoured everywhere | Where the resolved methods leave the password out, every password door refuses: the sign-in, the reset's request and its completion, the pending steps that end in a session through a password. The refusal is the same for every address. Stored passwords are kept; switching the password back on restores them. The fallback of §17 is the one exception. |
| Before the password is switched off | The switch says how many people have never signed in through a provider (`GET /tenant/accounts-without-provider`; connected, invited and service accounts are not counted). |
| The last way in cannot be switched off | Inside a workspace the last way in is refused with `identity_provider.last_way_in`. |
| The client follows the rules | `GET /auth/sign-in-rules` (§5.4) names the methods and providers; the card offers exactly those and no provider button before the rules name one. |

### 4.2 The steps

A sign-in is a step machine the contract names. `POST /auth/sessions` answers `201` with the pair
(§14), or `202` with a **pending credential** and the step that is owed; the client draws exactly
the step the server names.

| Step (`methods` in the `202`) | Owed when | Completed by |
|---|---|---|
| `TOTP` | The account has a second factor on | A current code, or one recovery code (§8, §9) |
| `ENROLL` | The rule requires a factor of this person and they have none | Enrolment and its confirmation (§8) |
| `PASSWORD_CHANGE` | The password is right but no longer meets the rule (too weak for a tightened rule, older than `max_age_days`, set before `rotation_from`) | Setting a new password; confirming it *is* the sign-in (§6) |
| `LINK` | A provider's first arrival meets an account that already holds a credential | The account's own proof (§11) |

* The pending credential (`hbt_mfa_…`, an `auth_pending` row with a purpose) can do nothing but
  complete its step: five minutes, single use, every other route refuses it. Its `expires_at` is
  answered so the card can show the time left.
* The window is a security bound and is never extended; when it ends, the client returns to the
  first step with the address kept.
* Steps chain: a `LINK` continues into `TOTP` where the account has a factor, carrying the link.

### 4.3 Refusals and attempts

* **One refusal.** "Wrong password" and "no such account" are the same answer byte for byte
  (`auth.sign_in_failed`). The client adds no hint the server refused to give.
* **The attempt ledger** counts per account (the address as presented) **and** per IP class, whether
  or not the account exists; it stores only hashes. Three failures are free, then the delay doubles
  from one second to a ceiling of fifteen minutes. The delay is the lockout.
* **Every door counts.** A wrong code or recovery code at the second step, at the enrolment's
  confirmation, at a step-up, at the `LINK` step and at the authenticator replacement advances the
  ledger. The failure is recorded in a transaction of its own **after** the refusing one returns,
  so the refusal's rollback does not erase it.
* The auth routes sit behind their own rate-limit bucket (`HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE`),
  which sheds a guesser before any lookup.
* A provider that is unreachable degrades as [observability-reliability.md](./observability-reliability.md)
  §7 says: existing sessions continue and password sign-in keeps working where it is a way in.

---

## 5. The password rule

### 5.1 The switches

The rule is a policy object, not a constant. Every switch ships at what NIST SP 800-63B-4 advises;
each one above that carries the sentence of what turning it on costs.

| Group | Switch | Shipped value |
|---|---|---|
| Password | `min_length` | 12 |
| | `min_lowercase`, `min_uppercase`, `min_digits`, `min_symbols`, `min_classes` | 0 (off) |
| | `max_repeat` | 0 (off) |
| | `common_passwords` (the embedded list and the operator's file) | on |
| | `context_words` (address, display name, workspace name and host) | on |
| | `breach_check` | off |
| | `max_age_days` (expiry) | 0 (off) |
| | `history_count` | 0 (off) |
| | `min_age_hours` | 0 (off) |
| Second factor and methods | `mfa_required_for` (`NONE`, `ADMINS`, `EVERYONE`) | `NONE` |
| | `methods` | password and every configured provider |
| Sessions | `session_max_days` | 30 |
| | `session_idle_minutes` | 0 (off) |
| The action | `rotation_from` — a moment, set by "require a new password from everyone" | unset |
| Installation only | `blocklist_file` — a path on the operator's disk | unset |

* **Zero is off, everywhere**, and no number is nullable.
* The four classes are counted separately *and* as `min_classes` (n of four), by Unicode category:
  `Ω` is an uppercase letter, an emoji is "other". A script without letter case can never satisfy
  the upper/lowercase counts, and the switch says so.
* A password is normalised to **NFKC before it is counted and before it is hashed**, so the same
  password typed on two keyboards is one password.
* The common-password check compares the folded form, as a substring, against the embedded list
  (written in-house, short) and the operator's `blocklist_file`; which list refused a password is
  never said.
* Expiry is not the default and not recommended. It exists because PCI DSS 4.0 §8.3.9 requires it
  where a password is the only factor; the one-time rotation is an event with a reason, not a
  calendar.

### 5.2 The product's bounds

The product's constants, below which nobody goes, the operator included: `min_length` at least 8
and at most 128; a password at most 1024 characters; `history_count` at most 10;
`session_max_days` at most 30 (the refresh token's own horizon); `min_age_hours` at most 24;
`max_age_days`, where on, at least 1. How the levels combine is §18.

### 5.3 Hashing

Argon2id with `m=64 MiB, t=3, p=2`. The parameters travel in the hash string; at sign-in, where the
plaintext is in hand, a hash carrying older parameters is recomputed, so a review of the parameters takes effect without a job.

### 5.4 The rules are data

* `GET /auth/sign-in-rules` is public the way sign-in is (workspace resolved as in §3) and answers
  four things: the methods, the providers, what a password must meet, and the operator's legal
  links. It answers nothing a guesser could use: no expiry, history depth, timeouts or lists.
* Each rule is a message code with parameters (`auth.password_rule.*`); the same codes are what
  `field_errors[]` carries when the server refuses a password — one entry per violated rule.
* `POST /auth/password:check` answers what only the server knows — the lists, the breach corpus,
  the history, "not the one you have now" — as rule ids, never a sentence. It demands **the same
  proof the setting demands** (otherwise it is an oracle for blocklists and histories) and has its
  own rate-limit bucket. A client asks it once the local rules hold, after typing stops, once per
  value.
* `api/fixtures/password-rules.json` holds rules, passwords and the violations they produce; the Go
  test and the client's test both read it, so the two predictions cannot drift.
* A refused password gets **no banner**: the field is marked invalid and the rules list names each
  rule broken ([design-system.md](../design/design-system.md) §10). A new password is typed once,
  in one field with the eye.
* The new surface is announced by `features.sign_in_rules` in `/meta/capabilities`; a client that
  does not find it draws the screen it had before and calls none of these routes.

### 5.5 What is deliberately not offered

An administrator-supplied regular expression (a ReDoS vector, and no sentence names its fix);
keyboard or alphabet sequence rules; forbidding spaces; password hints and security questions; a
strength meter as a requirement; magic links as an everyday method; SMS codes. A reset performed by
an administrator exists only for a managed account (§13).

---

## 6. Setting and changing a password

**One use case sets every password** — `SetPassword`, with four doors: an invitation token (§12),
a reset token (§7), the pending credential of the `PASSWORD_CHANGE` step (§4.2), or a bearer with a
step-up (`POST /auth/password`). One policy check, one audit action (`account.password_changed`,
without the password and without a hint about its shape), one place where the history is written.
The same use case is what an identity-provider or instance configuration that sets a password must
call; no second path sets one.

| Rule | Detail |
|---|---|
| The step-up is the old password | There is no "current password" field beside it; asking for both is asking twice. |
| A change ends every other session | The caller's session stays; personal access tokens keep working (they are their own credentials). |
| History | `account_password_history` holds at most ten hashes, goes with the account, and refuses the last `history_count` passwords. |
| Minimum age | `min_age_hours` holds a password before it may be replaced; a reset ignores it. |
| `password_set_at` | Recorded on the account; it is what `max_age_days` and `rotation_from` compare. |
| Enforcement is a step, never a job | A password that no longer meets the rule meets `PASSWORD_CHANGE` at its next sign-in. No job walks accounts or tenants; a person whose password already meets the new rule never notices it changed. |
| Rotation | "Require a new password from everyone" writes `rotation_from` in one write: every password set before it meets `PASSWORD_CHANGE`, every session opened before it is refused on its next request (§14). |

---

## 7. Forgetting a password

| Rule | Detail |
|---|---|
| One answer for every address | `POST /auth/password:forgot` answers `202` byte for byte the same for an address with and without an account, and the same whatever mail is sent. |
| The token | An `auth_pending` row with purpose `RESET`: 32 bytes, hashed under its own purpose label, thirty minutes, single use. A new link spends the account's earlier unspent ones; so does a change of the password or the address by any other way. |
| The mail | Its own job and handler, not a notification record (no preference applies). Rendered from message codes. An unreachable mail server never fails the request. |
| The second factor still stands | `POST /auth/password:reset` sets the password and ends every session; where a factor is on it answers the `202` that asks for it, and the pair only after the code. Mailbox control is one proof; it does not replace the one the account demanded. |
| Which mail | Depends on what the account holds and what is open (table below). |

| The account | The workspace | The mail |
|---|---|---|
| Holds a password | Password is a way in, or open as a fallback (§17) | A reset link |
| Holds no password; its provider lets it in | — | "Use your organisation's provider" |
| Holds no password; no provider it is connected to lets it in | Password open (a way in, a fallback, or an operator's opening) | A link to **set** a first password |
| Holds a password but is connected to no provider switched on here, or only to an offer that ended, or holds no credential at all | Password switched off | A link to **connect** the workspace's provider (§11) |
| Any | A managed account without an address | No mail is possible; the screen says so (§13) |

---

## 8. The second factor

| Rule | Detail |
|---|---|
| TOTP, written in-house | RFC 6238 on `crypto/hmac`, `crypto/sha1` and `crypto/subtle` — no dependency. A 20-byte secret, six digits, a 30-second step. |
| Verification | A code verifies within one step either side of now, never twice for the same step, and attempts count in the ledger (§4.3). |
| Enrolment | The secret is sealed under its own purpose ([security.md](./security.md) §8); the provisioning URI and the ten recovery codes are shown **once**. The factor is armed only after a valid code — an unconfirmed enrolment protects nobody and locks nobody out. The QR code is drawn by the client. |
| Who must hold one | `mfa_required_for`: `NONE`, `ADMINS` (`OWNER` and `ADMIN` role holders) or `EVERYONE` (every person; never a service account). A person the rule covers who has none is routed into `ENROLL` at sign-in rather than into a session. `require_admin_totp` on the workspace is derived from `mfa_required_for` and read-only; a write of it is translated into a `mfa_required_for` change under the same step-up, lock and tightening checks. |
| Turning it off | Takes a step-up like every privileged action (§16), so a provider-only account can do it with a code, a recovery code or its provider. Where the resolved rule requires a factor of this person, turning it off is refused and the profile says why. The request body's `password` is still accepted, marked `deprecated`, and goes with the next major contract version. |
| Replacing the authenticator | Its own action, offered whenever a factor is on — also where the rule requires one. A step-up with any method the account holds, a new secret, confirmation with a code from the new app, then one atomic swap that answers ten new recovery codes. The old factor and codes stay valid until the confirmation, so there is never a moment without a factor. The replacement waits ten minutes as a second, unconfirmed enrolment. Audited as `auth.mfa_replaced`. |
| No secret travels | The sealed secret and every code appear in no log, metric, trace, audit entry or API answer after their single showing. MFA state does not travel to devices ([offline-sync.md](./offline-sync.md) §4.2). |

Passkeys are planned and not built; the contract is already shaped for them (`methods` is a list,
`signed_in_with` a closed set, recovery codes belong to the account).

---

## 9. Recovery codes

**Recovery codes belong to the account, not to a factor.** They have their own route, outlive a
factor being replaced, and are the fallback for whatever second factor the account holds.

* Ten codes, each 80 bits, shown as four groups of four base32 characters. A presented code is
  normalised (case, spaces, dashes) before it is hashed, so a code read off paper works as shown.
* Stored hashed; each works exactly once. Using one at sign-in or at a step-up burns it, is
  audited, and the answer says how many are left.
* `POST /auth/mfa/recovery:regenerate` takes a step-up: ten new codes, the old ten burned in the
  same transaction, audited as `auth.mfa_recovery_regenerated` (older entries carry
  `mfa.recovery_regenerated`, and a filter for the new name finds them too).
* `recovery_codes_remaining` is answered to the account's holder and nobody else, at sign-in and on
  `GET /accounts/me`; zero is answered as zero, and zero is the number to act on.
* Somebody with neither the app, nor a code, nor a password, nor a provider is not let through. An
  administrator removes and re-invites them (`NG-weaker-recovery`).

---

## 10. Sign-in providers and who they admit

### 10.1 The relying party

OpenID Connect, authorization code + PKCE, with `state` and `nonce`. ID tokens are verified by
`github.com/coreos/go-oidc/v3` on `golang.org/x/oauth2`, imported by `infrastructure/oidc` only
([ADR-0036](../adr/ADR-0036-oidc-token-verification.md)): signature against the provider's JWKS,
`iss` compared exactly, `aud`, `exp`/`iat`/`nbf` with at most 60 s skew, the nonce this installation
minted, an algorithm allowlist (RS/ES only, never `none`). Discovery and JWKS are fetched through
`GuardedClient`, cached, and refetched on an unknown `kid`; metadata is refetched hourly. The
tampered-token cases are threat T-13 ([security.md](./security.md) §4).

* **Configuration refuses** an issuer that is not `https`, cannot be reached, disagrees with its own
  metadata, or offers no signing algorithm this installation accepts — found where somebody is
  looking at the form, not at the first sign-in.
* **There is no installation-wide provider health probe.** It would have to enumerate every
  workspace's providers.
* The client secret is sealed, write-only, and never answered again.
* A session opened through a provider is held to the workspace's session rules like any other
  (§14), and `signed_in_with = OIDC` names the provider.

### 10.2 Providers in the plural, at two levels

`identity_provider` rows belong to a workspace or, with `tenant_id` NULL, to the installation. Every
workspace reads the installation's rows and writes none. An installation provider is **offered**
(`/admin/identity-providers`), and each workspace switches an offered provider on for itself in its
list of ways in.

### 10.3 Presets

| Preset | Directory claim | Authoritative for an address when | Mode allowed |
|---|---|---|---|
| `MICROSOFT` | `tid` | `xms_edov` is present **and** `true` (the `email` claim alone never is) | any |
| `GOOGLE` | `hd` | the address is at `gmail.com`/`googlemail.com`, or `hd` equals the address's domain | `INVITED_ONLY` only (a public issuer: anybody in the world holds an account there) |
| `GENERIC` | none | never — a self-hosted issuer included | any; at installation level `INVITED_ONLY` only |

Presets are data: the issuer or its pattern, the scopes, the particulars, and the registration
instructions as message codes carrying this installation's redirect URI.

**A templated issuer is substituted, then compared exactly.** Microsoft's multi-directory endpoints
(`/common`, `/organizations`) publish `https://login.microsoftonline.com/{tenantid}/v2.0`. The
token's `tid` is read before verification, substituted, and `iss` compared exactly; the signature
must still come from the template's key set. The substitution decides nothing — the directory list
does — so a templated issuer with no directory list is refused at configuration.

### 10.4 Admission: one axis, who comes in

An unverified address comes in nowhere. Then:

| Mode | Screen wording | Admits on the provider's word | Creates accounts |
|---|---|---|---|
| `INVITED_ONLY` | "Only people invited here" | A verified address the provider is **authoritative** for, meeting an account that already exists | No |
| `DOMAINS` | "Anyone from these organisations" | A preset with a directory claim: the directory is in `allowed_directories`. `GENERIC`: the address's domain is in `allowed_email_domains` | Yes |
| `ANY` | "Anyone this provider knows" | Every verified address | Yes |

* **A preset with a directory claim ignores `allowed_email_domains` for admission** — two lists that
  both admit are two doors, and the weaker decides. An empty list under `DOMAINS` admits nobody; a
  row with domains and no directories under such a preset admits nobody until a directory is
  named, and the screen says so.
* The mode and the lists decide who comes in **new**. An existing member who brings their own proof
  (§11) or an invitation's own link (§12) is not coming in new.
* An account created on arrival holds no role; an administrator still has to give access.
* A mode a build does not know admits nobody.
* Microsoft's personal-account directory is `9188040d-6c67-4c5b-b112-36a304b66dad`; a company's
  directory is its tenant GUID. Google directories are Workspace domains as `hd` answers them.

### 10.5 What a provider asks for

Configuring, changing, offering, withdrawing and removing a provider each take a step-up (§16).
Configuration reads accept `READ_CONFIGURATION`; writes demand `identity_provider:manage` and are
audited.

### 10.6 The switch lives in the list

A provider is created switched off and is switched on or off in the workspace's list of ways in
only. `PUT /identity-providers/{id}` refuses a change of `enabled` with
`identity_provider.switch_in_list` and accepts the same value (so a client that echoes it keeps
working); the field is `deprecated` and goes with the next major contract version. Withdrawing and
removing an **offered** provider are §17.3.

---

## 11. Connecting a provider to an existing account

The safeguard belongs to the act of connecting, not to the provider kind: whoever configures a
provider must never be able to open somebody else's account by asserting their address.

| Account at a first arrival | What connects it |
|---|---|
| Holds a credential — a password, a second factor, or an identity at another provider | **Its own proof** at the `LINK` step: the password, then the second factor if one is on. The pending credential carries which provider and subject it will connect. |
| Holds a password, in a workspace that switched the password off | A **mailbox proof plus a fresh sign-in**: *Forgot your password?* mails a link to connect the workspace's provider (purpose `CONNECT`: thirty minutes, single use, the newest wins). The link and a fresh sign-in at the provider together replace the password — never the second factor, which is still asked. No password is stored. |
| Holds no credential (invited, never signed in, or its provider removed) | The provider's word only where it is authoritative for the address (§10.3) — that is a mailbox proof. Otherwise the way back is the mailbox (§7). |
| Holds only another provider's identity, no password | Refused with a sentence naming the way in it has (`identity_provider.link_needs_mailbox` where no mailbox proof is possible yet). |

* **At a first, unauthenticated arrival the provider's verified address must equal the
  account's.** An identity with a different address (a guest, a changed address) is connected only
  from a session already signed in, with the account's own proof, from its settings — not built yet
  (§21).
* **Once connected, an identity is found by issuer and subject only**, never by address again. An
  account already bound to another subject is refused, never re-pointed.
* A provider's `email_verified` alone activates nothing and connects nothing.
* Under `DOMAINS`, the list decides who comes in new, not whether an existing member may connect
  with their own proof.
* A refused provider sign-in is recorded in a transaction of its own, so the refusal's rollback does
  not erase the trail entry an operator reads to see a provider turning people away.

---

## 12. Invitations and activation

* `POST /accounts:invite` creates an `INVITED` account and queues the mail. The redemption token
  (`hbt_inv_…`) is minted on invite, hashed under a purpose label, shown once, lives fourteen days,
  and is redeemed once.
* The mail links `<base>/redeem#token=…`: the token travels in the **fragment**, which no `Referer`
  and no server log sees, and the client removes it from the history entry before the first request
  leaves (§14.4).
* `POST /auth/invitations:redeem` sets the first password through `SetPassword` (§6) and activates
  the account; a second redemption is refused.
* **A provider activates an invited account only with a second proof**: the provider is
  authoritative for the address (§10.3), or the person arrives through the invitation's own link —
  the invitation bound to the provider flow on the server, single use, expiring, and not spent by an
  arrival that fails. Under `INVITED_ONLY`, an arrival through the link is admitted even from a
  provider that is not authoritative for the address, for that invited account only and with the
  verified address equal to the invited one. Unknown, expired, accepted and foreign invitations are
  one refusal.
* Inviting without a mail server (copying the link), offering providers on the invitation card, and
  agreeing to terms at invitation are not built (§21).

---

## 13. Managed accounts

*Decided ([ADR-0074](../adr/ADR-0074-managed-accounts.md)); not built
([UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md)).*

* An owner or administrator creates an account with a **sign-in name** instead of an address:
  unique in the workspace, 3 to 32 characters, no `@`, compared case-insensitively after
  normalisation. Otherwise an ordinary `USER` account.
* Hubtask draws the start password and shows it once; the administrator never chooses it. The first
  sign-in is routed into `PASSWORD_CHANGE`.
* Where a workspace has managed accounts the first field reads "Email address or sign-in name"; the
  refusal and the ledgers are those of every sign-in (§4.3). A second factor may be set up and, where
  the rule requires one, must be.
* **The one exception to "an administrator does not recover an account":** for a managed account
  only, an owner or administrator may issue a new start password — after a step-up, recorded, ending
  every session of that account, routing the next sign-in into `PASSWORD_CHANGE`. An armed second
  factor is still demanded.
* What needs a mailbox says "not possible without a mailbox" where it would appear.
* Adding and confirming an address makes it an ordinary account; the sign-in name stays as a second
  identifier.
* *Managed accounts allowed* is a workspace setting, default on, with a default and a lock at the
  installation (§18).

---

## 14. Sessions

### 14.1 The pair

| Credential | Shape | Life | Stored |
|---|---|---|---|
| Access token | `hbt_sat_` + base64url of a 128-bit HMAC-SHA-256 tag, the expiry, and the tenant, session and account ids — signed under a key derived from `HUBTASK_SECRET_KEY` with its own purpose label | 15 minutes | Never; it verifies by its signature without a database read |
| Refresh token | `hbt_srt_<tenant>_<secret>` | 30 days, rotating | Hashed |

* A **session** is a row the person sees and revokes; both tokens hang off it and end with it. What
  the signature cannot answer — is the session alive, may the account act — is read from the row on
  every request.
* **Rotation and reuse.** Every refresh replaces the refresh token and slides the session's horizon.
  Presenting a retired refresh token invalidates the whole family, refuses the call, counts in
  `hubtask_auth_failures_total` with its own reason and raises the alert (T-01).
* A session records its client-binding hint — the user agent (at most 400 characters) and an IP
  class (IPv4 /24, IPv6 /48; the precise address never exists past the recording) — and how it was
  opened, `signed_in_with`: `PASSWORD`, `PASSWORD_TOTP`, `PASSWORD_RECOVERY`, `OIDC` (with the
  provider), `INVITATION` or `RESET`.
* There is no cookie session: every route takes a bearer.

### 14.2 Bounds

A session is refused on its next request when any of these holds — three comparisons in the check
the revocation list already performs:

* `created_at < rotation_from` (the workspace required a new password from everyone);
* it is older than `session_max_days`;
* it has been idle longer than `session_idle_minutes`.

The two bounds are written onto the session when it opens, not resolved per request: a bound
tightened later reaches new sessions only, and `rotation_from` is what ends the old ones in one
write.

### 14.3 Seeing and ending sessions

* `GET /auth/sessions` lists only sessions that are open by the same checks the next request makes
  — one definition of "open". No job tidies expired sessions (nothing may enumerate tenants); they
  age out through the `SESSION` retention kind.
* `DELETE /auth/sessions/{id}` ends one; `POST /auth/sessions:revoke-others` ends every session but
  the caller's (the screen's *Sign out everywhere else*); `DELETE /auth/sessions` ends every session
  including this one, for the API and `hubctl`.
* A password change ends every other session; a reset ends every session (§6, §7).

### 14.4 Credentials in a client

* The web app keeps the access and the refresh token in `sessionStorage`, never longer than the tab:
  what the refresh token buys it is rotation, not longevity. Signing out ends the session at the
  server and discards the local copy.
* A client exchanges the refresh token in **one place, once per refusal**: a `401` is answered by one
  refresh and one retry, and concurrent requests share the one exchange in flight — because
  presenting a retired token signs the person out everywhere.
* A credential is never put in a URL, a log, a message or the DOM beyond the field that shows it. One
  that arrives in the address — the invitation token in `/redeem`'s fragment, the provider's `code`
  and `state` at `/auth/callback` — is removed from the history entry before the first request
  leaves.
* A client does not shape-check a personal access token; the server decides.

---

## 15. Credentials for machines and apps

### 15.1 Personal access tokens

* `hbt_pat_<32 hex digits of the tenant>_<43 characters, base64url, 32 random bytes>`; scanning
  pattern `hbt_pat_[0-9a-f]{32}_[A-Za-z0-9_-]{43}`. The fixed prefix lets GitHub and GitLab secret
  scanning find a pasted token.
* The tenant travels inside because the lookup needs a tenant context before it can happen
  (`access_token` is behind row level security). Naming the wrong tenant gains nothing: the hash
  covers the whole string and is unique across the installation.
* Shown once; stored as HMAC-SHA-256 keyed on a pepper derived from `HUBTASK_SECRET_KEY` with a
  purpose label, so a hash from here cannot be replayed as a cursor or a feed token.
* A mandatory expiry of at most one year, scopes, and the last use visible. Asking for an `admin:*`
  scope takes a step-up (§16); `admin:tenants` is also checked against the operator register when
  minted and when exercised (§19).
* A personal access token is also the password a CalDAV client sends as HTTP Basic — only on the
  CalDAV tree ([security.md](./security.md) §4, T-22).

### 15.2 Service accounts

No sign-in, tokens only, bound to one tenant, with their own role. A service account may be an
operator (§19), because provisioning driven by a platform needs a credential that does not belong to
a person who may leave.

### 15.3 This installation as an OAuth2 provider

* Authorization code + PKCE only — no implicit, password or client-credentials grant. `S256` is the
  only challenge method.
* An administrator registers clients: a name, at most ten **exact-match** redirect URIs; a public
  client must use PKCE; a confidential client's secret (`hbt_ocs_…`) is stored hashed and shown once.
* **The authorization endpoint is headless.** `POST /oauth/authorize` is called by the signed-in
  person — never a token — naming the client, the exact redirect URI, the scopes and the challenge;
  the consent screen is a client of that route and adds nothing to it. **Consent is only ever to
  scopes from the catalogue** (`catalogue.Scopes()`), never to a parallel vocabulary.
* The code (`hbt_oac_…`) lives two minutes and is exchanged once; a replay is refused.
* Issued tokens are the session pair of §14 with the **grant** as their leash: the session carries
  the grant's scopes, and revoking the grant ends its sessions. Grants are listed per account with
  client, scopes and last use, beside the sessions.
* Every act through a grant is audited with the client as an actor attribute.

### 15.4 What a session carries

A session carries every declared scope except `admin:*` and the agent capability
(`catalogue.SessionScopes`). The one exception is the elevated session of §19. A grant session
carries only its grant's scopes.

---

## 16. The step-up

A fresh proof on the current session, demanded by a privileged action beyond the session itself.

### 16.1 Methods: whatever the account holds

| Method | Who holds it | How it proves |
|---|---|---|
| `PASSWORD` | An account with a password | The password, checked as at sign-in |
| `TOTP` | An account with a second factor on | A current code |
| `RECOVERY` | An account with a factor on and at least one code left | A recovery code, consumed as at sign-in |
| `PROVIDER` | An account with an identity at a provider switched on in its workspace | A fresh sign-in there |

`stepup.Methods` answers the methods the account holds, in this order; the refusal's
`params.methods` carries them and the client builds its prompt from that list alone.

**`PROVIDER` is verified twice.** The flow sends `prompt=login` and `max_age=0`, bound to the
session that asked (state and nonce as at sign-in). The proof holds only when the ID token's subject
is the identity already connected to this account (a different identity at the same provider is
refused, never connected), and `auth_time` is present, made after the flow left, no further ahead
than a minute's clock skew, and inside the step-up window. A provider that ignores `max_age` or omits
`auth_time` proves nothing fresh: `auth.step_up_provider_not_fresh`.

### 16.2 The proof and its use

* `POST /auth/step-up` (or `/auth/step-up:provider` and its return) answers a step-up token
  (`hbt_sup_…`), recorded on the session, valid for `HUBTASK_STEP_UP_WINDOW` (five minutes by
  default), and **consumed by the one privileged action** it is presented to. A second action needs a
  second proof. The privileged action does not learn how it was proven.
* Without one, a privileged action answers `403` with `auth.step_up_required` and the methods.
* A client asks for the proof only when the server answers `auth.step_up_required`, sends the token
  with the one retried request — the `X-Hubtask-Step-Up` header, or a restore's `step_up_token` in
  its body — and holds no token for a second action.
* Every step-up is audited with its method, never its credential; `RECOVERY` also records that a code
  was consumed.

### 16.3 Which actions demand it

**Each privileged operation declares its step-up on its use case descriptor** (`StepUp`, the
condition in words), and the architecture test fails a privileged operation that does not.

Today: requesting a workspace's deletion; granting or revoking the `OWNER` role; asking for a token
with an `admin:*` scope; writing the workspace's `sign_in_policy` (the one member of `PATCH /tenant`
that asks — a workspace's name, locale and zone do not); setting a password through the bearer door;
turning the second factor off, replacing it, and regenerating recovery codes; configuring, changing,
switching on or off, offering, withdrawing (including *Withdraw now*) and removing a provider;
raising a session to the installation (§19); an operator's opening of the password for one
workspace (§17.2); and the destructive restore modes (`REPLACE_TENANT`, `INSTANCE`;
[backup-restore.md](./backup-restore.md) §8). Decided and not built: a managed account's new start
password (§13) and the emergency access to a private hub
([ADR-0073](../adr/ADR-0073-private-hubs.md)).

---

## 17. Nobody is locked out: the ways back in

**No decision of the platform or of a level above a person takes away *whether* they can sign in.**
A withdrawal, a removal, a switched-off method or a tightened rule may change *how*. Where a door
would leave somebody with no way in, it either refuses (§4.1's last-way-in guard) or a way stays
open. Every way back is checked against P-02 as well: none of them skips a second factor.

### 17.1 The fallback

The password opens as the fallback **whenever the methods a workspace's rule resolves to leave no
way in that works** — the password is not among them and no provider is a way in there now —
**whatever the cause**: an offer that ended, an installation default or lock without the password, a
rescue lock lifted, a restore or an import, two administrators switching off the last two ways at
once.

* It opens for **every** account that holds a password, under the workspace's own password and
  second-factor rules.
* Every password sign-in it lets through is recorded in the workspace's trail with its cause
  (`NO_WAY_IN` or `OPERATOR`).
* The workspace's administrators see it on the sign-in settings until they switch on a way in, which
  ends it.
* An account without a password is mailed a link to set one (§7).
* It is read wherever the ways in are resolved; no job runs on the day an offer ends.

### 17.2 The operator opens the password for one workspace

For a provider that is switched on but broken — unreachable, or admitting nobody — the fallback
cannot see the problem. An operator then opens the password for **one named workspace**
(`POST /admin/tenants/{id}:open-password`):

* 24 hours by default, at most seven days, counted from the opening; a second opening is a new one,
  not an extension. It can be closed early (`:close-password`) and ends on its own.
* The requester (free text, a ticket reference will do) and the reason are required and recorded in
  the workspace's trail and in the installation's journal; they live on the row only while the
  opening does.
* The workspace's administrators are told by mail when it opens and when it closes, and the sign-in
  settings show it with the requester and reason.
* It is the fallback with cause `OPERATOR`: it overrides the workspace's switch and any installation
  lock, and it reads and changes nothing of the workspace's content.
* It takes the operator register, `admin:tenants` and a step-up. A suspended workspace is refused
  (its people are refused before any password is asked).

### 17.3 Withdrawing and removing an offered provider

* **A count, not names.** An offered provider shows how many workspaces have it switched on,
  computed when it is read by a read-only database function that answers only the number. The
  installation never learns which.
* **Withdrawal is announced.** The operator names a date — fourteen days ahead by default, at least
  24 hours — and sees the count. Until then the provider works everywhere, every workspace that uses
  it shows its administrators on the sign-in settings that it ends on that date, and the operator can
  cancel. On the date the offer ends: switched off everywhere, connected identities left in place, so
  offering it again restores sign-in. The date is honoured where the offer is read; no job.
* **Withdraw now**, for a compromised provider, ends it at once behind a step-up and a confirmation
  that repeats the count (compared inside the transaction). A withdrawal is never blocked because
  somebody uses the provider.
* **Removal follows an ended offer.** `DELETE /admin/identity-providers/{id}` is refused while the
  offer stands and a workspace uses it (`identity_provider.withdraw_first`). Removal deletes the
  connections between people and the provider; offering it again does not restore them, and the
  dialog says so.

### 17.4 Where the ways back end

* A lost second factor with lost recovery codes: an administrator removes and re-invites.
* A workspace that switched the password off on purpose is honoured; if its provider breaks, the way
  back is the operator's opening (§17.2).
* An account without an address cannot receive a link; its way back is the managed account's new
  start password (§13, not built).
* A household without a mail server receives no link; its way back is the operator's opening or the
  instance file (§19).
* An installation with nobody able to run it: the way back is a command in the server binary
  ([UC-INS-03](../usecases/admin/UC-INS-03-get-back-into-a-locked-installation.md)) — not built
  (§21).

---

## 18. The levels of the sign-in settings

```
product bounds (§5.2)  →  the installation's default and lock  →  the workspace, where no lock lies
```

* The installation sets a **default** and a **lock** per switch (stored as instance settings,
  [multi-tenancy.md](./multi-tenancy.md) §4.1). Open means a workspace may tighten it; closed means
  the value applies and the workspace's control is switched off **with its value, the reason, and
  who set it** — never hidden.
* A lock carries its origin, `INSTANCE` or `PLAN` (plans have no writer yet), because "ask your
  administrator" and "ask your provider" are different sentences.
* **A workspace only ever tightens.** The server enforces it field by field and answers a refusal
  against the field; a screen offers no choice looser than the level above.
* The effective rule is resolved on read along `Effective(product, instance, plan, workspace)` —
  one function, so the sign-in path, the settings screen and the check route cannot disagree. A
  change is in force in the same second; nothing walks rows.
* **One rule, one place.** Every sign-in switch is set on one screen per level, which shows each
  rule's source and lock.
* Writing the workspace's rule takes a step-up (§16.3) and is audited with the value before and
  after.
* The installation's methods lock is also a lever: locking the methods with the password among them
  opens the password in **every** workspace until the lock is lifted. For one workspace the lever is
  §17.2.
* **Never a switch:** exporting one's own data, deleting it, asking what is held, language, time
  zone and accessibility. Rate limits and the lockout curve, the key ring, and a workspace's name,
  colour and start page do not move to the installation level; language and time zone may be
  defaulted there and never locked.
* The legal links (imprint, privacy, terms, accessibility statement) resolve
  `workspace → installation → nothing` and are answered by `/auth/sign-in-rules`; an installation
  with nothing set shows no line at all.

---

## 19. Operators and the elevated session

| Rule | Detail |
|---|---|
| The register | `operator` lists accounts (people or service accounts) that run this installation. It is checked when `admin:tenants` is minted **and** when it is exercised — a token minted before a removal stops working. |
| Reached through four functions | `is_operator`, `operator_register`, `add_operator`, `drop_operator`, each `SECURITY DEFINER` and narrow; the table has no row-level policy and no grant to the application role, so no workspace can enumerate the operators. |
| The empty register | Answers *yes* only for an active `OWNER` (tenant scope) on an installation with **exactly one** workspace — the private installation, whose owner is its operator. With more than one workspace the register has to be filled. |
| The last operator | Cannot be removed; `drop_operator` deletes only while more than one remains, in the statement. |
| Elevation | A registered operator raises **their own session** to `admin:tenants` with a step-up (`POST /auth/sessions:elevate`): one hour, never sliding, not renewable (a second hour needs a second proof), bound to that session and ending with it or with a sign-out everywhere. The register is read again on every request, so an operator removed mid-hour loses the plane on the next call. Both ends are written to the installation's journal; the remaining time is on the screen. |
| A machine | Holds a personal access token with `admin:tenants`, as before. |
| The `/instance` area | Offered to an operator and to nobody else — **absent**, not disabled (`instance.reachable` in `/meta/capabilities`). It shows counts, states and limits, never the contents of a workspace. Whatever the API serves and the configuration permits, the area offers; the key ring is shown and never turned there. |
| Three doors, one API | The `/instance` area, `hubctl admin` and `HUBTASK_INSTANCE_FILE` (`seed` or `enforce`; `enforce` refuses the writing routes with the path in the refusal) go through the same use cases and the same refusals. |

The operator's levers on sign-in: the installation's defaults and locks (§18), offering, withdrawing
and removing providers (§17.3), and opening the password for one workspace (§17.2).

---

## 20. What is recorded, and what never travels

* No password, hash, token, session secret, TOTP secret, recovery code, client secret or reset link
  appears in a log, metric, trace, audit entry or error — proved by the redaction tests (rule 10,
  [security.md](./security.md) §4 T-18).
* Every sign-in, refusal, step-up, password change, factor change, provider change, withdrawal,
  fallback use and operator opening is an auditable action ([audit.md](./audit.md)); the second
  factor's actions form the `auth.mfa_*` family.
* The session's user agent, IP class and method, the reset token, the invitation token, the opening's
  requester and the account's provider identities are personal data with catalogue rows and deletion
  paths ([data-catalog.md](../privacy/data-catalog.md)).
* Every refused sign-in or refresh counts in `hubtask_auth_failures_total{reason}`, refresh-token
  reuse with its own reason; the use cases carry their metric and span like every other
  ([observability-reliability.md](./observability-reliability.md) §4).

---

## 21. Open points

| # | Point |
|---|---|
| I-1 | **A person is told when a way into their account changes** — a password set through a reset, a provider connected or disconnected — by mail as well as in the trail ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §6). Decided, not built; only the operator's opening is told today. |
| I-2 | **Connecting a provider from a signed-in session**, the way for an identity whose address differs from the account's ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §1). Decided, not built. |
| I-3 | **What newcomers get** under `DOMAINS` and `ANY` — nothing, a role or a group, applied in the creating transaction ([UC-ID-11](../usecases/identity/UC-ID-11-set-up-a-sign-in-provider-for-the-workspace.md)). Not built: a newcomer gets no role. |
| I-4 | **The first start and the locked installation**: a one-time setup code, `HUBTASK_OPERATORS`, and an operator restored by a command in the server binary ([UC-INS-01](../usecases/admin/UC-INS-01-start-a-fresh-installation.md), [UC-INS-03](../usecases/admin/UC-INS-03-get-back-into-a-locked-installation.md)). Not built. |
| I-5 | **Invitations without mail** (copying the link), the workspace's providers on the invitation card, and **terms of use** agreed at invitation and re-agreed by a `TERMS` step ([UC-ID-14](../usecases/identity/UC-ID-14-invite-people-with-a-role.md), [UC-ID-19](../usecases/identity/UC-ID-19-agree-to-the-terms.md)). Not built. |
| I-6 | **Managed accounts** (§13) and **private hubs** ([ADR-0073](../adr/ADR-0073-private-hubs.md)). Decided, not built. |
| I-7 | **A directory list under `INVITED_ONLY`** is stored and not read. [ADR-0071](../adr/ADR-0071-provider-admission.md) §4 intends it to narrow the mode further; the code does not. |
| I-8 | **The stored offer count** (`identity_provider.offered_workspaces`, `move_provider_offer`) is no longer read and is dropped by a migration one release after the counting function shipped (expand/contract). |
| I-9 | **The installation's own provider secret** is not re-sealed by a key rotation yet: the re-seal driver runs per workspace ([security.md](./security.md) §8.1). |
| I-10 | **A lever for one workspace whose own password switch is off on purpose** beyond §17.2 — whether one is needed is the owner's to decide. |
| I-11 | **Passkeys**, then plans, then custom domains, each its own decision. |
