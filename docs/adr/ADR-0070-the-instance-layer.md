# ADR-0070 — The instance layer: operators, instance settings, and the elevated session

**Status:** accepted · **Date:** 2026-09-26

## Context

Hubtask has had a control plane since H-06: `/admin/tenants` provisions, suspends, resumes,
deletes and exports workspaces, `/admin/encryption` holds the key ring, `/admin/tenants/{id}/quotas`
sets limits, and `/meta/health` answers the whole report to the one credential entitled to it. It
is reached with a personal access token carrying `admin:tenants`, minted behind a step-up, and
driven through `hubctl admin`.

Working out the sign-in concept made three gaps in that plane concrete, and each is a fact read
from the source rather than a preference:

1. **There is no operator register.** `core/application/service/admin/Provision.go` says it in its
   own words — "Authorisation here is the scope alone" — and every admin use case calls
   `actor.RequireScope("admin:tenants")`. Minting that scope needs a fresh step-up
   (`AccessToken.go`) and **nothing else**: any account that can pass a step-up can mint a token
   that provisions, suspends and deletes every workspace on the installation. On a private
   installation that is correct, because the owner *is* the operator. On a platform with a
   thousand workspaces it is not.
2. **There is nowhere to put an installation-wide setting.** What applies to every workspace today
   lives in environment variables read at start: rate limits, SMTP, the base URL. There is no way
   to say "this value applies to every workspace **and a workspace may not change it**", which is
   precisely what [ADR-0068](./ADR-0068-sign-in-policy-and-the-password-lifetime.md) needs and
   what a provider serving B2C beside B2B needs for its legal links.
3. **The control plane has no surface a person can look at**, and the credential it demands is one
   a browser should not hold: `admin:tenants` is deliberately carried by **no session**
   (`catalogue.SessionScopes`), so a dashboard would have to ask for a long-lived all-powerful
   token and keep it.

One precedent matters for all three. The schema already holds installation-wide rows:
`backup_target` with `tenant_id IS NULL` is "an instance-wide target", `item_capability_profile`
with `tenant_id IS NULL` is "readable by every tenant, writable by none", and `job` has no policy
at all because "a worker has to be able to claim a job before it can know whose it is". The shape
exists; what differs between those three is *who may read the NULL row*, and that is a decision
rather than a detail.

## Decision

### 1. An operator register, before anything else

A table of accounts that are operators of this installation, checked **when the scope is minted**
and **when it is exercised**. Seeded from `HUBTASK_OPERATORS` (addresses with their workspace) so
that a fresh installation has one before it has a UI, and maintained through the control plane
afterwards, with the rule that the last operator cannot remove themselves.

**In single mode the register is empty and that means the owner.** A private installation changes
in no way; it has one workspace and its owner is its operator, which is what is true today.

*Amended 2026-09-28 (SI-17).* "The owner" is the whole of it, and the first build read it as
"anybody": `is_operator` answered *yes* to every account of every workspace while the register was
empty. On the private installation the sentence describes, those are the same set. On a shared one
— the arrangement multi mode exists for, before its operator gets round to adding themselves —
they are not, and every guest was an operator. The empty-register branch now requires an active
`OWNER` membership at tenant scope, which is what §1 meant and what was true before the table
existed. Adding the first operator turns the branch off, unchanged.

*Amended again 2026-09-29 (SI-12).* "Every active owner" is still not what the sentence says: it has
**one workspace** in it, and the walk of the finished screens showed why that matters. A workspace
provisioned from the dashboard has an owner, and the moment that owner became active they were an
operator of the whole installation — able to suspend and delete every other customer. The branch is
now bound to an installation that has exactly one workspace. More than one, and the register has to
be filled, because an installation hosting customers with nobody registered to run it is a
misconfiguration rather than a state with a default.

It cannot lock anybody out, and the order things happen in is the reason: provisioning a second
workspace needs `admin:tenants`, which on an empty register only the single workspace's owner can
mint. By the time there are two, somebody was an operator and could have registered themselves.
("The oldest workspace's owner" was tried first and is worse — measured on the walk database, the
oldest workspace had no active owner at all, which left the installation with no operator and no
way to appoint one.)

**A service account may be an operator.** A purchase platform that provisions workspaces needs a
credential that does not belong to a person who may leave, and the first day of a platform is the
day that becomes true — not the day plans arrive.

