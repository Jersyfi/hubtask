# Milestone 0.6.0 — Multi-tenancy and operations

The goal: the system becomes something an operator can run for other people. A person signs in with
a password and sees their sessions; a tenant can demand MFA of its administrators; a destructive
act asks the actor to prove themselves again, and the restore modes that have refused since 0.4.5
finally run; a company signs in through its own identity provider, and a third-party app asks for
bounded access through OAuth2 instead of borrowing somebody's PAT; a provider provisions, suspends,
exports and deletes tenants with evidence, and no tenant can starve another — not through the API,
not through the job queue, not through a query; the big append-only tables partition before they
are big; overload sheds the deferrable work instead of the interactive latency; the installation's
own database has point-in-time recovery with a drill that proves it; and every alert in the
catalogue exists with a runbook. Phase 1 made the core complete, 0.4.5 made it operable, 0.5.0 made
it programmable; this milestone is what makes it *hostable*.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: SAML and SCIM (after `1.0.0`, security.md §15);
shard routing and data residency per region — the growth path multi-tenancy.md §2 describes stays a
growth path, prepared for by the `tenant_id` model and taken when a tenant demands it; billing,
plans and metering beyond `usage_record` — the quota values marked "plan-bound" are the operator's
numbers in `tenant.settings`, and the billing model is the roadmap's own "not ready"; the capacity
model from real load data (O-2, `0.9.0`) and any **published** performance figure — the numbers
this milestone produces are internal, by the owner's decision of 2026-08-21 (see decision 7);
GitOps (D-3, `0.9.0`); external audit chain anchoring (A-2, `0.9.0`); the public status page (O-3,
after `1.0.0`); cron notation for `SCHEDULE` triggers (recorded as sugar in 0.5.0, still sugar);
and every screen that renders any of it — sign-in, sessions, MFA, the administration area and the
tenant surface are **F4**'s, one window behind, exactly as `roadmap.md` says it should.

Eight decisions taken while writing this backlog, so that nobody re-derives them:

* **This time both halves are empty.** 0.4.5 found the schema full and the code empty; 0.5.0 found
  the schema full and the contract empty. 0.6.0 finds *neither*: no table holds a session, a
  refresh family, a TOTP secret, a recovery code, an OAuth client or grant, or a tenant's IdP
  configuration, and `openapi.yaml`'s "not yet elaborated" list still names `admin/tenants`.
  What phase 0 did prepare is exactly three seams: `account.password_hash` (Argon2id, local
  accounts), `account.external_subject` with its unique index (OIDC just-in-time provisioning),
  and the `tenant_status` lifecycle enum with `usage_record`. Every task here carries a real
  specification-first step **and** a real migration — expand/contract, forward only, as always.
* **The token shapes are already law.** security.md §5's table is implemented, not designed: the
  access token lives 15 minutes and verifies without a database round trip — signed under a
  `HUBTASK_SECRET_KEY` purpose label, the discipline every cursor and feed token already follows —
  the refresh token lives 30 days and rotates, reuse invalidates the family and raises A-15's
  second half, and a session is a row somebody can see and revoke. Even the retention data kind
  was anticipated: `retention_policy`'s comment has listed `'session'` since `0001_init`.
* **Sign-in needs a tenant before it can check a password.** Accounts are per-tenant
  (`account_email_uq` is `(tenant_id, lower(email))`), so in multi mode the tenant is resolved
  from the subdomain or header *before* the credential check, per the priority order in
  multi-tenancy.md §3 — and a contradiction between token and host stays `403 tenant_mismatch`.
  Single mode is the special case with one row, not a second code path.
* **Dependency decisions are ADRs, and there are three candidates, not one.** The OIDC token
  verification library (H-04), the NATS client (H-14) and the IMAP client (H-15) are each a supply
  chain decision this backlog does not take in passing: each task opens with a draft ADR the owner
  accepts before the implementation commits land. TOTP is deliberately **not** on that list — RFC
  6238 is `crypto/hmac`, `crypto/sha1` and `crypto/subtle`, and lands dependency-free. The OAuth2
  *provider* side likewise: it issues its own opaque tokens under the shapes above and needs no
  library.
* **The OAuth2 provider lands headless.** The protocol endpoints, the client registry, the grant
  model and the consent record land now, driven end to end by the scripted session; the consent
  *screen* belongs to the client track, which is exactly the shape 0.5.0 chose for every surface.
  Zapier's marketplace needs the flow at `0.9.0`; landing it two milestones early, proven without
  a browser, is what keeps that window slack rather than tight.
* **The admin API is the one legitimate tenant enumerator.** "Nothing enumerates tenants" is the
  rule for *jobs* (multi-tenancy.md §2.1, and the reason every per-tenant duty is self-seeded);
  `/admin/tenants` reads through a deliberate installation-scoped path behind the `admin:tenants`
  scope, because provisioning and lifecycle are the control plane's job and the control plane must
  see its rows. The hard-delete grace job stays self-seeded by the deletion request's own write —
  the rule for jobs survives the milestone that could most easily have broken it.
