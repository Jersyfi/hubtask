# ADR-0070 — The instance layer: operators, instance settings, and the elevated session

**Status:** proposed · **Date:** 2026-09-26

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
