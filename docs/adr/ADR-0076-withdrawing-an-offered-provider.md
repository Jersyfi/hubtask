# ADR-0076 — Withdrawing an offered sign-in provider: a count, a notice, and a way back in

**Status:** accepted (amended by [ADR-0077](./ADR-0077-nobody-is-locked-out.md): §1, §2, §4; by [ADR-0078](./ADR-0078-the-ways-back-in.md): §4) · **Date:** 2026-10-01 · **Accepted:** 2026-10-01

## Context

An installation offers sign-in providers to every workspace
([UC-INS-11](../usecases/admin/UC-INS-11-offer-sign-in-providers-to-every-workspace.md)); each
workspace switches an offered provider on for itself in its list of ways to sign in
([UC-ID-12](../usecases/identity/UC-ID-12-set-the-workspaces-sign-in-rules-in-one-place.md)). SC-06
made the last remaining way in impossible to switch off **inside a workspace**. Two doors are left
(UC-ID-12 *Today*):

* **The installation's own door.** An operator who withdraws or removes an offered provider can leave
  a workspace that uses it as its only way in with none. Guarding it the way SC-06 guards a workspace
  would mean reading every workspace's choice, which an installation action may not do
  ([P-01](../vision/principles.md#p-01-workspaces-never-see-each-other)); and blocking the withdrawal
  while anybody uses the provider would make a compromised provider impossible to switch off.
* **`enabled` on a provider's configuration.** `PUT /identity-providers/{id}` still accepts `enabled`,
  a second place for the switch the list holds
  ([P-06](../vision/principles.md#p-06-one-setting-one-place-every-level-visible-there)).

The owner agreed the shape on 2026-10-01 after it was put to him against deployments `D1`–`D7`.

## Decision

### 1. A number, not names

An offered provider carries a count of the workspaces that have it switched on. The count is kept by
the workspace's own switch — the write that turns the provider on or off in a workspace moves it in
the same transaction — and by nothing else. It is a number: the installation learns how many, never
which ([P-01](../vision/principles.md#p-01-workspaces-never-see-each-other)), and no job walks
tenants.

### 2. A withdrawal is announced, by default fourteen days ahead

Withdrawing an offered provider asks the operator for a date and shows the count: "12 workspaces use
it. Withdrawn on 15 October." Fourteen days is the default. Until that date:

* the provider keeps working everywhere;
* every workspace that has it switched on shows its administrators, on the sign-in screen, that the
  installation withdraws it on that date and that another way should be switched on;
* the operator can cancel the withdrawal
  ([P-04](../vision/principles.md#p-04-destruction-announces-itself-and-can-be-stopped)).

On the date the offer ends as UC-INS-11 check 5 says: switched off everywhere, connected identities
left in place, so offering it again restores sign-in. The date is honoured where the offer is read —
the sign-in and the list resolve "offered" against it — so no scheduled job is needed.

### 3. Withdraw now, for a compromised provider

*Withdraw now* stays available, behind a step-up and a confirmation that repeats the count. It is the
answer to a compromised provider, and the reason the withdrawal is never blocked.

### 4. The fail-safe: no workspace is left without a way in

If a workspace has no way to sign in left once an offer has ended, the resolution of its ways
re-opens the **password** — for accounts that hold one, under the workspace's own password and
second-factor rules. The workspace's trail records it, and its administrators see it on the sign-in
screen until they switch on another way, which ends the fallback. Accounts without a password gain
nothing: nothing is weakened for them (P-02), and an administrator re-invites them.

### 5. `enabled` lives in the list only

`PUT /identity-providers/{id}` refuses a change of `enabled` with `identity_provider.switch_in_list`,
a sentence pointing to the list of ways to sign in; the same value is accepted, so a client that
echoes the field keeps working. A provider is created switched off. The field is marked `deprecated`
and removed with the next major version of the contract.

## Consequences

* `D1`–`D3` are not touched: they rarely have installation providers.
* `D5` and `D6` get an honest withdrawal: the operator sees what it affects, the workspaces are told,
  and nobody is locked out on the day.
* The audit gains the withdrawal, its cancellation and the fallback as actions; the count is not
  personal data.

## Options considered

**A. Block the withdrawal while any workspace uses the provider.** Rejected: a compromised provider
could not be switched off.

**B. Name the affected workspaces to the operator.** Rejected: an installation does not read inside
workspaces (P-01).

**C. Withdraw immediately, as built.** Rejected as the default: it locks workspaces out without
warning (P-04). It stays as *Withdraw now*.

**D. Ignore `enabled` silently on `PUT`.** Rejected: a client would believe it switched a provider
that did not change (P-11).

**E. A count, a notice period, *Withdraw now*, and a password fallback (chosen).**
