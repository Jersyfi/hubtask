# Identity and sign-in

Two principles stand above every section:
[P-02](../vision/principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof) — an
account is opened only by its own strongest proof — and
[P-16](../vision/principles.md#p-16-nobody-is-locked-out) — nobody is locked out. Where they pull
apart, P-02 wins: a lost second factor with lost recovery codes is not answered by a weaker
proof.

Threats and gates: [security.md](./security.md) §4 and §13; authorisation:
[security.md](./security.md) §5 and [domain-model.md](./domain-model.md) §3.2; tenant resolution:
[multi-tenancy.md](./multi-tenancy.md) §3.

---

## 1. What this document holds

Accounts, credentials, sessions, the step-up, the ways back in, settings' levels, operators.
Decision records: [ADR-0005](../adr/ADR-0005-authn-authz.md),
[ADR-0036](../adr/ADR-0036-oidc-token-verification.md),
[ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md),
[ADR-0070](../adr/ADR-0070-the-instance-layer.md), [ADR-0071](../adr/ADR-0071-provider-admission.md),
[ADR-0074](../adr/ADR-0074-managed-accounts.md),
[ADR-0075](../adr/ADR-0075-step-up-with-what-the-account-holds.md),
[ADR-0076](../adr/ADR-0076-withdrawing-an-offered-provider.md),
[ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md), [ADR-0078](../adr/ADR-0078-the-ways-back-in.md).

---

## 2. Accounts

| Rule | Detail |
|---|---|
| Accounts belong to one workspace | The address is unique per tenant (`account_email_uq` on `(tenant_id, lower(email))`); one address in two workspaces is two accounts, with no global identity ([non-goals](../vision/non-goals.md)). |
| Two kinds | `USER` and `SERVICE_ACCOUNT`, recorded in every audit entry. A service account holds tokens only (§15). |
| States | `INVITED` (§12), `ACTIVE`, `DISABLED`, `RESTRICTED` (Art. 18, [data-protection.md](./data-protection.md) §4), `ANONYMIZED` (erased, authorship kept). Only `ACTIVE` and `RESTRICTED` act; a disabled account, or one of a suspended workspace, is refused at every door, a provider's token exchange included. |
| What an account holds | Any of: a password, one second factor with recovery codes, provider identities (`account_identity`, unique per provider and subject). `GET /accounts/me` answers `has_password` and `recovery_codes_remaining`. |
| A provider-only account | No password: the password doors refuse it, and it never meets a password change or password step-up. |

---

## 3. The workspace is found before the credential

A sign-in resolves its workspace **before** any credential is checked: the subdomain, then the
`X-Hubtask-Tenant` header, in single mode the one row ([multi-tenancy.md](./multi-tenancy.md) §3).
The header may confirm a tenant, never overrule one; token and host contradicting is
`403 tenant_mismatch`.

* Resolution answers one identifier or none, never a listing (`resolve_tenant`, a narrow
  `SECURITY DEFINER` function).
* A host without a workspace learns nothing: `/auth/sign-in-rules` answers byte for byte what an
  undecided workspace answers; sign-in and reset give their one answer (§4.3, §7).
* A non-interactive credential carries its tenant inside itself (§15.1); the identifier is
  resolved by address, or sign-in name (§13), within the workspace.

---

## 4. Ways in, and the steps of a sign-in

### 4.1 The ways in

A workspace's rule names its **methods**: `PASSWORD` and `OIDC` (each switched-on provider is one
way in), switched in that list and nowhere else (§10.6). Which methods resolve, and when the
password opens anyway, is §17.

| Rule | Detail |
|---|---|
| The password switch is honoured everywhere | Where the resolved methods leave the password out, every password door refuses — sign-in, the reset's request and completion, pending steps ending in a session through a password — alike for every address, stored passwords kept. The fallback of §17 is the one exception. |
| Before the password is switched off | The switch shows how many people never signed in through a provider (`GET /tenant/accounts-without-provider`; connected, invited and service accounts not counted). |
| The last way in cannot be switched off | Refused with `identity_provider.last_way_in`. |
| The client follows the rules | `GET /auth/sign-in-rules` (§5.4) names the methods and providers; the card offers exactly those. |

### 4.2 The steps

`POST /auth/sessions` answers `201` with the pair (§14), or `202` with a **pending credential** and
the step owed; the client draws exactly the step the server names.