* **Load figures stay internal, and the shape of the measurement was decided before the harness.**
  The owner decided on 2026-08-21: measure and record internally, publish nothing yet, buy no
  benchmark machine yet. Four consequences bind H-11: a concurrent-user count is a derived figure
  and never a headline (throughput ÷ a behaviour model, published beside the model or not at all);
  the number a provider can price is requests/second per vCPU at a held P95 and its decay with
  items per tenant; percent-level regressions need a quiet machine, so the harness has two tiers —
  a relative regression guard against a stored baseline with an explicit noise band, and a full
  capacity ramp on named hardware per release; and RT-6 cannot pass until load shedding exists,
  because observability-reliability.md §7 describes a threshold the code has never had. CI-1 is
  answered inside H-11, with the integration server as the named hardware unless the owner decides
  otherwise.
* **The Kubernetes half was built ahead, and is proved rather than added.** The chart already
  carries the four-role separation, HPA, PodDisruptionBudget and NetworkPolicy — the roadmap line
  predates the chart having them. Likewise `audit_log` and `change_log` already partition by month
  with `ensure_audit_partition` as the repair precedent. What 0.6.0 adds is the remaining three
  large tables (H-09), the production environment those templates have never met (H-10), and the
  load that proves the shedding and the fairness (H-11).

---

## H-01 — Sign-in: sessions, refresh rotation, and the invitation redeemed **[L]**

*Depends on: nothing. Everything that authenticates a person hangs off it.*

The password sign-in the system has never had: `openapi.yaml` carries one bearer scheme and no way
to obtain a bearer, which is why F1 signs in with a PAT and why the invitation flow stops at
`INVITED`. The routes are the specification-first step under `/auth/sessions`: sign in (password →
an access/refresh pair plus a session row), refresh, list one's sessions, revoke one, revoke all —
operation ids are the use case names, because the parity gate derives one from the other (G-05's
lesson). The migration brings the session and refresh-family tables; the `'session'` retention
data kind the schema comment promised ages out expired rows through the retention engine, not a
second sweeper.

