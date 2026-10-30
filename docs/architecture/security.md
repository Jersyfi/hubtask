# Security Concept

Binding for all building blocks: the threat model, the security gates, hardening, cryptography and
secrets. Accounts, sign-in, sessions, tokens and the step-up live in [identity.md](./identity.md);
the tenant boundary in [multi-tenancy.md](./multi-tenancy.md). Decision records:
[ADR-0005](../adr/ADR-0005-authn-authz.md), [ADR-0010](../adr/ADR-0010-multi-tenancy.md),
[ADR-0015](../adr/ADR-0015-security-baseline.md).

---

## 1. Principles

| Principle | What it means in the code |
|---|---|
| **Defence in depth** | Every protective rule holds at two levels at least: the tenant boundary in the application layer *and* PostgreSQL RLS; permissions through the token scope *and* the RBAC check. |
| **Secure by default** | The unconfigured state is the safe one: AI off, no self-registration (an account is invited, or admitted by a provider a workspace configured), every sign-in switch at what NIST advises, CORS empty, outbound HTTP only by allowlist, no default for a secret. |
| **Fail closed** | A missing tenant context, a cold policy cache or an unreachable permission source is a rejection (`403`/`503`), never passage. |
| **Least privilege** | A database role without `BYPASSRLS`; tokens with minimal scope; automation rules run with their `run_as` rights; containers non-root and read-only. |
| **No security through obscurity** | The model is public and must hold against an attacker who reads the source. |
| **Security is a CI gate, not a feeling in review** | Every rule below has an automated test or a build check; a rule without evidence counts as absent. |

---

## 2. Assets and protection goals

| Asset | Confidentiality | Integrity | Availability | Particularity |
|---|---|---|---|---|
| Tenant data (items, comments, media) | High | High | High | The core promise; a cross-tenant leak is a critical incident |
| Credentials, tokens, secrets | Very high | High | Medium | Stored only hashed or encrypted |
| Integration credentials (calendar, webhook secrets, AI keys) | Very high | High | Medium | Envelope encryption, never in clear text in logs or the API |
| Audit trail | Medium | **Very high** | High | Append-only; tampering must be detectable |
| Automation rules | Medium | High | High | A tampered rule is an execution primitive |
| Operational metadata (metrics, traces) | Medium | Medium | Medium | Must contain no user content |

---

## 3. Trust boundaries

```mermaid
flowchart LR
  subgraph U[Untrusted]
    B[Browser / client]
    N[n8n / Zapier / scripts]
    A[AI agent via MCP]
    W[Inbound webhooks]
  end
  subgraph E[Semi-trusted]
    IDP[OIDC provider]
    EXT[External HTTP targets]
    OBJ[S3-compatible]
    SMTP[Mail relay]
    LLM[AI provider]
  end
  subgraph T[Trust zone]
    API[api] --- WRK[worker/scheduler/automation]
    API --- DB[(PostgreSQL)]
    WRK --- DB
  end
  B & N & A & W -->|TB-1: authentication, rate limit, validation| API
  IDP -->|TB-2: signature check, issuer/audience| API
  T -->|TB-3: SSRF guard, allowlist, timeouts| EXT & SMTP & LLM
  T -->|TB-4: presigned, not the app origin| OBJ
  API -->|TB-5: RLS, a role without BYPASSRLS| DB
```

Everything left of `api` is hostile until proven otherwise — our own AI agent and automation rules
included. Inside the trust zone the tenant boundary still applies (TB-5).

---

## 4. Threat model (STRIDE)

`T-xx` are referenceable threat IDs. Every row has a countermeasure **and** test evidence (§13).