| Step (`methods` in the `202`) | Owed when | Completed by |
|---|---|---|
| `TOTP` | The account has a second factor on | A current code, or one recovery code (§8, §9) |
| `ENROLL` | The rule requires a factor of this person and they have none | Enrolment and its confirmation (§8) |
| `PASSWORD_CHANGE` | The password is right but no longer meets the rule (too weak, older than `max_age_days`, set before `rotation_from`) | Setting a new password; confirming it *is* the sign-in (§6) |
| `LINK` | A provider's first arrival meets an account that already holds a credential | The account's own proof (§11) |

* The pending credential (`hbt_mfa_…`, an `auth_pending` row with a purpose) only completes its
  step: five minutes (`expires_at` answered), single use, never extended; then the client returns
  to the first step, the address kept.
* Steps chain: a `LINK` continues into `TOTP` where the account has a factor.

### 4.3 Refusals and attempts

* **One refusal.** "Wrong password" and "no such account" are the same answer byte for byte
  (`auth.sign_in_failed`); the client adds no hint.
* **The attempt ledger** counts per account (the address as presented) **and** per IP class,
  whether or not the account exists, storing only hashes. Three failures are free, then the delay
  doubles from one second to fifteen minutes; the delay is the lockout.
* **Every door counts** — the second step, the enrolment's confirmation, a step-up, `LINK`, the
  authenticator replacement — each failure recorded in its own transaction **after** the refusing
  one returns, so the rollback does not erase it.
* The auth routes' own rate-limit bucket (`HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE`) sheds a guesser
  before any lookup.
* An unreachable provider degrades as
  [observability-reliability.md](./observability-reliability.md) §7 says.

---

## 5. The password rule

### 5.1 The switches

The rule is a policy object. Every switch ships at what NIST SP 800-63B-4 advises; each says what
turning it above that costs.

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
| The action | `rotation_from` — set by "require a new password from everyone" | unset |
| Installation only | `blocklist_file` — a path on the operator's disk | unset |

* **Zero is off, everywhere**; no number is nullable.
* The four classes count separately *and* as `min_classes` (n of four), by Unicode category (`Ω` is
  uppercase, an emoji "other"); the switch says a caseless script cannot meet the case counts.
* A password is normalised to **NFKC before it is counted and before it is hashed**.
* The common-password check compares the folded form, as a substring, against the embedded list and
  the operator's `blocklist_file`; which list refused is never said.
* Expiry exists for PCI DSS 4.0 §8.3.9 (a password as the only factor), not as a recommendation.

### 5.2 The product's bounds

Nobody goes below these, the operator included: `min_length` 8 to 128; a password at most 1024
characters; `history_count` at most 10; `session_max_days` at most 30 (the refresh token's
horizon); `min_age_hours` at most 24; `max_age_days`, where on, at least 1. How the levels combine
is §18.

### 5.3 Hashing

Argon2id with `m=64 MiB, t=3, p=2`. The parameters travel in the hash string; a hash with older
parameters is recomputed at sign-in, so a change needs no job.

### 5.4 The rules are data

* `GET /auth/sign-in-rules` is public like sign-in (workspace as in §3): the methods, the providers,
  what a password must meet and the legal links — nothing a guesser could use (no expiry, history
  depth, timeouts or lists).
* Each rule is a message code with parameters (`auth.password_rule.*`); a refused password carries
  one per violated rule in `field_errors[]`.
* `POST /auth/password:check` answers, as rule ids, what only the server knows — lists, breach
  corpus, history, "not the one you have now". It demands **the same proof the setting demands**
  (else it is an oracle) and has its own rate-limit bucket; a client asks once the local rules hold,
  after typing stops, once per value.
* `api/fixtures/password-rules.json` feeds both the Go and the client's test.
* A refused password gets no banner and a new one is typed once
  ([design-system.md](../design/design-system.md) §10).
* Without `features.sign_in_rules` in `/meta/capabilities` a client calls none of these routes.

### 5.5 What is deliberately not offered

An administrator-supplied regular expression (a ReDoS vector); keyboard or alphabet sequence rules;
forbidding spaces; hints and security questions; a strength meter as a requirement; magic links as
an everyday method; SMS codes. An administrator resets a password only for a managed account (§13).

---

## 6. Setting and changing a password

