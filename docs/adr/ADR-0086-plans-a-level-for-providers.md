# ADR-0086 — Plans, a level for providers

**Status:** accepted · **Date:** 2026-10-15

**Rule lives in:** [multi-tenancy.md](../architecture/multi-tenancy.md) §4.2, §4.3,
[identity.md](../architecture/identity.md) §18

## Context

[ADR-0070](./ADR-0070-the-instance-layer.md) §3 left plans — the grouping of workspaces that
carries limits, locks and feature entitlements — to a milestone of their own, and prepared three
things so they need no second model: the resolver's plan parameter, `tenant.plan_id`, and a lock
that carries its origin. Read against `main` on 2026-10-15, the preparation is thinner than the ADR
says:

* the sign-in resolver takes a plan, but applies the plan's value over an installation lock and
  never asks whether it tightens — the opposite of P-07, harmless only because no plan exists;
* the quota resolver and the legal-link resolver take no plan at all;
* `instance_setting` stores `PLAN` as an origin, and every read collapses it into a boolean;
* nothing in Hubtask is a feature a workspace is entitled to or not.

[UC-INS-13](../usecases/admin/UC-INS-13-put-workspaces-on-plans.md) asks for plans with their own
limits, features and locks, so that consumers and companies live on one installation, and a
downgrade that stops rather than deletes.

## Decision

1. **Plans prepare Hubtask for a provider's platform, and restrict nothing by themselves** (the
   owner, 2026-10-15 and #1550). A provider sells plans through its own purchase or billing
   platform; the platform pushes the plan in and reads use and events out (P-15, `NG-billing`), and
   Hubtask never asks it anything. Hubtask ships no plan. A workspace without one has every feature and the
   installation's limits — exactly what it has today — and a self-hosted installation that never
   defines a plan never meets one. Plans exist only in multi mode.
2. **A plan is an installation-level row that holds only what it decides.** Its values are sparse:
   what a plan does not set falls through to the installation, and nothing is copied into a
   workspace, so a change to a plan is in force at the workspace's next request. Like
   `instance_setting` it is read by every workspace and written only by the control plane; it holds
   configuration, never a person's data.
3. **One order, one rule for every area.** `product → installation → plan → workspace`. An
   installation lock binds the plan as it binds the workspace; a plan tightens what the installation
   left open — a limit may go higher where the installation's key says workspaces may — and may lock
   it, with the origin `PLAN`. The operator's exception for one workspace stays the last word on a
   limit, as ADR-0070's seventh settlement says. The legal links gain the plan level (#1531).
4. **A closed set of three entitlements** (#1530): automation rules, webhooks, and the workspace's
   own sign-in providers. Left out, each rests — it stops running and keeps everything it holds —
   and comes back from the moment it is included again. Everything else a plan reaches through
   values, locks and limits. Export, deletion, access requests, language, time zone, accessibility
   and every door (P-08) are no entry of the set, so no plan can switch them off.
5. **A limit on people** (#1533): `accounts` counts active and managed accounts and open
   invitations, never service accounts; reaching it refuses the next way a person would be added,
   and nobody existing is touched.
6. **Every door, a version, and the record** (#1532): plans are written through the screens,
   `hubctl admin plan`, the API and the instance file, with a version on every save; a workspace can
   be provisioned on a plan; a plan with workspaces on it cannot be removed. Moving a workspace is in
   the journal and its trail; changing a plan is in the journal.
7. **The platform's path is checked, not implied** (#1550): a plan's short name never changes and
   is never reused; a workspace carries the platform's own reference; a move bound to a version is
   refused when stale, so a late retry cannot undo a newer change; one workspace is read and the
   list filtered for reconciliation.
8. **The platform learns what happened and what was used** (#1551, UC-INS-14 in PL): versioned,
   state-carrying events, pushed and readable forward from a cursor, never switched off silently;
   use per day with its day basis and a final day, people and storage as each day's highest value.
9. **A platform ends what it began** (#1552): a service account in the register requests a deletion
   without a step-up, under the same grace and refusals.
10. **A shop is not the installation** (#1553): `admin:provisioning`, a subset of `admin:tenants`.
11. **A person is told whom to ask** (#1554): one account link per installation and plan, labelled
    by Hubtask, never a price.
12. **Use for billing is opt-in and outlives the purge** (#1555): offered only while metering is on;
    13 months as bare counts, also after a workspace's purge.

A research pass on 2026-10-16 compared the cut with how hosted, plan-based software is driven by
billing systems — a provisioning and plan-change lifecycle, marketplace fulfilment and metering
interfaces, signed webhooks with replay — and found decisions 7 to 12 missing; each was then put to
the owner.

## Options

1. **A sparse plan between installation and workspace (chosen).**
2. **A plan as a template copied into the workspace.** Changing a plan would need a job that visits
   every workspace (UC-INS-13 check 2 forbids it), and a downgrade would overwrite what the
   workspace set.
3. **Entitlements as limits set to zero.** The screen would say "limit reached" where it should say
   "not included", and a rule would fail rather than rest.
4. **An open list of switchable features.** Every use case would have to answer "and if the plan
   leaves it out?", and a door switched off per plan breaks P-08.

## Consequences

**Positive.** A provider sells "Private", "Family" and "Business" on one installation and moves a
customer between them at once. Every screen that already says "Set by the installation" says "Set
by your plan" through the same field. A self-hoster sees nothing new.

**Negative.** Four resolvers learn a fourth level at once, and one of them is corrected on the way;
a plan is one more table outside the tenant boundary that the three boundary lists must agree on.

**Countermeasures.** One task per pair of resolvers, each with table tests of the lock and tighten
rule at every level; the boundary lists are entered in the commit that creates the table.