| ID | Boundary | STRIDE | Threat | Countermeasure | Evidence |
|---|---|---|---|---|---|
| T-01 | TB-1 | Spoofing | A stolen access token is replayed | 15-minute access tokens, refresh rotation with reuse detection (the family invalidated, the alert raised), a client-binding hint on the session ([identity.md](./identity.md) §14) | Token replay integration test |
| T-02 | TB-1 | Spoofing | Credential stuffing / brute force | Argon2id; an attempt ledger per account **and** IP class advanced by every password and code door; one generic refusal; a second factor a workspace can require ([identity.md](./identity.md) §4.3, §8) | "Login enumeration" test; a per-door ledger test against the real database |
| T-03 | TB-1 | Tampering | A manipulated `tenant_id`/`container_id` in the request | The tenant **never** comes from the body, only from the token or host; RLS as the second level | Cross-tenant negative test suite |
| T-04 | TB-5 | Info disclosure | IDOR: another tenant's `item_id` in the URL | Authorisation in the application layer through scope inheritance; RLS hides foreign IDs → `404`, not `403` | A test per resource |
| T-05 | TB-1 | Info disclosure | Mass extraction through the query DSL | A field allowlist, a maximum filter depth, a capped `limit`, rate limit plus quota, a bounded `expand` depth | Fuzz test of the DSL |
| T-06 | TB-1 | Tampering | SQL injection through filter expressions | Only parameterised queries (`sqlc`); the DSL produces an AST → parameters; `fmt.Sprintf` in query building forbidden by `depguard` | Lint + fuzz |
| T-07 | TB-3 | Elevation | SSRF through a webhook or HTTP action | `GuardedClient`: resolve first, block RFC 1918/loopback/link-local/ULA, pin the checked IP, `http(s)` only, redirects re-checked, a timeout, a response size limit, an allowlist (mandatory in provider operation) | Unit test with malicious DNS |
| T-08 | TB-3 | Elevation | An automation rule as a "confused deputy" | Actions run with the rights of `run_as`, which can never exceed the author's rights at save time; revoking rights disables the rule | "Rights revocation" test |
| T-09 | Internal | DoS | An automation loop (A triggers B triggers A) | A causality chain of maximum depth 5, loop detection over the `causation_id` path, a throttle per rule, deactivation after *n* failed runs | Golden loop test |
| T-10 | TB-1 | Elevation | Prompt injection through item content against the MCP agent | User content reaches the model as **data**, never instruction; destructive MCP tools blocked by default; agent tokens with their own scope; every agent action audited with actor `AI_AGENT` | "Injected instruction" test case |
| T-11 | TB-4 | Info disclosure | Stored XSS through an upload (SVG/HTML) | Delivery only from a separate origin; `Content-Type` from sniffing, not the client's claim; only a short allowlist of inert image types inline, everything else — SVG and HTML included — as `Content-Disposition: attachment`; `Content-Security-Policy: sandbox` | Upload matrix test |
| T-12 | TB-1 | Tampering | CSRF against cookie sessions | There is no cookie session ([identity.md](./identity.md) §14); should one appear: `SameSite=Lax`, `Secure`, `HttpOnly` and a double-submit token. State changes never over `GET` | Test |
| T-13 | TB-2 | Spoofing | A manipulated ID token / wrong issuer | Full verification ([identity.md](./identity.md) §10.1): signature, exact `iss`, `aud`, `exp`, `nonce`, JWKS rotation, no `alg: none`, skew ≤ 60 s | Test with a tampered JWT |
| T-14 | TB-1 | Repudiation | A user disputes a deletion | Append-only `activity_entry` plus the audit with actor, time, `request_id` and a before/after diff; trash kept 30 days | Test |
| T-15 | Internal | Tampering | Tampering with the audit trail | The app role holds no `UPDATE`/`DELETE` on the audit tables, a trigger lock, and a hash chain per tenant ([audit.md](./audit.md) §3) | Test with the app role |
| T-16 | Supply chain | Tampering | A compromised dependency or build | `go.sum` pinning, `govulncheck`, a CycloneDX SBOM per release, signed images (cosign) with provenance, reproducible builds, no `curl \| sh` in CI | Release gate |
| T-17 | TB-1 | DoS | Large bodies / zip bombs / expensive regexes | A body limit per endpoint, an upload limit, no user-controlled regexes, `statement_timeout`, a request deadline | Load test + test |
| T-18 | Operations | Info disclosure | Secrets in logs, traces or error messages | A redaction layer in the logger, the `Secret` type with a masking `String()`, no `%+v` over config structs (lint), a "the log contains no token" test; the one secret written to the console on purpose, the first start's setup code, is T-24's and never passes through the logger | Test |
| T-19 | TB-1 | Spoofing | A forged inbound webhook | A 32-byte token per rule in the address, hashed under its purpose label, rotatable; every refusal one identical `404`; the run acts as `run_as`, the body enters CEL as data ([automation.md](./automation.md) §1.1) | Test |
| T-20 | Operations | Info disclosure | A backup or export containing another tenant | The export runs through the API's RLS path; the restore drill includes an isolation check; an archive is restored only from its own workspace, except the operator's new-workspace restore of a workspace this installation does not hold ([backup-restore.md](./backup-restore.md) §8.2) | Restore drill, BK-10 |
| T-21 | TB-1 | Info disclosure | A calendar feed URL leaks (a log, a `Referer`, a shared screen, a client's history, a synchronised device) | Revocable at once; stored only as an HMAC under its own purpose label; never logged, its type masking every print; one unique-hash seek, every refusal (unknown, revoked, view-less, owner-less) one identical `404`; its own rate-limit bucket; read as its **owner** at fetch time, so a lost membership narrows it and a disabled account silences it; the document holds a title, a date and a link, never notes; a data feed's token (UC-INT-15) is a feed of its own kind, refused by the calendar's routes and CalDAV as a calendar's is by the OData routes, its rows never carrying notes | The feed suite: the token refused as a bearer, the refusals compared byte for byte, the masking test, the golden `.ics` |
| T-22 | TB-1 | Tampering, Info disclosure | The CalDAV tree parses client XML and takes HTTP Basic: an entity bomb, a document naming another account's tree, a password guessed by a silently retrying client | `encoding/xml` (no DTD entity expanded, a document naming one refused) under the body limit; the rate-limit buckets shed a guesser before the token lookup; Basic **only** on the tree (refused before any lookup elsewhere), its password a personal access token, never the account password; a title is XML text, never markup; another account's tree is `404`; calendars read as the owner through the ICS feed's use case, carrying what the feed carries; a write is the ordinary use case through the registry behind `If-Match` (`428`/`412`), an unmodelled property refused by name; no `sync-token`, since one hiding deletions would lie | `presentation/calendar/CalDav_test.go`, `CalDavWrite_test.go`, `presentation/rest/Auth_test.go` |
| T-23 | TB-2 | Spoofing, Elevation | A provider asserts somebody else's address, connecting to their account and skipping its second factor | Connecting needs the account's own proof or a mailbox link plus a fresh sign-in, never skipping an armed factor; a provider is trusted only for addresses it hosts ([identity.md](./identity.md) §10.4, §11) | `OidcLinking_test.go` (`TestAProviderCannotOpenAnAccountThatHoldsAPasswordAndAFactor`), `OidcAdmission_test.go` |
| T-24 | TB-1, Operations | Spoofing, Elevation | The first start's setup code is guessed, or read by somebody who should not run the installation | Issued only while there is no workspace and nobody in the operator register; sixty bits, valid thirty minutes, spent on first use with every other code, stored only as a hash under its own purpose label; the setup route is in the auth rate class. The code is written once, as a banner, to the process's standard error - the one deliberate secret on the console, outside the redacting logger so that T-18's "no token in the log" test stands as it is; whoever reads the container's output can reach its database too. After the first workspace the way back is the machine's command, never a route (UC-INS-01, UC-INS-03) | `test/integration/first_start_test.go`, `cmd/server/FirstStart_test.go`, `core/application/service/identity/Setup_test.go` |
| T-25 | TB-1 | Elevation | Somebody who manages members hands out or takes away a role above their own - an administrator makes an account they control an owner, joins a group that holds `OWNER`, or unmakes an owner | A grant or a revocation needs `MANAGE_MEMBERS` at the scope **and** a role along its path at least as strong as the role at stake, decided in the authoriser for every door that places or takes a role (`service.MayGrant`, [domain-model.md](./domain-model.md) §3.2); a change of a group's members or its deletion is held to it for every role the group holds, under the group's row lock; refused with `access.role_above_own` and recorded; clients offer only the server's `grantable_roles` and a group's `members_changeable` | `core/domain/service/Authorization_test.go`, `core/application/service/access/AuthorizationService_test.go`, `core/application/service/identity/Group_test.go`, `test/integration/role_ceiling_test.go` |
| T-26 | TB-1 | Elevation | A purchase platform's credential reaches more of the installation than provisioning, or ends a workspace without anybody's proof | `admin:provisioning` admits only the verbs its use cases declare (`AlsoScope`, UC-INS-06 check 6), minted behind a step-up by a registered operator and dropped when the register no longer names the holder, like `admin:tenants`; no consent hands an app an `admin:*` scope; the step-up of the deletion and of the installation's subscriptions is waived only for a service account whose own register row is a service account's - never a person, an agent or the empty register's owner, and never on an unreadable register - and the typed name, the 30-day grace, the cancellation and the legal-hold refusal stay ([multi-tenancy.md](./multi-tenancy.md) §4.3, §5) | `test/security/provisioning_scope_test.go`, `core/application/service/admin/ProvisioningScope_test.go`, `Deletion_test.go`, `test/integration/tenant_deletion_test.go`, `core/application/service/identity/Oauth_test.go` |
| T-27 | TB-1 | DoS | Whoever holds a workspace's intake address makes things arrive in its jumble, and with *Prepare proposals for new jumble entries* on, each arrival spends the workspace's AI | Off by default and the workspace's own switch; the intake doors are capped and answer every refusal with one `404`; each arrival is asked about at most once (a job per entry, deduplicated), counted against `ai_arrival_proposals_per_day` (200 a day by default in both modes, [multi-tenancy.md](./multi-tenancy.md) §4) before the call and against the token budget of the source that answers; past either, nothing is sent and every other feature works; the queue's growth per kind is alert A-06 | `test/integration/arrival_proposals_flow_test.go` (the day's cap and the spent budget), `core/application/service/quota/Quota_test.go` |
| T-28 | TB-1 | Spoofing, Info disclosure | A workspace's own domain whose DNS still points at the installation after its owner gave it up, or a forged `Host` header, puts a stranger in front of a workspace's address or sends a person to a host nobody verified | *Built:* a host names a workspace only through a serving `tenant_host` row; a host that is neither that nor the installation's own names nothing - `404` before a credential, header or not, and `403 access.tenant_mismatch` after one - and the header speaks only at the installation's own host; the provider's return forwards only to the main address the server answers. *Decided, not built - milestone CD* ([ADR-0095](../adr/ADR-0095-a-workspaces-address-resolves-through-its-hosts.md)): an own domain is proven by a TXT mark and re-proven by a job, and turns `BROKEN` with the subdomain taking over when the proof is gone; the ingress gets a certificate only for a verified, unblocked host, asked on the operations port; every redirect and forward goes to the main address the server knows, never to a value in the request; a changed address frees the old host at once and answers it like an unknown one | CD-02, CD-03, CD-07 |
| T-29 | TB-1 | Spoofing, Tampering | A forged or replayed forge delivery links code to entries, or turns a merged pull request back | The hook's secret proven by the forge's HMAC signature of the body or GitLab's token, derived from `HUBTASK_SECRET_KEY` under its own purpose label and never stored; a delivery remembered three days by its identifier; every refusal one identical `404`; written as the hook's definer only while they hold WRITE_ITEMS on the hub, on the hub's own entries alone; a link follows the forge's own moment and a merge is never undone ([automation.md](./automation.md) §3.5) | `core/application/service/forge/Forge_test.go`, `infrastructure/security/CodeHookSecrets_test.go` |
| T-30 | TB-1 | Info disclosure, Elevation | A calendar push writes more than its owner may see, or into somebody else's calendar, or its grant is used for more than the calendar | The consent asks for the calendar's events alone; the grant sealed under the push's own purpose label, never answered; the feed read as its owner at every run, so a lost membership narrows it; a push listed, connected and disconnected by its owner alone, a sign-in state hashed, good once, for ten minutes and for whoever started it; an event holds a title, a moment and a link, never notes ([automation.md](./automation.md) §3.6) | `core/application/service/calendarpush/CalendarPush_test.go`, `infrastructure/calendarpush/Calendar_test.go` |

A new bounded context gets a short STRIDE analysis at design time, added here (Definition of Ready,
"security assessment").

---

## 5. Identity, sessions, permissions

| Topic | Rule |
|---|---|
| Permission check | Exactly one place: `core/application/service` through `AuthorizationService`; adapters and repositories never authorise; the architecture test fails a violation (SG-5). Roles and inheritance: [domain-model.md](./domain-model.md) §3.2 |
| Two bounds on every request | The role along the path **and** the credential's scopes; either alone refuses ([identity.md](./identity.md) §15) |
| Everything else | [identity.md](./identity.md): passwords §5–§7, second factor and recovery codes §8–§9, providers §10–§11 (T-13, T-23), sessions and client credentials §14, machine credentials and OAuth2 grants §15, the step-up §16 (declared on each privileged operation's descriptor, enforced by the architecture test), the ways back in §17, operators §19 |

---

## 6. Tenant isolation (summary)

Details in [multi-tenancy.md](./multi-tenancy.md). The security core:

* `hubtask_app` has **no** `BYPASSRLS` and no `SUPERUSER`; `FORCE ROW LEVEL SECURITY` on all tenant
  tables (SG-4 names the exception).
* `SET LOCAL app.tenant_id` is set in **one** place, the transaction wrapper; without it a query
  returns nothing or fails.
* Migration and maintenance roles are separate and used only by the migration job.
* **Every** repository method has a negative test with a foreign tenant; a method without one fails
  the CI gate (a count of methods against tests).

---

## 7. Input and output handling

* **Validation** at the edge (the OpenAPI schema, generated types) *and* in the domain (value
  objects with constructor invariants): the adapter validates form, the domain meaning.
* **Unicode** is normalised (NFC) before storage and comparison; homoglyph display names in
  invitations get a warning, not a block.
* **Output** is JSON; apart from a static error page the server renders no HTML. The web client is
  bound by the interface's `Content-Security-Policy` (§9).
* **Markdown in notes and comments** is stored raw and never rendered server-side; the client renders
  it through a mandatory sanitiser.
* **Inbound mail is hostile input.** Its HTML is stored as text, never rendered by the server; its
  sender is provenance, never an identity (the intake address is the credential); every bound is
  checked before allocation. A client draws it as text, never markup or a live link, and labels the
  sender as what the transport claimed.
* **CEL expressions** run under a time, depth and cost limit (`cel-go` cost budget), with no network
  or time functions beyond the ones provided.

---

## 8. Cryptography and secrets

| Purpose | Method |
|---|---|
| Password hash | Argon2id ([identity.md](./identity.md) §5.3) |
| Token storage | HMAC-SHA-256 keyed on a pepper derived from `HUBTASK_SECRET_KEY` (not in the database), under a purpose label per credential kind, so a hash of one kind cannot be replayed as another |
| Signed tokens | The session access token, cursors and media tokens: HMAC-SHA-256 under a key derived from `HUBTASK_SECRET_KEY` with their own purpose label; none is JOSE |
| Derived secrets | A forge hook's secret: HMAC-SHA-256 under a pepper derived from `HUBTASK_SECRET_KEY` with its own purpose label, over the workspace, the hook and a salt the hook stores; a forge signs with the secret itself, so a hash would not do, and derived it is in no store a rotation of the encryption keys must reach. A new salt is a new secret; another installation's key derives another one |
| Audit fingerprints | The hash of a sensitive value in the audit trail: HMAC-SHA-256 under a pepper derived from `HUBTASK_SECRET_KEY` with its own purpose label, over the workspace and the value, prefixed `hmac-sha256:` ([audit.md](./audit.md) §4). Comparable within one workspace and one secret; a guess cannot be checked without the secret |
| Integration credentials, webhook secrets, backup target credentials | AES-256-GCM envelope encryption: a data key **per value** (the master key only encrypts random keys, within GCM's per-key bound), wrapped by a master key from `HUBTASK_ENCRYPTION_KEYS` (§8.1) whose ID is stored. The ciphertext is bound to a caller-supplied purpose and cannot move between rows |
| Backup archives | AES-256-GCM under a key derived from the master key with HKDF-SHA256, bound to the target; no passphrase is accepted ([backup-restore.md](./backup-restore.md) §4) |
| Signatures on outbound webhooks | HMAC-SHA-256, a secret per subscription, the header `X-Hubtask-Signature` with a timestamp |
| Transport | TLS 1.2+ (target 1.3); mTLS inside the cluster optional; HSTS where TLS is terminated |
| Randomness | Only `crypto/rand` for tokens, IDs and nonces; the `RandomSource` port uses it in production |
| Home-grown crypto | Forbidden. Only `infrastructure/crypto` names a cipher: `gate-architecture` fails an import of `crypto/aes`, `crypto/cipher`, `crypto/hkdf` or `golang.org/x/crypto` anywhere else, except `golang.org/x/crypto/ssh` in `infrastructure/backupstorage` (the SFTP target). A small, closed protocol over standard primitives — TOTP, AWS SigV4, SFTP — is written in-house (§11) |

Secrets come only from environment variables or mounted secret files (the `HUBTASK_*_FILE`
convention). A secret has no default: if one is missing, the process does not start, with a message
code.

### 8.1 Rotating the master key

The master key stays in the environment, out of any database dump
([ADR-0045](../adr/ADR-0045-master-key-in-the-environment.md)). `HUBTASK_ENCRYPTION_KEYS` is a
ring, current first, and every key in it stays readable; since the master key wraps one data key per
value, a rotation is a configuration change. The procedure is drilled
([evidence](../archive/evidence/S-2-2026-09-04.md), [completion](../archive/evidence/S-2-2026-09-06.md)):

1. **Mint the new key**: at least 32 bytes (`openssl rand -base64 48`), an identifier of lower-case
   letters, digits and underscores.
2. **Put it first, keep the rest**: `HUBTASK_ENCRYPTION_KEYS=k2,k1` with both keys' material. New
   values seal under `k2`, `k1` values still open; dropping `k1` here is an outage.
3. **Re-seal**: `POST /admin/encryption:reseal` (`hubctl admin encryption reseal`) queues one round
   per workspace and one for the installation's own secrets, rewrapping each value's data key under
   `k2`, never decrypting the value. Asking again before a round ran queues nothing.
4. **Read the census, then retire the old key**: `GET /admin/encryption` (`hubctl admin encryption
   show`) counts the stored values naming each key, across every workspace and the installation's
   own level - each installation secret once - and by kind, never by workspace. Remove `k1` only at
   zero; earlier, its rows answer `crypto.unknown_key`, the census shows it `in_ring: false` with
   the kinds it still seals, and the operator's health report warns `crypto.key_missing` per kind
   (counted in the background at start, hourly and on every census read, and saying when) until it
   is put back and re-sealed. The census also counts the **backup archives** still kept under each
   key (`backup_archive`): an archive is never rewritten, so no re-seal moves it, and the count
   falls only as archives expire. A key therefore stays in the ring for as long as the archives
   written under it are kept, yearly generations included ([backup-restore.md](./backup-restore.md)
   §4).

**What a round touches.** The eight sealed places — the second factor, an identity provider's client
secret, the AI provider's key, a webhook's current and previous signing secret, a backup target's
credential, a rule's HTTP header secret at any depth, the header secret of an HTTP call still
waiting to be made or retried (in its job, until the job ends), and a CMDB connector's credential
or alert intake secret (`cmdb_connector`, purpose `cmdb.connector:<id>`) — each re-sealed by its owning service
under its purpose. A call's secret is moved under the purpose it was sealed for, its request's or the
rule's it was carried over from; one the envelope refuses under that purpose is skipped and counted,
and it drains with its job. The installation's own round - a job without a tenant, run in the scope whose writes the
installation policies admit - re-seals its sign-in providers' client secrets and its offered AI
models' keys, each under the purpose it was sealed for. No version or `updated_at` moves. A value
naming a key outside the ring is skipped and counted. Counts land in `hubtask_secret_reseals_total`
by store and outcome, as `encryption.resealed` in the workspace's trail when anything moved, and as
`instance.secrets_resealed` in the installation's journal for its own round.

---

## 9. HTTP hardening

| Measure | Requirement |
|---|---|
| Security headers | `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cross-Origin-Resource-Policy: same-site`, a minimal `Permissions-Policy`; a `Content-Security-Policy` per origin (below) |
| CORS | Empty by default; an explicit allowlist; never `*` with credentials |
| Rate limits | Per IP (unauthenticated), per token, per tenant; stricter for login, password reset, invitation, search and bulk; `429` with `Retry-After` and `RateLimit-*` |
| Request sizes | A global body limit (1 MiB by default), a separate upload limit, a header limit |
| Deadlines | A server-side request timeout; every handler receives a `context` with a deadline |
| Methods | State changes never over `GET`; `OPTIONS` only for CORS |
| Error responses | RFC 9457 with a stable `code` and a `request_id`; **no** stack traces, query fragments, versions or paths |
| Version disclosure | The `Server` header without a version; the version only through the authenticated `/meta` endpoint |

**Three origins, three policies.** The headers above are the same everywhere; the
`Content-Security-Policy` is not.

| Origin | Policy | Why |
|---|---|---|
| The API (`/api/*`, `/mcp`) | `default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'` | It answers JSON only; a body reaching a browser as HTML could load or send nothing |
| The web interface (everything else, when enabled) | `default-src 'none'`, then `'self'` for script, style, font, connect, manifest and worker; `img-src 'self' data: blob:`; no `'unsafe-inline'` or `'unsafe-eval'` — a **constraint on the frontend framework**. Under `HUBTASK_STORAGE_KIND=s3` the media origin is added to `connect-src` and `img-src` only ([ADR-0047](../adr/ADR-0047-media-origin-in-the-interface-policy.md)) | Bundle and API share one origin ([ADR-0028](../adr/ADR-0028-embedded-web-ui.md)); the upload flow needs the media origin |
| The media origin | `sandbox` | T-11 |

`presentation/webui` sets its policy on every answer, 404 and 405 included. **The upload's middle
step carries no credential**: the presigned URL is its own, and a bearer sent to a bucket is leaked.

---

## 10. Automation and AI as attack surface

Rules trigger HTTP calls and agents write, so:

1. **No scripting language** — only declarative rules with CEL conditions
   ([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)); no primitive executes arbitrary code.
2. **`run_as` is the permission boundary** (T-08), not the triggering user.
3. **Outbound calls only through `GuardedClient`** (T-07), plus a network egress allowlist in
   provider operation. The object store, an operator-configured endpoint in the database DSN's trust
   class, may use its own client, listed in the architecture test's exceptions, with deadlines,
   refused redirects, the breaker and the bulkhead.
4. **A causality bound** against loops and amplification (T-09).
5. **AI is opt-in per tenant**, off by default; content goes only to the configured provider
   (documented in the data catalogue); AI results are suggestions with provenance.
6. **MCP tools** carry `readOnly`/`destructive` hints; destructive tools need explicit release per
   token and are audited.
7. **A model writes no expression and names no address** (T-10). A rule drafted from a sentence
   holds its condition as sentences the server compiles, every value an escaped literal; it drafts no
   branch condition, dedupe key, outbound request or webhook, and only entry-level steps from an
   allowlist ([ai-first.md](./ai-first.md) §2).

---

## 11. Supply chain and build

| Control | Implementation |
|---|---|
| New dependencies | A new direct third-party dependency arrives only through an accepted ADR, before the first commit using it, pinned, licence-gated, and imported by exactly one adapter package a gate names (e.g. `cel-go`, the OIDC libraries in `infrastructure/oidc`, the NATS client in `infrastructure/eventbus`, `golang.org/x/crypto` as §8 says). A small, closed protocol is written here instead (§8) |
| Dependency updates | Dependabot ([ADR-0022](../adr/ADR-0022-github-platform.md)): grouped version updates weekly, ungrouped security updates on the advisory; `go.sum` mandatory; no `replace` onto forks without an ADR |
| Vulnerabilities | `govulncheck` in every pipeline run, a container scan (Trivy/Grype) in the release gate |
| Static analysis | `gosec` within `golangci-lint`; `depguard` enforces layer boundaries and forbids risky packages |
| Secret scanning | A push rule plus a history scan (gitleaks) |
| Build | Reproducible, `CGO_ENABLED=0`, ldflags with version and commit; no network outside the module proxy |
| Artefacts | A CycloneDX SBOM per release, a keyless cosign signature, a provenance attestation; the target is SLSA build level 3 |
| Base image | distroless/static, non-root (UID 65532), read-only root filesystem, no shell, multi-arch |
| Release integrity | Signed tags; publication only from a protected branch with review |

---

## 12. Data protection (reference)

The rules live in [data-protection.md](./data-protection.md), [data-retention.md](./data-retention.md)
and the [data catalogue](../privacy/data-catalog.md). The security core: logs without user content,
metrics without personal labels, traces with masked attributes (rule 10); no new personal data field
is merged without its catalogue row and deletion path.

---

## 13. Security gates in CI

The build fails if any row fails. No merge with a red gate, no exception by comment.

| Gate | Check |
|---|---|
| SG-1 | `govulncheck` with no known exploitable vulnerability |
| SG-2 | `gosec`/`golangci-lint` with no new findings (the baseline is frozen) |
| SG-3 | The cross-tenant negative test suite green and **complete** (every repository method covered) |
| SG-4 | The application role has no `BYPASSRLS` and owns no table; RLS is **active** on every tenant table (a catalogue query). `FORCE` has one exception, `item_capability_profile`, whose defaults the owner seeds ([ADR-0052](../adr/ADR-0052-managed-postgresql-support.md)). Exceptions live in one list per suite, each with its reason |
| SG-5 | The authorisation architecture test: no repository call without a prior policy check; no authorisation in adapters |
| SG-6 | The SSRF test suite against `GuardedClient` (metadata IPs, rebinding, redirect chains) |
| SG-7 | Secret scanning with no findings; the "log output contains no tokens or secrets" test |
| SG-8 | Fuzz tests for the parsers that take untrusted input — the query DSL, CEL input, signature verification, the intake parser — each target in its own nightly job (`nightly.yml:fuzz`, the targets found by `make fuzz-targets`); a crash fails the nightly, keeps the crashing input as an artefact and files a `finding`. Each has a target: the query DSL compiler (`FuzzCompile`), a rule's CEL condition (`FuzzCompileCondition`), the outbound webhook signature (`FuzzVerifySignature`), and the intake's two doors - the mail parser (`FuzzParseMail`) and the inbound credential a webhook delivery presents (`FuzzParseInboundToken`) |
| SG-9 | A container scan with no critical findings; the image verified as non-root and read-only |
| SG-10 | An SBOM produced and the image signed (release pipeline only) |
| SG-11 | Auth negative tests: an expired/tampered/revoked token, the wrong issuer, a missing scope; the purchase platform's narrow scope admitting exactly its verbs, and the deletion's step-up waived for nobody but a registered service account (T-26) |
| SG-12 | The upload matrix test (SVG, HTML, polyglot files, the wrong content type) |
| SG-13 | Every writing use case declares its audit obligation, and every auditable action is in the `AuditableAction` registry ([audit.md](./audit.md) §7). The named exceptions, audited only when refused, are listed in the gate (`test/architecture/parity_test.go`) with their reason: *GiveKudos* and *TakeBackKudos*, because a trail of every give would let an auditor count who acknowledged whom (UC-WRK-29 check 6) |

---

## 14. Handling incidents

1. **Reporting and aims** are in [`SECURITY.md`](../../SECURITY.md): a private GitHub security advisory or `security@hubtask.eu`; acknowledgement, a CVSS assessment and a fix are best-effort aims; disclosure is coordinated.
2. **Remediation** goes into every line inside its support period ([versioning-release.md](./versioning-release.md) §5).
3. **Communication** through a GitHub security advisory with the affected versions, a workaround and detection guidance (log patterns).
4. **Follow-up**: a blameless post-mortem note in the repository; the fix includes a regression test, without which the incident is not closed.

### 14.1 Coordinated vulnerability disclosure

The process `SECURITY.md` publishes is aligned with ISO/IEC 29147 (what the reporter and the public
see) and ISO/IEC 30111 (what happens inside). The internal half:

| Step | What is done | Record |
|---|---|---|
| Intake | The report lives in a private GitHub security advisory; a mailed report is moved there | The advisory draft |
| Triage | Reproduced; CVSS scored; the affected lines named; **actively exploited or not** decided on evidence (the reporter's, a public exploit, a known-exploited list) | The advisory draft |
| Root cause and variants | The flaw's class is searched for in the rest of the code, not only at the reported place | The fix's pull request |
| Fix | On every line in its support period, with a regression test; a private fork of the advisory where the fix must not be public before release | The release notes |
| Disclosure | The advisory published with the release that fixes it, or on the date agreed with the reporter | The advisory, a CVE (GitHub is a CNA) |
| Dependency findings | A vulnerability in a dependency Hubtask does not reach (`govulncheck` reports reachability, SG-1) gets a VEX statement *not affected* (§14.2); one it reaches is a vulnerability of Hubtask's. A flaw found in a dependency is reported to its maintainers | The VEX document, the upstream report |

### 14.2 The advisory format

| What | Where | Format |
|---|---|---|
| The advisory itself | The repository's GitHub security advisories, with its CVE; mirrored by GitHub into its advisory database in OSV form | GitHub advisory, CVE record |
| The machine-readable advisory | `https://hubtask.eu/.well-known/csaf/`, beside the provider metadata `/.well-known/csaf/provider-metadata.json` | CSAF 2.0, profile *security advisory* |
| *Not affected* statements | The same directory | CSAF 2.0, profile *VEX* |
| The components a release contains | Attached to the GitHub release (`sbom.cdx.json`) | CycloneDX, produced by `release.yml` (SG-10) |
| The contacts | `https://hubtask.eu/.well-known/security.txt`, built by the website (`apps/website/src/routes/.well-known/security.txt/`) | RFC 9116 |

The CSAF directory and its provider metadata are **published with the first advisory, not before**:
an empty feed would claim a publishing process that has never run (the website's rule, *only what
is true today*). The documents are written, not typed: an advisory is one file
`security/advisories/HUBTASK-YYYY-NNNN.yaml` - `id`, `title`, `kind` (`advisory` for a flaw of
Hubtask's, `vex` for a dependency's flaw it does not reach), `cve`, `initial_release`, `summary`,
`cvss_v3` (`vector`, `score`, `severity`), the `affected`, `fixed` and `not_affected` versions, a VEX
statement's `component` and `justification` (one of CSAF's five), `remediation`, `references` -
committed with the fix. `make csaf` (`tools/csaf`) turns them into the directory: one CSAF 2.0
document per advisory at `<year>/<id>.json` with its `.sha256` and `.sha512`, `index.txt`,
`changes.csv` newest first, and `provider-metadata.json` (role `csaf_publisher`). `release.yml`
writes it, signs each document keyless (`.sigstore.json` beside it) and attaches `csaf.tar.gz`;
`website.yml` writes it into the site's static files, so `/.well-known/csaf/` carries exactly what
the repository's advisories say. With no file there, both write nothing. A malformed advisory
fails the release rather than publishing half of it. The GitHub advisory and its CVE stay the
advisory of record; the CSAF document is its machine-readable form.

---

### 14.3 Reporting under the Cyber Resilience Act

**Not legal advice.** This section is the project's reading of Regulation (EU) 2024/2847 as the
research of 2026-10 records it
([foundation-law-standards-2026-10.md](../backlog/suite/research/foundation-law-standards-2026-10.md)
§1.4); the role question in particular deserves a lawyer's view before anybody sells Hubtask.

**The project's role.** Hubtask has no paid edition and sells nothing
([ADR-0080](../adr/ADR-0080-hubtask-is-apache-2-0.md)), so the project is not a manufacturer of it.
It acts as an **open-source software steward** would (Art. 24): a documented cybersecurity policy
(this document and `SECURITY.md`), coordinated disclosure (§14.1), cooperation with market
surveillance authorities on request, and Art. 14 reporting to the extent it is involved in the
development. Art. 3(14) defines a steward as a legal person and the project is held by a natural
person, so this is followed voluntarily — the process is the same either way, and it is what
providers and NIS2 customers ask of a supplier.

**What is reported, since 2026-09-11** (Art. 14): an **actively exploited vulnerability** in
Hubtask, and a **severe incident** having an impact on its security (for the project: a compromise
of the repository, the release pipeline, the signing identity or the image registry). Reports go
through ENISA's single reporting platform to the CSIRT designated as coordinator of the Member State
of the main establishment — for an owner in Germany, the research reads that as the BSI
(unverified).

| Clock (from becoming aware) | Actively exploited vulnerability | Severe incident |
|---|---|---|
| 24 hours | Early warning: that it is exploited, and the Member States where Hubtask is known to be affected if known | Early warning: whether it is suspected to be caused by an unlawful or malicious act |
| 72 hours | Notification: the product, the general nature of the exploit and of the vulnerability, the corrective or mitigating measures taken and those users can take, how sensitive the information is | Notification: the nature of the incident, an initial assessment, the measures |
| 14 days after a fix or mitigation is available | Final report: description and severity, the actor if known, the security update or other measures | — |
| 1 month after the notification | — | Final report: description and severity, the likely root cause, the measures |

**The runbook.** The clock starts at *awareness*, not at understanding, so the first hour decides
whether it has to be reported, not how to fix it:

1. **Decide the case** — in the advisory draft (§14.1): is there evidence of exploitation (a report
   saying so, a public exploit used against an installation, an entry on a known-exploited list)?
   Is it a severe incident of the project's own infrastructure? If neither, it is ordinary
   coordinated disclosure and nothing below applies. Write down the moment of awareness.
2. **24 h — early warning** on the reporting platform, with what is known; uncertainty is said, not
   estimated away.
3. **Inform the users** (Art. 14(8)) as soon as it is safe: a GitHub security advisory, published or
   — where publishing would help the attacker more than the users — a short notice naming the
   affected lines and the mitigation; providers learn it from the same advisory.
4. **72 h — notification** with the mitigation the users can apply now.
5. **Fix** on every line in its support period ([versioning-release.md](./versioning-release.md)
   §5), release, publish the advisory, its CVE and its CSAF document (§14.2).
6. **Final report** within 14 days of the fix (1 month after the notification for an incident).
7. **Close** with the post-mortem note and the regression test (§14 item 4).

A vulnerability the project finds in a **dependency** is reported to that dependency's maintainers
(Art. 13(6)); if it is actively exploited, the steps above apply to Hubtask's exposure.

### 14.4 What the project supplies to a provider who is a manufacturer

A provider who sells Hubtask, a hosted service built on it, or its own distribution is a
**manufacturer** under the CRA for what it places on the market (from 2027-12-11 in full, with
Art. 14 reporting already since 2026-09-11), and receives from the project what it needs to be one:

| The manufacturer needs | The project supplies | Where |
|---|---|---|
| The components (Annex I Part II (1)) | A CycloneDX SBOM per release image, a provenance attestation, a cosign signature | The GitHub release; `release.yml` (SG-10) |
| Known vulnerabilities and their state | Advisories with CVE, CSAF 2.0, VEX *not affected* statements | §14.2 |
| A support period to build on (Art. 13(8)) | The published support period of every line; every release kept available | [versioning-release.md](./versioning-release.md) §5, `SECURITY.md` |
| A disclosure channel upstream | Coordinated disclosure, and the report of an exploited vulnerability shared with providers through the advisory | §14.1, `SECURITY.md` |
| Secure-by-default evidence (Annex I Part I) | The principles and the gates | §1, §13 |
| Controls to cite in its own attestation | The control mapping to ISO/IEC 27001, BSI C5 and OWASP ASVS | [control-mapping.md](../privacy/control-mapping.md) |
| Answers for its NIS2 customers' supply-chain questions | The NIS2 supplier pack: SBOM, signatures, release records, disclosure, advisories, patch aims, support periods, the product's measures | [nis2-supplier-pack.md](../privacy/nis2-supplier-pack.md) |

The manufacturer declares its own support period (at least five years unless the product's expected
use is shorter), runs its own conformity assessment, CE marking and Art. 14 reporting, and carries
fixes into a line after the project's support for it has ended — the Apache-2.0 licence allows that
without asking.

## 15. Deliberately not included

| Not included | Reason |
|---|---|
| End-to-end encryption of content | Incompatible with server-side search, automation and AI |
| Our own WAF/IDS | The operator's job (ingress/reverse proxy); the application supplies clean signals |
| Certifications (ISO 27001, SOC 2) | Organisational, not architectural |
| SAML/SCIM | After `1.0.0` |

---

## 16. Open points

| # | Point | Needed by |
|---|---|---|
| S-1 | An external penetration test / code audit before the first commercial operation | Before `1.0.0` |
| S-4 | A threat row for the web client itself; today the interface's policy (§9) and the client's credential rules ([identity.md](./identity.md) §14.4) cover it | — |
| S-5 | Bug bounty yes/no, and its framing | After `1.0.0` |