### 2. Instance settings: one table, read by all, written by the plane

`instance_setting` holds key, value, lock, and who changed it when. Two properties decide its
shape, and they pull in opposite directions:

- **Every workspace must be able to read it** — the effective sign-in rule is resolved on every
  password screen.
- **No tenant may write it.**

The standard policy `tenant_id = current_tenant_id()` makes a NULL row invisible to everybody,
which is exactly what it does to `backup_target`'s instance-wide rows today. The
`item_capability_profile` policy (`tenant_id IS NULL OR tenant_id = current_tenant_id()`, with a
`WITH CHECK` that no tenant satisfies) reads correctly but its writes are the *migrator's*, at seed
time, not the application's at run time.

So: **`instance_setting` carries no row-level policy, like `job`**, and is bounded by grants and by
the one place that writes it — the control-plane use cases, behind `admin:tenants` and the register.
It holds installation configuration and never a person's data, which is what makes that acceptable.
The exception is entered in **all three** lists that must agree about the tenant boundary: the
policy block in `db/schema.sql`, the reasoned map in the boundary integration test, and the
restore drill's own copy.

The workspace's own value stays in `tenant.settings`, behind the policy, where it belongs.

### 3. The lock carries its origin

A lock is `INSTANCE` or `PLAN` (the second has no writer yet; plans are their own milestone). The
screen says which, because "ask your administrator" and "ask your provider" are different
sentences and a reader who is told the wrong one writes the wrong mail. Three cheap preparations
land with this ADR so the plan layer is not a migration through the sign-in path later: the
resolver takes the plan as a parameter, `tenant.plan_id` exists as a nullable column, and the lock
carries its origin from the first row.

### 4. The elevated session

A registered operator raises **their existing session** to `admin:tenants` by passing a step-up.
The elevation lasts **one hour, is not renewable, belongs to that one session, ends with it**, is
written into the instance journal at both ends, and its remaining time is on the screen.

This deliberately weakens the rule that `admin:tenants` is never carried by a session. It weakens
it to: *only for a registered operator, only after a fresh proof, only for an hour, only on the
session that proved it, and written down.* What it buys is that nobody has to mint a long-lived
all-powerful token and paste it into a browser to change a switch — which is the outcome the
strict rule produces in practice, and which is worse.

The personal access token stays exactly as it is, for automation.

### 5. One API, three doors

The API is the product; `hubctl` and the dashboard are two clients of it, and a file is the third:

- **As code:** `HUBTASK_INSTANCE_FILE` with `seed` (written once, at first start) or `enforce`
  (written every start, and the writing routes refuse with a clear code). For GitOps and immutable
  containers. **One source per mode, never two**, and the health report says which is in force.
- **`hubctl admin`:** the group exists and gains `settings`, `operator`, `legal` and `provider`.
- **The dashboard:** a route area `/instance` in the same web app — not a second bundle, not a
  second frame, not a second translation. It shows workspaces and their lifecycle, the instance
  values with their locks, the operators, the health report and the journal.

**And never the contents of a workspace.** The tenant boundary is a database policy rather than a
role, and the dashboard does not go around it. What an operator needs to run an installation is
counts, states and limits — not rows.

*Amended 2026-09-28 (SI-12, SI-17).* Three doors to one API means three doors to the **same verbs**,
and the first build gave the dashboard only the reads. That is not a door; it is a window. Two
sentences settle it, and they are the same sentence read in both directions:

- **Whatever the API serves and the configuration permits, the dashboard offers.** Every verb of
  the control plane has a control on the screen that already shows its subject — the workspace
  lifecycle and its limits, the instance values, the providers this installation offers — so that
  running an installation does not require a terminal. Where the configuration closes a door, the
  screen draws no control at all rather than one the server refuses: `HUBTASK_INSTANCE_FILE` in
  `enforce` mode is the case that exists today, and `is_enforced_from_file` is what says so.
- **One exception, and it is named here so it is not read as an oversight:** the key ring. It stays
  in the environment ([ADR-0045](./ADR-0045-master-key-in-the-environment.md)) and the dashboard
  *shows its census and does not turn it*. A rotation needs the new key in the process before the
  first value is re-sealed; a button in a browser could start one for a key nothing is holding,
  which ends with values no key opens. The screen says that where the buttons would be.

