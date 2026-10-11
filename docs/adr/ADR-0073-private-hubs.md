# ADR-0073 — A private hub narrows what the workspace's roles reach

**Status:** accepted · **Date:** 2026-09-30 · **Accepted:** 2026-09-30
**Rule lives in:** [identity.md](../architecture/identity.md) §22; the emergency access (§4) and the
setting (§6) are not built yet

## Context

Rights in Hubtask only add up: the effective role is the highest held anywhere along the path from
the workspace down ([ADR-0005](./ADR-0005-authn-authz.md),
[domain-model.md](../architecture/domain-model.md) §3.2). Whoever is owner or administrator of a
workspace therefore reaches every hub in it. For a company that is the point. For a household it
makes "private" a promise: two parents who both run the family's workspace can each open the other's
diary, and a teenager's list is open to every adult with a role.

[UC-ID-16](../usecases/identity/UC-ID-16-keep-a-hub-private.md) asks for a hub that another
administrator cannot open. Two things pull the other way. The workspace owner is the controller:
erasure, legal holds, export and retention are their duties and need the data. And households have
emergencies — illness, death — in which somebody must be able to get at what was kept.

The owner decided the shape on 2026-09-30, measured against deployments `D1`–`D7`.

## Decision

### 1. A hub may be private, and a private hub is reached only by its own members

A hub carries `private`. For a private hub, `EffectiveRole` ignores every membership held **above**
the hub on the path: only a membership on the hub itself, or below it, grants anything there. This is
one rule in one place in the domain service — not a second kind of role and not a deny list.

Everything that reads through the authoriser follows: navigation, the hub list, search, the overview,
the calendar feed, saved views, automation rules and AI run on another person's behalf. None of them
reaches into a private hub the person is not a member of.

### 2. Who may make one

Any person may create a private hub **for themselves** where the workspace allows private hubs, and
becomes its owner-member; they may add other members to it. A person who may already create hubs may
also mark an existing hub they own private. Without this, a family member without an administrator's
role — the one who needs privacy most — could not have one.

### 3. What administrators see

That private hubs exist, whose they are, and how much they hold — as rows without names and without
contents. Enough to run the workspace (storage, a person leaving) and nothing to read.

### 4. The owner's emergency access is never secret

The workspace owner — only the owner — may open a private hub after a fresh proof and with a stated
reason, for one hour, not renewable. Opening it **notifies every member of the hub at once** ("Maria
opened your private hub on … — reason: …") and is recorded in the workspace's trail. Transparency
instead of secrecy is what reconciles the controller's duties and a family's emergencies with the
promise of privacy: nobody reads along unnoticed.

### 5. What still applies

Backups and the workspace export contain private hubs — the data belongs to the workspace — marked as
private, and a restore keeps the mark. Retention rules and legal holds apply as everywhere; a rule is
not a reader. When the last member leaves, the hub goes to the trash with the ordinary grace period
and the owner is told; they may take it over through the emergency access or let it go (P-03, P-04).

### 6. One setting, one place

*Private hubs allowed* is a workspace setting under Administration, default **on**, with a default
and a lock at the installation and the plan ([ADR-0070](./ADR-0070-the-instance-layer.md)). A company
may switch it off for traceability; a B2B provider may lock it for its customers; a family plan may
leave it on. Switching it off makes no existing hub public: existing private hubs stay private, and
no new ones can be made.

## Consequences

* `D2` gets real privacy inside one shared workspace, with an emergency exit that cannot be used in
  the dark. `D3` gets personal space beside client work. `D4`/`D6` decide per company. `D1` sees
  nothing of it.
* The authoriser's path gains the hub's `private` flag; every repository narrowing by readable
  scopes already asks the authoriser, and a gate test lists the readers that must honour it.
* A new notification category for the emergency access, and an audit action for it.

## Options considered

**A. One workspace per person.** Rejected: two accounts, two addresses, no shared home
([NG-global-identity](../vision/non-goals.md)).

**B. Encrypting private hubs.** Rejected: search, rules and AI would stop working inside them, and
a lost key loses the content ([NG-e2e-encryption](../vision/non-goals.md)).

**C. Absolute privacy, no owner access at all.** Rejected: incompatible with the controller's
duties and with a household's emergencies.

**D. Private with transparent emergency access (chosen).**