The shapes are decision 2's: 15-minute signed access tokens verified without a database read,
30-day rotating refresh tokens where reuse kills the family, raises `hubtask_auth_failures_total`
with its own reason, and feeds A-15. T-02 is implemented, not cited: Argon2id verification in
constant shape, progressive delay, lockout after n failures per account **and** per IP, one
generic refusal that discloses no account's existence — and the auth rate-limit bucket
(`HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE`) already exists to stand in front of it. A session records
the client-binding hint (user agent, IP class) T-01 asks to log — which makes both **personal
data**: catalogue rows with purpose and deletion path, and the fields travel in no log or trace.
The invitation closes its loop: a redemption token (minted on invite, hashed under a purpose
label, shown once — D-08's discipline) lets an `INVITED` account set its password and become
`ACTIVE`; the data-catalogue note that has pointed at this milestone since E-10 is resolved.
Sign-in is a bearer flow end to end; the optional session cookie of T-12 stays unbuilt until a
first-party client needs it, and the refusal to set cookies is recorded in the task, not silent.

**Acceptance:** a person signs in with email and password and calls the API with the returned
access token; the refresh rotates and an old refresh token replayed after rotation invalidates the
whole family, refuses the call, and the metric and alert fire, proved with a clock; sessions list
with created/last-used/client hint, revoking one refuses its pair immediately, revoking all ends
every device; the lockout engages per account and per IP with progressive delay and the same
generic error for "wrong password" and "no such account", byte for byte; an invited account
redeems its token once, sets a password under the policy (length 12, Argon2id parameters from
security.md §5), and a second redemption is refused; expired sessions age out through the
retention engine under the new data kind; sign-in resolves the tenant per decision 3 and a
mismatch answers `403 tenant_mismatch`; no password, hash, token or session secret appears in any
log, metric, trace or audit entry, proved by the redaction tests; the use cases are registered
with metric, span and audit declarations, the parity gate is green, and the cross-tenant negative
test proves sessions of one tenant are invisible in another.

**Read:** security.md §5, §4 (T-01, T-02, T-12); ADR-0005; multi-tenancy.md §3;
`docs/privacy/data-catalog.md` (the invitation note); api-guidelines.md §7

---

## H-02 — TOTP: a second factor a tenant can demand **[L]**

*Depends on: H-01. The factor is proved at sign-in, so there must be one.*

TOTP per RFC 6238, dependency-free (decision 4): enrolment mints a secret sealed through E-02's
`Encryptor` under its own purpose, answers the provisioning URI once (the QR is a client's job),
and arms only after the caller confirms with a valid code — an unconfirmed enrolment protects
nobody and locks nobody out. Verification allows one step of drift either side, refuses replay of
the same step, and rate-limits attempts per account. Ten single-use recovery codes are minted at
enrolment, stored hashed, shown once; using one burns it and the response says how many remain.
Disabling MFA requires a fresh password verification — the one case where "recently signed in" is
not enough, because a stolen session removing the second factor is the attack.

Enforcement is the tenant's, per security.md §5: a `tenant.settings` switch demands TOTP of
`OWNER` and `ADMIN` role holders. Sign-in becomes two-step for enrolled accounts — the password
answers a short-lived pending credential that can do nothing but present a code — and an
administrator under enforcement who is not yet enrolled is routed into enrolment rather than into
a session. The pending state is a row with the session machinery's discipline, not a special
token class invented here.

**Acceptance:** enrolment round-trips — secret sealed, URI shown once, armed only after a valid
code; a code verifies within the drift window, refuses outside it, and the same code never
verifies twice; recovery codes work exactly once each and their consumption is audited; sign-in
for an enrolled account refuses a full session until the code, and the pending credential can
reach no other route, proved by trying; the tenant switch makes an unenrolled `ADMIN`'s sign-in
end in enrolment, while a `MEMBER` is untouched; disabling requires the fresh password and is
audited; the sealed secret and every code appear in no log, metric, trace, audit entry or API
response after their single showing, proved by the redaction tests; use cases registered, parity
green, cross-tenant negative test holds; new fields carry catalogue rows and merge rules
(offline-sync.md §4.2 — MFA state does not travel to devices, and that is written down, not
implied).

**Read:** security.md §5, §8; E-02 in `milestone-0.4.5.md` (the sealing discipline); RFC 6238;
H-01's session shapes

---

## H-03 — Step-up: the proof before the irreversible **[L]**

*Depends on: H-01, H-02. The verifier asks for what they built.*

`core/port/stepup` has had a seam and no verifier since E-06 — the shipped implementation answers
"this installation cannot ask anybody", and every destructive restore mode has been refused with
`backup.restore_step_up_unavailable` since. This task builds the verifier: a step-up is a fresh
re-authentication on the current session — the password, or the TOTP code where one is enrolled —
recorded on the session with a timestamp, valid for a short window (minutes, configured), and
consumed by the privileged action that demanded it. The REST layer surfaces the demand as a
distinct refusal (`403` with `auth.step_up_required` and the accepted methods), so a client knows
to ask the person rather than to retry.

The privileged action list is security.md §5's, made real: deleting a tenant, changing the `OWNER`
role, minting a token with an admin scope — and the destructive restore modes, whose two refusal
codes E-06 built precisely so that this task could flip one and keep the other (`step-up not
given` remains; `step-up unavailable` dies). The declaration lives with the use case descriptor,
the way audit and scopes already do, so the registry knows which operations demand it and the
architecture test can prove no privileged operation forgot — a list in a document is how the next
privileged action ships without one.

**Acceptance:** a destructive restore with a fresh step-up runs — the refusal that has stood since
0.4.5 is flipped, not deleted, and the restore drill's destructive round trip finally completes
end to end; without the step-up the same call answers `auth.step_up_required` naming the methods,
and `restore_step_up_unavailable` is gone from the codebase except its test's tombstone; the
window expires by the clock and a consumed step-up does not cover a second privileged action;
changing `OWNER` and minting an admin-scoped token demand it through the descriptor declaration,
proved by the registry test that fails for an undeclared privileged operation; a step-up event is
audited with its method, never its credential; use cases registered where new, parity green.

**Read:** backup-restore.md §8.3 (the three pinned steps); security.md §5 (privileged actions);
E-06's refusal decision in `milestone-0.4.5.md`; H-01/H-02

---

## H-04 — OIDC: the company's identity provider signs people in **[L]**

*Depends on: H-01 — the flow ends in the same session machinery. The dependency ADR opens the task.*

The relying-party half of ADR-0005, per tenant: a tenant configures its identity provider —
issuer, client id, a client secret sealed through E-02, optionally the allowed email domains —
and its people sign in through authorization code + PKCE, landing in exactly the session H-01
built (one session model; how you proved yourself is an attribute, not a class). Discovery and
JWKS are fetched through `GuardedClient` and cached with rotation; verification is T-13 verbatim
— signature, `iss`, `aud`, `exp`, `nonce`, no `alg: none`, clock skew ≤ 60 s — and the test suite
includes the tampered-JWT cases the threat row names.

Provisioning is just-in-time onto the seam phase 0 cut: the subject lands in
`account.external_subject` under its unique index. Linking an existing local account happens on
verified email match within the tenant's allowed domains, and is recorded; an OIDC account holds
no password and the password path refuses it the way it refuses service accounts. Degradation is
the row observability-reliability.md §7 has always promised: the provider down means existing
sessions continue and local accounts sign in, with a clear code — proved by stopping the fake
IdP, not asserted. The task opens with the draft ADR for the verification library (decision 4):
what is verified, what the candidates are, why hand-rolling JOSE is excluded by security.md §8's
ban on home-grown crypto — the owner accepts it before implementation commits land.

**Acceptance:** the ADR is accepted and the dependency pinned before any implementation commit; a
full code+PKCE round trip against a test IdP ends in a session indistinguishable from H-01's; each
T-13 tampering (wrong issuer, wrong audience, expired, bad signature, stripped nonce, `alg: none`)
is refused with its test; JIT provisioning creates the account once and finds it thereafter, and
the email-match link is recorded and only happens inside the allowed domains; the sealed client
secret appears nowhere after configuration; a suspended or disabled account is refused at token
exchange, not just at first sign-in; the IdP being unreachable degrades exactly as the table says,
proved against a stopped container; configuration reads accept `READ_CONFIGURATION` (A-4's
precedent), writes demand the admin permission and are audited; parity green, cross-tenant
negative test holds — one tenant's IdP configuration is invisible and unusable in another.

**Read:** ADR-0005; security.md §4 (T-13), §8; observability-reliability.md §7;
multi-tenancy.md §3; the ADR this task writes

---

## H-05 — OAuth2: this installation becomes a provider **[L]**

*Depends on: H-01 — grants are consented by a signed-in person and issue the same token shapes.*

The other direction from H-04: third-party apps get bounded access without borrowing a PAT.
Authorization code + PKCE only — no implicit, no password grant, no client credentials in this
milestone — with a client registry an administrator manages: name, exact-match redirect URIs,
public clients with PKCE mandatory, confidential clients with a hashed secret under the token
discipline. The person consents to named scopes from the catalogue — the same scopes descriptors
already declare, no parallel vocabulary — and the grant is a row they can see and revoke beside
their sessions. Issued tokens are H-01's shapes with the grant as their leash: revoking the grant
kills its family the way refresh reuse does.

Headless, per decision 5: the authorize endpoint validates client, redirect and PKCE, the consent
is recorded through the API, and the scripted session drives the whole dance — the consent screen
arrives with the client track. Every act is audited with the client as a first-class actor
attribute, because "which app did this" is the question this feature exists to answer.

**Acceptance:** the full code+PKCE flow, scripted: register a client, authorize, consent, exchange
the code, call the API with the issued token, refresh, revoke the grant, watch the next call
refuse; a wrong or extra redirect URI is refused exact-match; a public client without PKCE and a
code replayed after exchange are refused; a token's scopes bound it exactly as a PAT's do, proved
by the existing scope tests extended to this issuance path; grants list per account with client,
scopes and last use, and revocation is immediate and audited; client secrets and codes appear
nowhere after their single showing; use cases registered, parity green, cross-tenant negative
test holds.

**Read:** api-guidelines.md §7; security.md §5, §8; H-01's shapes; RFC 7636 (PKCE);
`domain-model.md` §5 (Integration)

---

## H-06 — Multi mode and the lifecycle of a tenant **[L]**

*Depends on: H-01 (sign-in must resolve tenants), H-03 (deletion is a privileged action).*

`HUBTASK_TENANCY_MODE=multi` becomes true: `/admin/tenants` leaves the "not yet elaborated" list
with the lifecycle multi-tenancy.md §5 has always drawn. Provisioning creates the tenant with its
defaults — default hub, example collection, owner membership, locale and time zone, standard
buckets and labels — idempotent under `Idempotency-Key`, exactly the §5 table. Suspension flips
the middleware: the API answers `403 tenant_suspended`, data remains, the read export still works;
reactivation is one write. A deletion request moves the tenant to `PENDING_DELETION` — access
blocked, an export provided, automations disabled — and the hard delete runs after the 30-day
grace as a job **seeded by the request's own write** (decision 6): it cascades across every
storage location the §5 table names — database rows, media objects, search, outbox, queue — and
leaves evidence where a per-tenant trail cannot live, which this task decides and documents
(the instance angle audit.md has never had to answer before).

Tenant resolution lands as §3's priority order — token claim, subdomain, header, never the path —
with `tenant_mismatch` on contradiction. The admin surface reads through the installation scope
behind `admin:tenants` (decision 6), and deleting a tenant demands H-03's step-up plus the typed
tenant name the restore precedent set. Single mode keeps working untouched — one row, no
selection, the same code — and the compose e2e stack gains a multi-mode variant so the mode is
exercised rather than configured.

**Acceptance:** a provisioned tenant is immediately usable — its owner signs in, the default
structure exists — and provisioning twice under one key creates one; a suspended tenant's API
calls refuse with the code while its export still answers; a deletion request blocks access,
provides the export, disables the automations (visible on the rules), and the grace job hard
deletes after 30 days by the clock — rows, media bytes, outbox, queue and search entries are gone,
proved by counting in each store, and the evidence entry exists where the task decided; the
deletion demands step-up and the typed name; resolution follows the priority order with a test per
source and `tenant_mismatch` on contradiction; `single` mode behaves exactly as before, proved by
the untouched existing suites; use cases registered, parity green; the cross-tenant suite gains
the admin surface — a tenant admin of one tenant can do nothing here.

**Read:** multi-tenancy.md §1, §3, §5; ADR-0010; H-03; E-10's erasure machinery in
`milestone-0.4.5.md` (the deletion precedents); `deploy/docker/` (the e2e stack)

---

## H-07 — The tenant export: everything, documented, portable **[G]**

*Depends on: H-06 — the lifecycle provides it; E-01's `/jobs/{id}` reports it.*

`POST /admin/tenants/{id}:export` per multi-tenancy.md §5: a complete, documented JSON Lines
archive plus media — the basis for GDPR access requests, provider migration, and the "no lock-in"
promise. A job (`202` + `JobRef`, E-01's machinery), written through `backup.StoreOpener` to a
backup target the way the audit export is (E-09's precedent — one discipline for "bytes leave the
installation"), and the export runs through the same RLS path as the API, which is T-20's row and
gets T-20's test: an archive of tenant A contains not one byte of tenant B, proved by writing into
a two-tenant installation and grepping the archive.

The format is the deliverable as much as the code: a documented, versioned shape under `docs/`
(entity per line, the manifest naming counts and the product version, media by reference with
checksums) — documented enough that someone could build an importer against the document alone,
because that is what "no lock-in" means. It works for `ACTIVE`, `SUSPENDED` and
`PENDING_DELETION` tenants — the suspended and leaving are exactly who needs it — and the export
is audited with its target, never its content.

**Acceptance:** an export job completes and `/jobs/{id}` reports it; the archive validates against
its documented format, the manifest counts match the database, media checksums verify; the
two-tenant isolation grep holds (T-20); a suspended and a pending-deletion tenant export
successfully, an unknown tenant answers the indistinguishable admin `404`; the format document
exists and `gate-docs` covers it; concurrent export jobs respect the §4 quota; use case
registered, parity green, audit entry present.

**Read:** multi-tenancy.md §5, §6; security.md §4 (T-20); E-01 and E-09 in
`milestone-0.4.5.md`; H-06

---

## H-08 — Quotas and fairness: no tenant starves another **[G]**

*Depends on: nothing. Runs beside the auth track.*

The §4 table becomes enforcement: per-tenant limits from `tenant.settings` with the mode's
defaults — API requests per minute per token (the rate limiter learns the per-tenant bound it has
configured globally until now), items per tenant, media storage, automation runs per hour,
webhook targets, concurrent export jobs — refused with a distinct, documented code per limit
(rate is `429` with `Retry-After`; a capacity quota is a `422`-shaped refusal naming the quota and
the ceiling, because waiting does not help). Counting goes through `usage_record` where the
period matters and live counts where the row does; the approach ratio is
`hubtask_tenant_quota_usage_ratio`, which finally gives A-18 something to watch.

Fairness is the half a limit table cannot give: the job queue learns weighted claiming so one
tenant's storm cannot monopolise the workers — per-tenant round-robin at claim time, not a second
queue — and the query DSL gets the cost rejection §4 names: a query whose estimated cost is
obviously unaffordable is refused before it runs, with the existing depth and node caps as the
floor this builds on. Single mode defaults stay effectively unlimited; the enforcement exists
everywhere, the numbers differ — one code path, per §1.

**Acceptance:** each table row has an enforcement test — the limit engages at its bound with its
code and headers, and the single-mode default does not engage in normal use; a tenant at 90% of
any quota raises the ratio metric and A-18 fires in the rules test; two tenants flooding the queue
share the workers measurably — the fairness test asserts both make progress and neither claims
the whole pool; an unaffordable query is refused with its code and a affordable one is not,
with the boundary documented; quota reads accept `READ_CONFIGURATION`, writes are the operator's
(admin scope) and audited; nothing regresses for an installation that configures nothing, proved
by the untouched existing suites.

**Read:** multi-tenancy.md §4; observability-reliability.md §4, §10 (A-18);
api-guidelines.md §3, §5; the queue's claim path (B-10, D-03 precedents)

---

## H-09 — Partitioning: the big tables, before they are big **[G]**

*Depends on: nothing. Land it before H-11 fills two million rows.*

`activity_entry`, `outbox_event` and `rule_run` partition by month — the three multi-tenancy.md
§7 names, on the pattern the schema already runs twice: `audit_log` and `change_log` partition
with a default catch-all, RLS carried per partition, and `ensure_audit_partition` as the
SECURITY-DEFINER create-and-repair precedent (E-09). The leader creates this month and next for
each; the default partition catches what a gap would otherwise lose; and the retention engine
learns that an aged-out month of a partitioned kind is a dropped partition, not a million-row
`DELETE` — the sweep's shape changes for exactly these kinds and no other.

The hard part is that these three tables exist and carry data: the migration must be
expand/contract and safe under a rolling update (deployment.md §5 — old and new versions run
simultaneously). Whether that is attach-the-existing-table-as-first-partition, a swap under a
short lock, or a phased copy is the task's decision to make and record; what is not negotiable is
forward-only, no lost rows, no window in which a write fails, and `db/schema.sql` mirroring the
result exactly (the schema reference is diffed against a migrated database — drift is a red gate).

**Acceptance:** all three tables are partitioned with RLS proved per partition (the catalogue
query test extends to them); writes during a simulated rolling update (old binary against the new
schema) succeed throughout the migration, proved in the migration rehearsal test; the leader
creates coming partitions and repairs a hand-broken policy, per the E-09 precedent tests; a
retention run over an aged-out month drops the partition and the evidence trail says so; the
default partition catches an out-of-range row rather than erroring; `make generate` produces no
diff and the schema reference matches; existing suites — activity, outbox dispatch, rule runs —
pass untouched.

**Read:** multi-tenancy.md §7; migration 0043 and E-09's partition decisions in
`milestone-0.4.5.md`; ADR-0003; deployment.md §5; `db/schema.sql` (the two existing patterns)

---

## H-10 — PITR and the production decision **[L]**

*Depends on: nothing in the code; on the owner for D-1 and D-2. Open it early — decisions have
lead time.*

Three coupled decisions come due, and they are the task's first half, as one draft ADR presenting
the options with costs: **D-1**, where `production` runs (integration is decided and running;
production deliberately was not the same decision); **D-2**, the database — own container,
operator, or managed service — which determines who does PITR; and **B-2**, whether Hubtask
orchestrates system backups or leaves them to the operator, which backup-restore.md has held open
since 0.4.5. **B-3** rides along: whether object lock is recommended or required against
ransomware for backup targets. The owner decides; the ADR is written so the decision is one
paragraph of choosing, not one evening of research.

The second half implements the choice: WAL archiving with a tested recovery path in the chosen
shape, the RPO ≤ 5 min / RTO ≤ 60 min targets from observability-reliability.md §2 made
measurable, A-12's PITR half emitting (the alert has watched a backup-age metric since E-05; the
replication/PITR gap half has had nothing to watch), and **RT-9 as a drill that runs per
release**: restore the system backup to a fresh environment, run the consistency and isolation
checks (T-20's drill half), record the evidence under `docs/evidence/` — the discipline A-20
already expects. The self-hosting story stays honest: the Compose file's documented `pg_dump`
remains the minimal variant, and the docs say which guarantees it does not give.

**Acceptance:** the ADR is accepted with D-1, D-2, B-2 and B-3 answered and the documents updated
(deployment.md, backup-restore.md, data-protection where P-5 touches); a PITR restore to a
point between two writes recovers the first and not the second, proved in the drill; the drill is
a workflow that runs per release, writes its evidence file, and its absence past 90 days is what
A-20 already alerts on; A-12 fires in the rules test when the gap exceeds threshold; RPO/RTO are
measured by the drill and recorded internally (decision 7's discipline applies to these numbers
too); the `INSTANCE` restore scope stays refused unless the ADR decides otherwise, and the
refusal text now points at the decided operator procedure.

**Read:** deployment.md §3, §8 (D-1, D-2); backup-restore.md §10 (B-2, B-3);
observability-reliability.md §2, §10 (A-12, A-20); security.md §4 (T-20)

---

## H-11 — Load shedding, then the load told the truth about **[L]**

*Depends on: H-08 (the storm proves fairness), H-09 (two million rows live in partitions). Late
by design: it measures the milestone's shape, not the milestone's absence.*

First the missing mechanism: observability-reliability.md §7's load shedding —
above a threshold on `inflight_requests`, new **non-interactive** requests (bulk, export, search,
the heavy `:query` shapes) answer `503` with `Retry-After` before latency tips over for everyone.
The gauge exists; the threshold, the route classification and the refusal do not. A config
variable per role, a metric for shed requests, and RT-6 becomes runnable: a load test beyond
capacity in which shedding engages, interactive P95 holds, and nothing OOMs.

Then the harness, on the RT-8 seed (it already counts from the client, paces independently of
responses — the coordinated-omission discipline — and records a timeline): a ramp mode, dataset
seeding (two million items across a realistic tenant distribution, seeded by script against a
real stack), and the **automation storm** (a bulk write fanning into rules, webhooks and the
outbox at once). Two tiers per decision 7: the relative regression guard runs nightly against a
stored baseline with an explicit noise band and answers only "did this get significantly worse";
the full capacity ramp runs per release on named hardware — the integration server, closing CI-1,
unless the owner's answer to the ADR question in this task names different iron. Results are
recorded under `docs/evidence/` and stay internal; the figure recorded is requests/second per
vCPU at held P95 and its decay with dataset size — never a bare user count.

**Acceptance:** RT-6 runs in the nightly and passes: shedding engages at the threshold,
interactive P95 stays in target while bulk/export/search shed, memory holds; shed requests carry
`Retry-After` and the metric counts them by class; the seeding script builds the two-million-item
dataset reproducibly and the ramp completes against it; the storm exercises H-08's fairness — the
non-storming tenant's P95 is asserted, not eyeballed; the regression guard fails the build on a
seeded slowdown beyond the band (`gate-selftest`-style proof) and stays green on noise; baselines
live in the repository with the band written beside them; the first evidence file exists with the
per-vCPU figure and the behaviour model beside every derived user count; CI-1 is closed in
ci-cd.md with what was decided.

**Read:** observability-reliability.md §6, §7, §12 (RT-6); `test/resilience/rt8`;
the owner's 2026-08-21 decision (decision 7 above); ci-cd.md §8 (CI-1); H-08, H-09

---

## H-12 — The alert catalogue, complete, with the dashboards **[G]**

*Depends on: H-01 (A-15 needs an auth path), H-08 (A-18 needs the ratio); loosely on the rest —
last of the observability track.*

The full catalogue A-01…A-18 as shipped rules — today `alerts/prometheus-rules.yaml` carries the
reduced self-hosting set of eight, and the provider set has been "with provider operation" since
the file existed. This milestone is provider operation. The burn-rate pair A-01/A-02 lands
against SLO-1 with the multiwindow shape, and every alert ships with its runbook or does not ship
— `make gate-observability` already enforces the pairing in both directions and `promtool` checks
the expressions; this task feeds the gate rather than building it. The two dashboards the
directory has promised join the shipped ones: `tenant.json` (quotas, top tenants — behind the
tenant label opt-in, cardinality per §3.2) and the SLO view (budgets, burn, the eight SLO rows as
panels).

**O-1** is decided inside the task: the alerting backend for our own operation — the obvious
answer is Prometheus + Alertmanager on the integration cluster feeding the owner's inbox, and if
the decision is that, it is written into observability-reliability.md with the config under
`deploy/observability/`, wired and firing a test alert, not described.

**Acceptance:** every catalogue row A-01…A-18 exists as a rule with a runbook, `promtool` and the
gate are green, and the rules test exercises each alert's firing condition at least synthetically;
A-01/A-02 use multiwindow burn rates and the runbook explains the arithmetic; `tenant.json`
renders against a multi-tenant stack with the label enabled and degrades to a notice without it;
the SLO dashboard shows all eight; O-1 is closed in the document and a deliberately provoked test
alert reached the decided backend once, with the evidence noted in the PR; the self-hosting
reduced set is unchanged (nothing new is forced on a single-tenant operator).

**Read:** observability-reliability.md §2, §10, §11, §14 (O-1); `deploy/observability/`;
H-08's metric; G-02's lag alert precedent

---

## H-13 — The parked defaults become decisions **[G]**

*Depends on: nothing technically; on the owner for every one of them. G-12's shape: none large,
each one owed.*

Four decisions with 0.6.0 written beside them since their documents were young:

* **S-2** (security.md §16): master key management in provider operation — environment, KMS, or
  Vault. E-02 built the keyring so this is an adapter question, not a redesign: the draft ADR
  weighs the three against the threat that matters (a database dump plus a filesystem read), and
  if the answer adds a KMS client it is this task's dependency decision under decision 4's
  discipline. Whatever is chosen, a full rotation — new key first in the ring, old data re-sealed
  lazily, old key retired — is exercised against a real stack and documented as the operator
  procedure, because a rotation nobody has run is a hypothesis (A-20's logic, applied to keys).
* **A-1** (audit.md §9): the default audit retention period, legally agreed — evidentiary
  interest against storage limitation. The owner settles the number (400 days has been the
  placeholder); the decision lands in audit.md, the default in the retention seed, and the data
  catalogue row stops saying "configurable" as if that were an answer.
* **P-5** (data-protection.md §12): backup retention against the deletion obligation — the
  binding period after which a deleted tenant is gone from backups too, documented where H-06's
  hard delete can point at it. Coupled to H-10's B-2 answer and written in the same review.
* **P-6**: anonymisation or full deletion as the tenant default. E-10 built both modes; this
  decides which one `tenant.settings` starts with and documents the trade-off (authorship
  preserved against maximal erasure) so a tenant changes it knowingly.

**Acceptance:** each of the four ends with its document updated and its open-point row closed;
S-2's ADR is accepted, the chosen shape is implemented if it is more than "stay with the
environment", and the rotation drill ran once with its procedure documented; A-1's number is in
the seed and the catalogue; P-5's period is stated in days in both documents and H-06/H-10
reference it; P-6's default is in the settings schema with the trade-off documented; no refusal
existed to flip — these were defaults pretending to be decisions, and now they are decisions.

**Read:** security.md §16 (S-2); audit.md §9 (A-1); data-protection.md §12 (P-5, P-6);
E-02 and E-10 in `milestone-0.4.5.md`; H-10's ADR

---

## H-14 — The NATS adapter: the optional bus, optionally **[G]**

*Depends on: G-02's CloudEvents (shipped). Independent of everything here.*

The optional JetStream transport ADR-0007 has named since the beginning: an outbound adapter
behind the event bus port that publishes the same CloudEvents the dispatcher already delivers,
for operators whose consumers speak NATS rather than webhooks. Off unless configured; when
configured, it is one more subscriber on the dispatcher — at-least-once, the consumer-side
dedupe library G-02 shipped applies, replays are withheld exactly as they are from every
subscriber that does not opt in. The degradation row observability-reliability.md §7 has always
carried is proved: NATS down means the circuit opens, nothing is lost (the outbox holds), and
delivery resumes on return without a restart — RT-1's container discipline, applied to a new
dependency.

The task opens with the dependency ADR (decision 4): the client library, pinned, licence-gated,
behind the adapter and imported by exactly one package — `gate-architecture` proves it the way it
proves `cel-go`'s confinement.

**Acceptance:** the ADR is accepted and the dependency pinned first; with NATS configured, every
emitted event arrives on the stream as a CloudEvent validating against its `api/events/` schema;
without configuration, nothing changes and no connection is attempted; stopping the NATS
container opens the breaker, `/meta/health` and the degraded-mode metric say so, the outbox
backlog drains on return, and no event is lost or duplicated beyond at-least-once, proved in the
RT-1-shaped test; replays are not published; the library imports in one package only; the
configuration surface is documented in deployment.md §6.1.

**Read:** ADR-0007; G-02 in `milestone-0.5.0.md`; observability-reliability.md §6, §7;
the ADR this task writes

---

## H-15 — Mail intake by IMAP: AM-1 answered **[G]**

*Depends on: G-11's parser port (shipped). The last intake transport, or the recorded decision
not to have one.*

Open point AM-1 comes due with its four questions, and the draft ADR answers them before any
code: **which** IMAP client library — a supply chain decision about code that reads hostile input
on behalf of every tenant; **where the credentials live** — a mailbox password is a stored
credential, sealed through E-02 like every other; **who polls** — a per-tenant job seeded by the
write that configures the mailbox, because nothing enumerates tenants; and **what happens to a
mailbox nobody can reach** — a poll failing for a week is an inbox silently not arriving, so
sustained failure surfaces as a warning in `/meta/health` and a notification to the configuring
administrator through C-09's machinery. The owner accepts the ADR — and "the webhook bridge is
enough, no IMAP" is an acceptable answer that closes AM-1 by decision rather than by code; the
task then ends with the documents saying so.

If the ADR says build: the adapter feeds G-11's transport-independent parser — the port cut there
is what makes this an adapter rather than a rewrite — with polled mail deleted or flagged on the
server only after the entry is durably stored, the poll bounded by `HUBTASK_MAX_MAIL_BYTES`
before allocation, and the mailbox configuration a tenant-admin surface with the sealed secret
shown never (it is theirs already; nothing needs showing).

**Acceptance (if built):** the ADR is accepted and the dependency pinned first; a mail placed in
a test mailbox becomes a jumble entry through the same parser path G-11 proved, attachments and
all; the credential is sealed, masked everywhere, and rotatable; the poll job is self-seeded by
the configuration write, backs off on failure, and a week of failure has raised the warning and
notified the administrator (clock-driven test); a mail is never lost between fetch and store,
proved by killing the worker mid-poll; the mailbox surface is audited and cross-tenant negative
tested. **Acceptance (if not):** AM-1 is closed in automation.md with the reasoning, the arc42
context row loses its "or IMAP poll" ambiguity, and the roadmap stops implying it.

**Read:** automation.md §5 (AM-1 — the four questions are quoted there); G-11 in
`milestone-0.5.0.md`; multi-tenancy.md §2.1; E-02's sealing; C-09's notification path

---

## H-16 — hubctl grows with the milestone **[G]**

*Depends on: H-01 … H-15. The last task.*

The dogfooding client learns the milestone's verbs: `hubctl login` (password, the TOTP prompt
when challenged, the session held for the run), `hubctl session ls/revoke`, `hubctl mfa
enroll/disable` (the provisioning URI printed once, stderr warning — the `calendar mint`
discipline), `hubctl admin tenant create/suspend/resume/delete/export` against a multi-mode
stack, `hubctl quota show`, and the OAuth2 dance scripted (`hubctl oauth ...`) far enough that
the e2e session registers a client, consents, and calls the API with the issued token. Step-up
appears where the verbs need it: the CLI re-prompts and retries the privileged call.

The scripted session gains the milestone's proof: provision a tenant in multi mode, sign its
owner in with a password, enroll TOTP under tenant enforcement, run a **destructive restore
through a real step-up** — the round trip that has been refused since 0.4.5, now the drill's
closing scene — export the tenant and verify the archive against its format document, and watch a
second tenant stay invisible throughout. New sections observe the rate-limit discipline the
budget taught (`curl --retry` where the session's own 429s are the risk), and the support matrix
rows still pass on every platform B-15 declared.

**Acceptance:** the scripted session runs the sequence above green against the Compose stack in
multi mode; every new verb renders errors through the message-code catalogue and `--json` stays
pipeable; secrets and provisioning URIs print once with warnings on stderr; the destructive
restore section exists and passes — the E-12 stopping point is deleted, not skipped; the support
matrix jobs stay green.

**Read:** B-13, C-13, D-09, E-12, G-13 precedents; `scripts/hubctl-e2e.sh` (the stopping point);
support-matrix.md; H-03, H-06, H-07

---

## The order at a glance

```
H-01 ─┬─ H-02 ─── H-03 ─┬─ H-06 ─── H-07 ─┐
      ├─ H-04           │                 │
      ├─ H-05           │                 │
      └─ H-12 ←─ H-08 ─┬┘                 ├─ H-16
H-09 ─────────┬────────┴─ H-11 ──────────┤
H-10 ────────┤                            │
H-13  H-14  H-15 (independent) ───────────┘
```

H-01 opens the auth track and H-02…H-05 fan out from it; H-03 is what H-06's deletion and the
restore drill wait for. H-08, H-09, H-10, H-13, H-14 and H-15 start independently — H-10 and
H-13 early, because they wait on the owner's decisions, not on code. H-11 runs late by design: it
measures the milestone with partitions in place and fairness to prove. H-12 closes the
observability track once A-15 and A-18 have something to watch. H-16 is last, as always: it
consumes every surface the others opened.

**Definition of Done for the milestone:** a person signs in, refreshes, sees and revokes their
sessions, and the invitation loop closes; a tenant enforces TOTP on its administrators and a
destructive restore demands and receives a real step-up — the 0.4.5 refusal is flipped end to
end; a company signs in through its own IdP and a third-party app through OAuth2 code+PKCE, both
landing in the one session model; a multi-mode installation provisions, suspends, exports and
hard-deletes a tenant with evidence, and the export's format is documented well enough to import
from; quotas refuse at their bounds with their codes, the queue shares fairly under a storm, and
an unaffordable query is refused before it runs; the three large tables partition with retention
dropping months instead of deleting rows; load shedding holds interactive P95 under deliberate
overload (RT-6 green in the nightly), and the two-tier harness records internal baselines with
the per-vCPU figure; PITR is decided, implemented in the decided shape, and drilled per release
(RT-9), with D-1, D-2, B-2, B-3, S-2, A-1, P-5, P-6, O-1, CI-1 and AM-1 all closed in their
documents; the full alert catalogue A-01…A-18 ships with runbooks and the two promised
dashboards; the NATS adapter publishes schema-valid CloudEvents when configured and costs nothing
when not; every new dependency arrived through an accepted ADR; and the scripted session proves
the whole of it — multi mode, sign-in, MFA, step-up, restore, export — against the Compose stack.