**And the area is offered to an operator and to nobody else.** Not disabled for everyone else —
**absent**. The manifest answers `instance.reachable`, caller-scoped like every other entry in it,
read against the operator register; the navigation row exists only where that is true. A greyed row
would tell a guest on somebody else's installation that a control plane is there and that they are
outside it, which is a question they then have to ask somebody. `CapabilityGate` is for a refusal a
person might otherwise have expected; this is the other case, where they never might have.

### 6. What does not move to the instance layer

Rate limits and the lockout curve (protection from an attacker is not a preference) · the key ring
([ADR-0045](./ADR-0045-master-key-in-the-environment.md)) · a workspace's name, colour and start
page · the language and time zone, which the instance may *default* and may never lock, because a
company that cannot work in its own language because the operator set a switch is a product defect.

## Options

1. **A register, a settings table without a policy, and an elevated session (chosen).**
2. **Leave it in the environment.** No lock, no change without a rollout, and no answer for B2C
   and B2B on one installation.
3. **A NULL-tenant row under the capability-profile policy.** Reads correctly, but its `WITH CHECK`
   forbids the run-time write this needs, and loosening that policy loosens it for a table every
   tenant reads.
4. **A separate control-plane database.** What `multi-tenancy.md` §2 keeps as the growth path for
   sharding. It is the right answer at a scale this installation is not at, and it is a second
   datastore ([ADR-0003](./ADR-0003-postgresql-as-single-datastore.md) says why that is expensive).
5. **A separate operator application.** A second frame, a second bundle, a second translation and a
   second sign-in for one list of workspaces.

## Consequences

**Positive.** A platform operator can set a value once for every workspace and fix it. A workspace
sees what it may change and who decided the rest. Provisioning has a credential that is not a
person's. The control plane becomes visible to the operator who runs it without becoming visible
to anybody else.

**Negative.** A table outside the tenant boundary is a table three lists have to agree about, and
those lists have been wrong before. The elevated session is a real weakening of a rule that was
absolute, and it is only as good as the register behind it. The dashboard is a screen area that
has to be excluded from the mobile shells' route areas ([ADR-0032](./ADR-0032-client-capability-matrix.md)).

**Countermeasures.** The exception list is entered in all three places in the same commit, and the
boundary test names the reason rather than the table. Every elevation, every settings write and
every operator change is in `instance_event`, which is append-only. The `/instance` area is tagged
in the route table, and `routes.test.ts` already asserts that the tagged set and the prefix agree.

**What this does not decide.** Plans — the grouping of workspaces that carries limits, locks and
feature entitlements — are a milestone of their own. This ADR only makes sure they do not need a
second model: the resolver takes them, the workspace has the column, and the lock knows where it
came from.

## What is built, and what this decision still owes

SI built the layer itself and two of §5's three doors. The accepted decision stands whole; what
follows is the record of where the code is against it, so that nobody reads this document as a
description of what exists.

**Built.** The `operator` register with its four functions, `instance_setting` with the three
boundary lists entered, the lock with its origin, `tenant.plan_id`, the resolver's plan parameter,
`GET`/`PUT /admin/settings`, `GET`/`POST /admin/operators`, `DELETE /admin/operators/{accountId}`,
`POST /auth/sessions:elevate` with `session.elevated_until`, and the journal at both ends of an
elevation. **Since SI-17** the `/instance` route area, and the two reads it needed:
`GET /admin/overview` (the census) and `GET /admin/journal`. **Since SI-10** the providers in the
plural at both levels, `/admin/identity-providers` among them. **Since §5's amendment** the area is
seven screens rather than five — the providers and the key ring's census joined it — every verb the
control plane serves has a control, and `instance.reachable` in `/meta/capabilities` decides whether
the area is offered at all.

**Not built, each its own task.** `hubctl admin settings|operator|legal|provider`.
`HUBTASK_INSTANCE_FILE` in either mode, and therefore the health report's line saying which source
is in force — the instance values screen reads `source` and will say which door is in force the day
there is more than one.

**And one gap the plural providers opened.** The installation's own provider holds a sealed client
secret, and the re-seal's driver runs the resealers per tenant (`RunReseal` takes the actor's
workspace). `ListIdentityProviderSecrets` compares the tenant with `IS NOT DISTINCT FROM`, so a pass
under the installation's own scope would pick up exactly the rows that belong to no workspace — what
is missing is a driver that runs one. Until then an installation-level provider's secret stays under
the key it was sealed with, which a key rotation's census will report rather than hide.