**One use case sets every password** — `SetPassword`, with four doors: an invitation token (§12), a
reset token (§7), the `PASSWORD_CHANGE` pending credential (§4.2), or a bearer with a step-up
(`POST /auth/password`). One policy check, one audit action (`account.password_changed`, no hint of
the password's shape), one place the history is written.

| Rule | Detail |
|---|---|
| The step-up is the old password | No separate "current password" field. |
| A change ends every other session | The caller's session and personal access tokens stay. |
| History | `account_password_history` holds at most ten hashes and refuses the last `history_count`. |
| Minimum age | `min_age_hours` holds a password before it may be replaced; a reset ignores it. |
| `password_set_at` | What `max_age_days` and `rotation_from` compare. |
| Enforcement is a step, never a job | A password no longer meeting the rule meets `PASSWORD_CHANGE` at its next sign-in. |
| Rotation | "Require a new password from everyone" writes `rotation_from` once: older passwords meet `PASSWORD_CHANGE`, older sessions are refused on their next request (§14.2). |

---

## 7. Forgetting a password

| Rule | Detail |
|---|---|
| One answer for every address | `POST /auth/password:forgot` answers the same `202` byte for byte, with or without an account, whatever mail is sent. |
| The token | An `auth_pending` row, purpose `RESET`: 32 bytes, hashed under its purpose label, thirty minutes, single use; a new link, or a change of password or address, spends the earlier ones. |
| The mail | Its own job from message codes, not a notification (no preference applies); an unreachable mail server never fails the request. |
| The second factor still stands | `POST /auth/password:reset` sets the password and ends every session; where a factor is on it answers the `202` asking for it, the pair only after the code. |

| The account | The workspace | The mail |
|---|---|---|
| Holds a password | Password a way in, or a fallback (§17) | A reset link |
| No password; its provider lets it in | — | "Use your organisation's provider" |
| No password; no connected provider lets it in | Password open (way in, fallback, operator's opening) | A link to **set** a first password |
| A password but no provider switched on here (or only an ended offer), or no credential at all | Password switched off | A link to **connect** the workspace's provider (§11) |
| A managed account without an address | Any | None; the screen says so (§13) |

---

## 8. The second factor

| Rule | Detail |
|---|---|
| TOTP, written in-house | RFC 6238 on `crypto/hmac`, `crypto/sha1`, `crypto/subtle`: a 20-byte secret, six digits, a 30-second step. |
| Verification | One step either side of now, never twice for one step; attempts count in the ledger (§4.3). |
| Enrolment | The secret is sealed under its own purpose ([security.md](./security.md) §8); the provisioning URI and ten recovery codes are shown **once**; the factor is armed only after a valid code. The client draws the QR code. |
| Who must hold one | `mfa_required_for`: `NONE`, `ADMINS` (`OWNER` and `ADMIN` holders) or `EVERYONE` (people, never service accounts). A covered person without one is routed into `ENROLL`. The workspace's `require_admin_totp` is derived and read-only; a write of it becomes a `mfa_required_for` change under the same checks. |
| Turning it off | A step-up (§16), so a provider-only account can do it; refused, the reason on the profile, where the rule requires a factor of this person. The body's `password` is `deprecated` until the next major contract version. |
| Replacing the authenticator | A step-up with any method held, a new secret confirmed by a code from the new app, one atomic swap answering ten new recovery codes; until then the old factor and codes hold, the replacement waiting ten minutes as an unconfirmed enrolment. Audited as `auth.mfa_replaced`. |
| No secret travels | The secret and every code appear nowhere after their single showing; MFA state does not travel to devices ([offline-sync.md](./offline-sync.md) §4.2). |

---

## 9. Recovery codes

**Recovery codes belong to the account, not to a factor**: their own route; they outlive a replaced
factor and back whatever second factor the account holds.

* Ten codes of 80 bits, shown as four groups of four base32 characters; a presented code is
  normalised (case, spaces, dashes) before hashing.
* Stored hashed, each once: using one at sign-in or a step-up burns it, is audited, and the answer
  says how many are left.
* `POST /auth/mfa/recovery:regenerate` takes a step-up: ten new codes, the old ten burned in the same
  transaction, audited as `auth.mfa_recovery_regenerated` (a filter for it also finds the older
  `mfa.recovery_regenerated`).
* `recovery_codes_remaining` is answered to the holder only, at sign-in and on `GET /accounts/me`.
* Somebody with neither the app, a code, a password nor a provider is not let through
  (`NG-weaker-recovery`, §17.4).

---

## 10. Sign-in providers and who they admit

### 10.1 The relying party

OpenID Connect, authorization code + PKCE, with `state` and `nonce`. ID tokens are verified by
`github.com/coreos/go-oidc/v3` on `golang.org/x/oauth2`, imported only by `infrastructure/oidc`
([ADR-0036](../adr/ADR-0036-oidc-token-verification.md)): signature against the JWKS, `iss` exact,
`aud`, `exp`/`iat`/`nbf` with at most 60 s skew, our nonce, RS/ES algorithms only (never `none`).
Discovery and JWKS come through `GuardedClient`, cached, refetched on an unknown `kid`, metadata
hourly (T-13, [security.md](./security.md) §4).

* **Configuration refuses** an issuer that is not `https`, unreachable, inconsistent with its own
  metadata, or without an accepted signing algorithm.
* No installation-wide provider health probe: it would enumerate every workspace's providers.
* The client secret is sealed and write-only; a provider session follows §14.

### 10.2 Providers in the plural, at two levels

`identity_provider` rows belong to a workspace or, with `tenant_id` NULL, to the installation, whose
rows every workspace reads and none writes. An installation provider is **offered**
(`/admin/identity-providers`); each workspace switches it on in its list of ways in.

### 10.3 Presets

| Preset | Directory claim | Authoritative for an address when | Mode allowed |
|---|---|---|---|
| `MICROSOFT` | `tid` | `xms_edov` is present **and** `true` (the `email` claim alone never is) | any |
| `GOOGLE` | `hd` | the address is at `gmail.com`/`googlemail.com`, or `hd` equals the address's domain | `INVITED_ONLY` only (a public issuer) |
| `GENERIC` | none | never — a self-hosted issuer included | any; at installation level `INVITED_ONLY` only |

Presets are data: the issuer or its pattern, scopes, particulars, and registration instructions as
message codes carrying this installation's redirect URI.

**A templated issuer is substituted, then compared exactly.** For Microsoft's
`https://login.microsoftonline.com/{tenantid}/v2.0` the token's `tid` is substituted and `iss`
compared exactly, the signature still from the template's key set. The directory list decides, so a
templated issuer without one is refused at configuration.

### 10.4 Admission: one axis, who comes in

An unverified address comes in nowhere. Then:

| Mode | Screen wording | Admits on the provider's word | Creates accounts |
|---|---|---|---|
| `INVITED_ONLY` | "Only people invited here" | A verified address the provider is **authoritative** for, meeting an account that already exists | No |
| `DOMAINS` | "Anyone from these organisations" | A preset with a directory claim: the directory is in `allowed_directories`. `GENERIC`: the address's domain is in `allowed_email_domains` | Yes |
| `ANY` | "Anyone this provider knows" | Every verified address | Yes |

* **A preset with a directory claim ignores `allowed_email_domains`** — two lists that both admit are
  two doors. An empty list under `DOMAINS` admits nobody; domains without directories under such a
  preset admit nobody until a directory is named, and the screen says so.
* The mode and lists decide who comes in **new**; a member bringing their own proof (§11) or an
  invitation's link (§12) is not new.
* An account created on arrival holds no role.
* A mode a build does not know admits nobody.
* Microsoft's personal-account directory is `9188040d-6c67-4c5b-b112-36a304b66dad`; a company's is
  its tenant GUID. Google directories are Workspace domains as `hd` answers them.

### 10.5 What a provider asks for

Every provider write — configuring, changing, offering, withdrawing, removing — takes a step-up
(§16), demands `identity_provider:manage` and is audited; reads accept `READ_CONFIGURATION`.

### 10.6 The switch lives in the list

A provider is created switched off and is switched in the workspace's list of ways in only.
`PUT /identity-providers/{id}` refuses a change of `enabled` with
`identity_provider.switch_in_list` and accepts the same value; the field is `deprecated` until the
next major contract version. Withdrawing and removing an offered provider are §17.3.

---

## 11. Connecting a provider to an existing account

Configuring a provider must never open somebody else's account by asserting their address; the
safeguard belongs to connecting, not to the provider kind.

| Account at a first arrival | What connects it |
|---|---|
| Holds a credential — a password, a second factor, or another provider's identity | **Its own proof** at the `LINK` step: the password, then the second factor if on. The pending credential carries provider and subject. |
| Holds a password, the workspace switched the password off | **A mailbox proof plus a fresh provider sign-in**: *Forgot your password?* mails a connect link (purpose `CONNECT`: thirty minutes, single use, newest wins). They replace the password, never the second factor; no password is stored. |
| Holds no credential (invited, never signed in, or its provider removed) | The provider's word only where it is authoritative for the address (§10.3); otherwise the mailbox (§7). |
| Holds only another provider's identity | Refused, naming the way in it has (`identity_provider.link_needs_mailbox` where no mailbox proof is possible yet). |

* **At a first, unauthenticated arrival the verified address must equal the account's**; another
  address connects only from a signed-in session (§21 I-2).
* **Once connected, an identity is found by issuer and subject only**; one bound to another subject
  is refused, never re-pointed.
* `email_verified` alone activates and connects nothing.
* A refused provider sign-in is recorded in its own transaction, surviving the rollback.

---

## 12. Invitations and activation

* `POST /accounts:invite` creates an `INVITED` account and queues the mail. The token (`hbt_inv_…`)
  is hashed under a purpose label, shown once, lives fourteen days, redeemed once.
* The mail links `<base>/redeem#token=…`, the token in the **fragment** (§14.4);
  `POST /auth/invitations:redeem` sets the first password through `SetPassword` (§6) and activates
  the account.
* **A provider activates an invited account only with a second proof**: it is authoritative for the
  address (§10.3), or the person arrives through the invitation's own link — bound to the provider
  flow on the server, single use, expiring, not spent by a failed arrival; under `INVITED_ONLY` that
  link admits even a non-authoritative provider, for that account and the invited address only.
* Unknown, expired, accepted and foreign invitations are one refusal.
* Not built: §21 I-5.

---

## 13. Managed accounts

*Decided ([ADR-0074](../adr/ADR-0074-managed-accounts.md)), not built
([UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md)).*

* An owner or administrator creates a `USER` account with a **sign-in name** instead of an address:
  unique in the workspace, 3 to 32 characters, no `@`, compared case-insensitively after
  normalisation.
* Hubtask draws the start password and shows it once; the first sign-in meets `PASSWORD_CHANGE`.
  The first field reads "Email address or sign-in name"; §4.3 and the second factor rule apply.
* **The one exception to "an administrator does not recover an account":** an owner or
  administrator may issue a new start password — step-up, recorded, every session ended, next
  sign-in into `PASSWORD_CHANGE`, an armed factor still demanded.
* What needs a mailbox says "not possible without a mailbox"; a confirmed address makes it an
  ordinary account, the sign-in name kept.
* *Managed accounts allowed* is a workspace setting, default on, with an installation default and
  lock (§18).

---

## 14. Sessions

### 14.1 The pair

| Credential | Shape | Life | Stored |
|---|---|---|---|
| Access token | `hbt_sat_` + base64url of a 128-bit HMAC-SHA-256 tag, the expiry, and the tenant, session and account ids, signed as [security.md](./security.md) §8 says | 15 minutes | Never; it verifies by its signature |
| Refresh token | `hbt_srt_<tenant>_<secret>` | 30 days, rotating | Hashed |

* A **session** is a row the person sees and revokes; both tokens end with it, and every request
  reads from it whether it is alive and the account may act.
* **Rotation and reuse.** Every refresh replaces the refresh token and slides the horizon; a
  retired one presented invalidates the family and raises the alert (T-01).
* A session records the user agent (at most 400 characters), an IP class (IPv4 /24, IPv6 /48; never
  the address) and `signed_in_with`: `PASSWORD`, `PASSWORD_TOTP`, `PASSWORD_RECOVERY`, `OIDC` (with
  the provider), `INVITATION` or `RESET`.
* There is no cookie session: every route takes a bearer.

### 14.2 Bounds

A session is refused on its next request when `created_at < rotation_from`, when it is older than
`session_max_days`, or when it has been idle longer than `session_idle_minutes`. The two bounds are
written onto the session when it opens: a tightened bound reaches new sessions only, and
`rotation_from` ends the old ones in one write.

### 14.3 Seeing and ending sessions

* `GET /auth/sessions` lists only sessions open by the next request's checks; expired ones age out
  through the `SESSION` retention kind, no job.
* `DELETE /auth/sessions/{id}` ends one; `POST /auth/sessions:revoke-others` ends all but the
  caller's (*Sign out everywhere else*); `DELETE /auth/sessions` ends all, for the API and `hubctl`.

### 14.4 Credentials in a client

* The web app keeps both tokens in `sessionStorage`, never longer than the tab; signing out ends the
  session at the server and discards the copy.
* A client exchanges the refresh token in **one place, once per refusal**: a `401` gets one refresh
  and one retry, concurrent requests sharing the exchange in flight.
* A credential is never put in a URL, a log, a message or the DOM beyond its field. One arriving in
  the address — the invitation token in `/redeem`'s fragment, `code` and `state` at
  `/auth/callback` — is removed from the history entry before the first request leaves.
* A client does not shape-check a personal access token.

---

## 15. Credentials for machines and apps

### 15.1 Personal access tokens

* `hbt_pat_<32 hex digits of the tenant>_<43 characters, base64url, 32 random bytes>`; scanning
  pattern `hbt_pat_[0-9a-f]{32}_[A-Za-z0-9_-]{43}`, so GitHub and GitLab secret scanning find it.
* The tenant travels inside because the lookup runs behind row level security; naming the wrong one
  gains nothing, as the hash covers the whole string.
* Shown once; stored as an HMAC under its own purpose label ([security.md](./security.md) §8).
* A mandatory expiry of at most one year, scopes, the last use visible. An `admin:*` scope takes a
  step-up (§16); `admin:tenants` is checked against the operator register (§19).
* It is the HTTP Basic password of a CalDAV client, on the CalDAV tree only
  ([security.md](./security.md) §4, T-22).

### 15.2 Service accounts

No password, factor or sign-in; tokens only, one tenant, their own role. A service account may be
an operator (§19), so provisioning by a platform does not depend on a person who may leave.

### 15.3 This installation as an OAuth2 provider

* Authorization code + PKCE only; `S256` is the only challenge method.
* An administrator registers clients: a name, at most ten **exact-match** redirect URIs; a
  confidential client's secret (`hbt_ocs_…`) is stored hashed and shown once.
* **The authorization endpoint is headless.** `POST /oauth/authorize` is called by the signed-in
  person — never a token — from the consent screen, which consents only ever to catalogue scopes
  (`catalogue.Scopes()`).
* The code (`hbt_oac_…`) lives two minutes and is exchanged once.
* Issued tokens are the session pair of §14 leashed to the **grant**: the session carries its
  scopes, revoking the grant ends its sessions, and every act through it is audited with the client
  as an actor attribute. Grants are listed per account with client, scopes and last use.

### 15.4 What a session carries

Every declared scope except `admin:*` and the agent capability (`catalogue.SessionScopes`), save for
the elevated session of §19. A grant session carries only its grant's scopes.

---

## 16. The step-up

A fresh proof on the current session, demanded by a privileged action.

### 16.1 Methods: whatever the account holds

| Method | Who holds it | How it proves |
|---|---|---|
| `PASSWORD` | An account with a password | The password, checked as at sign-in |
| `TOTP` | An account with a second factor on | A current code |
| `RECOVERY` | An account with a factor on and at least one code left | A recovery code, consumed |
| `PROVIDER` | An account with an identity at a provider switched on in its workspace | A fresh sign-in there |

`stepup.Methods` answers the methods held, in this order; the refusal's `params.methods` carries
them and the client builds its prompt from that list alone.

**`PROVIDER` is verified twice.** The flow sends `prompt=login` and `max_age=0`, bound to the
session that asked; the proof holds only when the ID token's subject is the identity connected to
this account (another is refused, never connected) and `auth_time` is present, after the flow left,
at most a minute ahead, and inside the step-up window — otherwise `auth.step_up_provider_not_fresh`.

### 16.2 The proof and its use

* `POST /auth/step-up` (or `/auth/step-up:provider` and its return) answers a step-up token
  (`hbt_sup_…`), recorded on the session, valid for `HUBTASK_STEP_UP_WINDOW` (five minutes by
  default), **consumed by the one privileged action** it is presented to, which does not learn how
  it was proven.
* Without one, a privileged action answers `403` `auth.step_up_required` with the methods; only then
  does a client ask, sending the token with the one retried request (the `X-Hubtask-Step-Up` header,
  or a restore's `step_up_token` in its body).
* Every step-up is audited with its method, never its credential; `RECOVERY` also records the
  consumed code.

### 16.3 Which actions demand it

**Each privileged operation declares its step-up on its use case descriptor** (`StepUp`); the
architecture test fails one that does not. The descriptors are the list: a workspace's deletion
request, granting or revoking `OWNER`, an `admin:*` token, the workspace's `sign_in_policy` (the one
member of `PATCH /tenant` that asks), and every write of §6 (bearer door), §8–§9, §10.5, §17.2, §19
and [backup-restore.md](./backup-restore.md) §8's destructive modes. Decided, not built: §13's new
start password and a private hub's emergency access ([ADR-0073](../adr/ADR-0073-private-hubs.md)).

---

## 17. Nobody is locked out: the ways back in

**No decision of the platform or of a level above a person takes away *whether* they can sign in**
([P-16](../vision/principles.md#p-16-nobody-is-locked-out)), only *how*. A door that would leave
somebody with no way in either refuses (§4.1's last-way-in guard) or a way stays open, and no way back skips a second factor (P-02).

### 17.1 The fallback

The password opens **whenever the resolved methods leave no way in that works** — the password is
not among them and no provider is a way in now — **whatever the cause**: an ended offer, an
installation default or lock, a lifted rescue lock, a restore or import, two administrators
switching off the last two ways at once.

* It opens for **every** account holding a password, under the workspace's password and factor
  rules; one without a password is mailed a link to set one (§7).
* Every sign-in through it is recorded in the workspace's trail with its cause (`NO_WAY_IN` or
  `OPERATOR`); the sign-in settings show it until a way in is switched on.
* It is read wherever the ways in are resolved; no job runs.

### 17.2 The operator opens the password for one workspace

The fallback cannot see a provider switched on but broken; for that an operator opens the password
for **one named workspace** (`POST /admin/tenants/{id}:open-password`):

* 24 hours by default, at most seven days, never extended (a second opening is new); it can be closed
  early (`:close-password`).
* Requester (free text) and reason are required; their texts go to the workspace's trail (read with
  `READ_CONFIGURATION`) and live on the row only while the opening does, the installation's journal
  recording only `requester_present` and `reason_present`.
* The workspace's administrators are mailed when it opens and closes; the sign-in settings show it
  with requester and reason.
* It is the fallback with cause `OPERATOR`, overriding the workspace's switch and any installation
  lock, touching no content. It takes the operator register, `admin:tenants` and a step-up; a
  suspended workspace is refused.

### 17.3 Withdrawing and removing an offered provider

* **A count, not names.** An offered provider shows how many workspaces have it switched on, from a
  read-only database function that answers only the number.
* **Withdrawal is announced.** The operator names a date — fourteen days ahead by default, at least
  24 hours — seeing the count. Until then the provider works, each workspace using it shows the
  date, and the operator can cancel; on the date (read, no job) it is switched off everywhere,
  identities kept for a later offer.
* **Withdraw now**, for a compromised provider, ends it at once behind a step-up and a confirmation
  repeating the count (compared in the transaction). Use never blocks a withdrawal.
* **Removal follows an ended offer**: refused while the offer stands and a workspace uses it
  (`identity_provider.withdraw_first`). It deletes the connections; offering again does not restore
  them, and the dialog says so.

### 17.4 Where the ways back end

* A lost second factor with lost recovery codes: an administrator removes and re-invites.
* A password switched off on purpose is honoured; a broken provider there: §17.2.
* No address: §13. No mail server: §17.2 or the instance file (§19).
* An installation nobody can run: a server command
  ([UC-INS-03](../usecases/admin/UC-INS-03-get-back-into-a-locked-installation.md)), not built.

---

## 18. The levels of the sign-in settings

```
product bounds (§5.2)  →  the installation's default and lock  →  the workspace, where no lock lies
```

* The installation sets a **default** and a **lock** per switch, as every instance setting
  ([multi-tenancy.md](./multi-tenancy.md) §4.1); a locked control shows **the value, the reason and
  who set it**. A lock's origin, `INSTANCE` or `PLAN` (no writer yet), tells "ask your
  administrator" from "ask your provider".
* **A workspace only ever tightens**, enforced field by field; a screen offers nothing looser than
  the level above.
* `Effective(product, instance, plan, workspace)` resolves the rule on read for the sign-in path,
  the settings screen and the check route, so a change is in force at once.
* Each level has one screen for every sign-in switch, showing source and lock.
* Writing the workspace's rule takes a step-up (§16.3) and is audited with before and after.
* Locking the installation's methods with the password among them opens the password in **every**
  workspace while locked; for one workspace the lever is §17.2.
* **Never a switch:** exporting, deleting or asking about one's own data, language, time zone,
  accessibility. Rate limits, the lockout curve, the key ring, and a workspace's name, colour and
  start page have no installation level; language and time zone may be defaulted there, never locked.
* The legal links (imprint, privacy, terms, accessibility statement) resolve
  `workspace → installation → nothing`, answered by `/auth/sign-in-rules`; none set, no line shown.

---

## 19. Operators and the elevated session

| Rule | Detail |
|---|---|
| The register | `operator` lists the accounts (people or service accounts) running this installation, checked when `admin:tenants` is minted **and** exercised. |
| Four functions | `is_operator`, `operator_register`, `add_operator`, `drop_operator`, narrow `SECURITY DEFINER`s; the table has no row policy and no grant to the application role. |
| The empty register | Answers *yes* only for an active `OWNER` on an installation with **exactly one** workspace. |
| The last operator | Cannot be removed: `drop_operator` deletes only while more than one remains. |
| Elevation | An operator raises **their own session** to `admin:tenants` with a step-up (`POST /auth/sessions:elevate`): one hour, never sliding or renewed, the register read on every request, both ends in the installation's journal. A machine holds a personal access token with `admin:tenants`. |
| The `/instance` area | **Absent**, not disabled, for anybody but an operator (`instance.reachable` in `/meta/capabilities`); it shows what [multi-tenancy.md](./multi-tenancy.md) §4.1 allows, the key ring shown, never turned there, and offers what the API serves and the configuration permits. |
| Three doors, one API | The area, `hubctl admin` and `HUBTASK_INSTANCE_FILE` share the use cases and refusals; under `enforce` the file refuses the writing routes, naming the path. |

---

## 20. What is recorded, and what never travels

* No password, hash, token, secret, recovery code or reset link appears in a log, metric, trace,
  audit entry or error (rule 10, [security.md](./security.md) §4 T-18).
* Every sign-in, refusal, step-up, credential or provider change, withdrawal, fallback use and
  operator opening is audited ([audit.md](./audit.md)); every refused sign-in or refresh counts in
  `hubtask_auth_failures_total{reason}`.
* The session's user agent, IP class and method, the reset and invitation tokens, the opening's
  requester and the provider identities are personal data in the
  [data catalogue](../privacy/data-catalog.md).

---

## 21. Open points

| # | Point |
|---|---|
| I-1 | Telling a person by mail when a way into their account changes ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §6); only the operator's opening is told today. |
| I-2 | Connecting a provider from a signed-in session, for a differing address ([ADR-0078](../adr/ADR-0078-the-ways-back-in.md) §1). Decided, not built. |
| I-3 | A role for newcomers under `DOMAINS` and `ANY` ([UC-ID-11](../usecases/identity/UC-ID-11-set-up-a-sign-in-provider-for-the-workspace.md)). Not built. |
| I-4 | The first start and the locked installation: a setup code, `HUBTASK_OPERATORS`, a server command ([UC-INS-01](../usecases/admin/UC-INS-01-start-a-fresh-installation.md), [UC-INS-03](../usecases/admin/UC-INS-03-get-back-into-a-locked-installation.md)). Not built. |
| I-5 | Invitations without mail, providers on the invitation card, terms of use with a `TERMS` step ([UC-ID-14](../usecases/identity/UC-ID-14-invite-people-with-a-role.md), [UC-ID-19](../usecases/identity/UC-ID-19-agree-to-the-terms.md)). Not built. |
| I-7 | A directory list under `INVITED_ONLY` is stored and not read; [ADR-0071](../adr/ADR-0071-provider-admission.md) §4 intends it to narrow the mode. |
| I-8 | `identity_provider.offered_workspaces` and `move_provider_offer` are unread; a contract migration drops them one release after the counting function. |
| I-9 | The installation's provider secret is not re-sealed by a key rotation ([security.md](./security.md) §16, S-6). |
| I-10 | Whether a lever beyond §17.2 is needed for a workspace whose password is off on purpose — the owner's to decide. |
| I-11 | Passkeys (the contract is shaped for them: `methods` a list, `signed_in_with` a closed set, recovery codes on the account), then plans, then custom domains. |