**And one thing §1 says that the code does differently.** There is no `HUBTASK_OPERATORS`. The
bootstrap is the rule §1 already states for the private installation, used as the way in: an empty
register answers *yes* to `is_operator`, so the first `POST /admin/operators` on a fresh
installation is made by whoever can already mint the scope, and from that row onwards the register
is the bound. One mechanism instead of two, and no address parsed at start-up. An environment
variable can still be added later for an installation that wants the register present before its
first request; nothing here forecloses it.

## What the implementation settled

Three things decided while SI-05 and SI-06 were built, and five more while SI-10 and SI-17 were.

1. **The operator register keys on the account alone, and lives behind four functions.** §1 does not
   say how it is reached. It carries no row-level policy *and* no grant to the application role:
   unlike `instance_setting` its rows name accounts across workspaces, so a policy-free table
   `hubtask_app` could read would let every workspace enumerate the installation's operators. The
   four doors are `is_operator`, `operator_register`, `add_operator` and `drop_operator`, each
   `SECURITY DEFINER` and each narrow by construction - `resolve_tenant`'s discipline applied to a
   table. The workspace is read from the account by the function rather than named by the caller,
   which is also what keeps rule 3 intact: no repository method here takes a tenant.

2. **The last-operator rule is in the statement.** `drop_operator` deletes only while
   `(SELECT count(*) FROM operator) > 1`, because two operators removing each other at the same
   moment would both read "there are two".

3. **The elevation does not slide, and the register is read again on every request.** §4 says "one
   hour, not renewable": what that means in code is that activity extends a session's own horizon
   and never `elevated_until`, and that a second hour needs a second proof. And the scope is granted
   per request rather than at the elevation, so an operator removed while a raised session is open
   loses the control plane on their next call rather than at the end of the hour.

4. **The census is a fourth `SECURITY DEFINER` function, and it answers five integers.** §5 says the
   dashboard shows counts and never rows; the overview needs a count of accounts *across*
   workspaces, and `account` is behind row level security and `FORCE`, so the application role
   cannot produce one at all. `instance_census()` is the narrow door for it — no parameter, five
   `bigint`s, and a caller that wanted rows would have to change the function, which is a migration
   somebody reviews. The workspace counts go through the same function rather than through
   `/admin/tenants`, because the overview's numbers have to agree with each other on one instant,
   which one statement gives and two do not.

5. **`/instance` is a fourth route area, not a second `administration`.**
   [ADR-0032](ADR-0032-client-capability-matrix.md) names three; the shells exclude this one exactly as
   they exclude administration, and the reason it is its own is the capability: an administrator
   runs a workspace and an operator runs the installation. A shell that shipped one because it
   shipped the other would be shipping the control plane by accident. It is drawn by the *same*
   section column as the other two, which is §5's "not a second frame" kept literally.

6. **What the dashboard may write is the server's answer, never the screen's guess.** §5's
   amendment says the dashboard offers whatever the configuration permits, and there are exactly two
   ways a screen could know what that is: ask, or decide. It asks. `is_enforced_from_file` on the
   settings document is what removes the controls from the instance values screen; the presets'
   `provisioning` list is what removes the modes a provider may not hold; `instance.reachable` is
   what removes the area. A client that worked any of these out for itself would be a second copy of
   a rule, and the copies disagree on the installation nobody tested.

7. **Absence is the refusal for a level, and `CapabilityGate` is the refusal for an entry.** The two
   look alike and are not. A person refused an *entry* was reaching for something they could
   plausibly have had, and `domain-model.md` §2 is explicit that this must never become silent
   ignoring — so the gate renders the reason. A person outside the operator register was never
   reaching for anything: they have no workspace where the control plane applies, and a disabled row
   is a fact about somebody else's installation that they cannot act on. The rule this settles, for
   the next level somebody adds: **explain a refusal, omit a level**.

8. **The journal gained a read, and the port's own comment was the thing that changed.** It said
   "there is no read method because no API serves it — reading it is the operator's, at the
   database". §5's dashboard is the API that serves it, so `Journal` has a `Page` now: newest first,
   keyed on the moment *and* the identifier, because two entries can share a moment and an offset
   would then skip or repeat one.
