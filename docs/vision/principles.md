# Product principles

Sixteen principles every use case is measured against. They are about what a **person** meets,
not about how the code is layered — the engineering rules are in [`CLAUDE.md`](../../CLAUDE.md)
and the ADRs, and several principles here are the reason those rules exist.

**When two principles pull in different directions, the lower number wins.** A use case names the
principles it serves in its `serves:` field; a principle nobody serves is either finished or
forgotten, and the index shows which.

Each principle has three parts: the rule, why it holds, and **how you notice it is broken** — the
part a reviewer uses.

---

### P-01: Workspaces never see each other

**Rule.** Nothing of one workspace — a title, a name, a count, the fact that an address exists —
reaches another workspace or the people running the installation. An operator sees numbers and
states, never rows.

**Why.** The tenant boundary is the reason Hubtask exists as a multi-tenant product. It is enforced
by the database, not by a role, so no screen and no operator can go around it
([ADR-0010](../adr/ADR-0010-multi-tenancy.md), [ADR-0070](../adr/ADR-0070-the-instance-layer.md) §5).

**Broken when** a screen, an export, an error message, a log line or a metric lets somebody infer
something about a workspace they are not in — including "this address already has an account over
there".

### P-02: An account is opened only by its own strongest proof

**Rule.** A person's account is never entered, taken over, linked or recovered on the strength of a
weaker proof than the one it already holds. An account with a password and a second factor is not
opened by an address a provider vouches for, by a mailbox alone, or by an administrator's say-so.

**Why.** Every other promise — privacy, audit, ownership — depends on the account being the person.
A second way in that is weaker than the first *is* the way in.

**Broken when** any path — a provider sign-in, a reset, a link, an invitation, an administrator
action — ends in a session for an account without the proof that account would demand at the
front door. Found in the SI review: a provider sign-in linked an existing account by address and
skipped its second factor.

### P-03: Nothing is lost

**Rule.** Work is never lost silently: deleting goes to the trash, archiving is reversible, a
restore is drilled, an offline edit merges instead of overwriting, a downgrade of a plan or a
switch turned off stops something rather than deleting it.

**Why.** "Data is never lost" is what the end user expects above everything else
([arc42](../architecture/arc42.md) §1.3), and it is the proof the product is sold on.

**Broken when** an action removes data without a way back that the person was told about, or a
configuration change makes stored data unreachable instead of dormant.

### P-04: Destruction announces itself and can be stopped

**Rule.** Anything that destroys data or access says what it will cost *in the dialog where the
person decides*, gives a grace period where the law and the case allow one, and can be called off
during it. A confirmation names the thing, not "Are you sure?".

**Why.** Retention rules that delete, a workspace deletion, a destructive restore, removing the
last operator — each is the one click somebody regrets.

**Broken when** a destructive action has no confirmation, a confirmation does not say what is lost,
or a text promises an undo that does not exist.

### P-05: What you may not do, you do not see

**Rule.** A control that a person may not use is **absent**, not disabled and not offered only to be
refused. Where a refusal is something the person could reasonably have expected to pass, the
screen says why ("Your workspace requires a second factor"). A level somebody is outside of — the
installation for a member, administration for a guest — does not appear at all.

**Why.** The owner's words from the SI walk: *"Wenn man eine Aktion noch nicht ausführen soll, dann
sollte diese auch nicht zur Verfügung stehen."* Every refused click teaches a person that the
product is unreliable.

**Broken when** a button leads to a server refusal the screen could have known in advance —
"Turn off the second factor" while the workspace demands one, "Change password" on an account that
has none, "New workspace" on an installation that cannot hold two.

### P-06: One setting, one place, every level visible there

**Rule.** Every rule is changed in exactly one place. That place shows the value in force, where
it comes from (Hubtask's default, the installation, the plan, this workspace) and whether it is
locked. Older fields that express the same rule are derived from it and read-only.

**Why.** Two controls for one rule will disagree, and the person cannot tell which one wins. Found
in the SI review: "Administrators need a second factor" under *Workspace* and "Second factor" under
*Sign-in* were two independent values, one of which bypassed the lock and the step-up.

**Broken when** the same question can be answered in two screens, or a screen shows a value
without saying where it came from when somebody else set it.

### P-07: The stricter level wins, and says who decided

**Rule.** Settings resolve Hubtask's product minimum → installation → plan → workspace. A lower
level may tighten where no lock lies, never loosen past the level above. A lock carries its origin,
and the screen says it: "Set by the installation", "Set by your plan".

**Why.** It lets a provider decide once for everybody, lets a company be stricter than its
provider, and tells a person whom to ask ([ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md),
[ADR-0070](../adr/ADR-0070-the-instance-layer.md)).

**Broken when** any path writes a value looser than the level above allows, or a person meets a
value they cannot change without learning who set it.

### P-08: Every door can do everything

**Rule.** Whatever can be done in the web app can be done through the API, `hubctl`, MCP and an
automation rule, with the same permissions and the same trail. A door that cannot do something is a
defect of that door, not a choice about scope.

**Why.** The API is the product ([ADR-0004](../adr/ADR-0004-api-first-openapi.md)); the parity gate
proves it for use cases. The principle extends it to the doors a person uses: a screen with a raw
text box for a number, or a `hubctl` command that cannot send a step-up, fails it too.

**Broken when** a task is only possible through SQL, only through the UI, or only by hand-crafting
a request a documented tool cannot produce.

### P-09: Yours to run

**Rule.** One image and PostgreSQL run the whole product. The first start leads to a working
workspace without touching the database. Nothing phones home, nothing loads from a foreign origin,
no licence key switches anything off, and every piece of your data can be exported.

**Why.** The sovereign individual is the first audience ([market analysis](../marketing/market-analysis.md)
§3.1), and "will this still be mine in five years?" is their question.

**Broken when** an installation step needs a database shell, a feature needs an outside service
the operator did not choose, or data can be put in but not taken out.

### P-10: The smallest deployment comes first

**Rule.** A feature must work, and make sense, for one person running Hubtask for themselves before
it is accepted for a platform. Concepts that only exist because of scale — installation, operator,
plan, provider — stay out of the way where they coincide with the person using it.

**Why.** The same product serves one person and ten thousand customers
([deployments.md](./deployments.md)). A private person asked to understand "the installation's
lock on the workspace's rule" has been handed a platform's problem.

**Broken when** a use case marked for `D1` requires the person to learn a term or visit a screen
that only means something in `D5`–`D7`.

### P-11: Honest about its own state

**Rule.** Hubtask says what it knows and what it does not: what is degraded, how long a code stays
valid, how many recovery codes remain, whether a check could not be made. It never shows a
spinner for something that failed.

**Why.** Self-diagnosis is a quality goal ([arc42](../architecture/arc42.md) §1.2) and the thing that
makes a self-hosted product operable by a person alone.

**Broken when** a failure looks like loading, a limit is met without warning, or "could not be
checked" is shown as "fine".

### P-12: Data from the server, sentences for people

**Rule.** The server answers codes and parameters; the client turns them into a sentence in the
reader's language. The words are plain, one concept has one name everywhere — *second factor*, not
2FA, TOTP and two-step side by side — and nothing internal leaks: no field names, no UUIDs a person
has to read, no "enrolment" or "armed".

**Why.** [ADR-0011](../adr/ADR-0011-i18n-message-codes.md) makes every language possible; this
principle makes every sentence understandable.

**Broken when** a person reads a raw key, an identifier, a field name, or two words for one thing.

### P-13: Accessible by number

**Rule.** WCAG 2.2 AA, checked by criterion: nothing sends itself, codes can be pasted, passwords
can be shown, focus follows the step, colour is never the only signal.

**Why.** Accessibility is a legal duty for many operators (BFSG, EAA) and a quality of the product
for everybody.

**Broken when** an input refuses what a person pastes, a keyboard cannot reach a control, or a
state is told only in colour.

### P-14: AI is optional, and nobody consents in another's place

**Rule.** The product with AI switched off is the whole product. Content is sent to a model only
after the workspace has consented, and no operator, plan or default gives that consent in its
place. Inside the workspace a person may keep what they authored out, unless the workspace makes AI
part of everybody's work on a named legal basis, which every person then sees instead of the
switch. Who processes the content, and where, is always visible.

**Why.** [ai-first.md](../architecture/ai-first.md) §2, [ADR-0049](../adr/ADR-0049-ai-provider-surface.md)
and [ADR-0072](../adr/ADR-0072-ai-at-the-installation-level.md) §3; a provider may offer a model,
never impose one. Only the controller can name a legal basis, so only the workspace can make AI
part of everybody's work ([data-protection.md](../architecture/data-protection.md) §4.1).

**Broken when** a feature stops working without AI; content leaves the installation without a
consent the workspace itself gave; or a person's content reaches a model after they objected,
without a legal basis shown to them.

### P-15: Hubtask holds state; the platform holds the business

**Rule.** Hubtask knows whether a workspace is active, which plan it is on, and what it has used.
It does not know prices, contracts, trials, invoices or dunning. A purchase platform pushes state
in; Hubtask never asks anybody whether it may run.

**Why.** It keeps a self-hosted installation self-hosted ([ADR-0018](../adr/ADR-0018-privacy-by-design.md))
and lets any platform sit in front of it.

**Broken when** a feature needs a price, a contract term or a call to a licence server to decide
what Hubtask does.

### P-16: Nobody is locked out

**Rule.** No decision of the platform or of a level above a person — a withdrawn provider, a
switched-off method, a tightened rule, a restore, an outage — takes away *whether* that person can
reach Hubtask and their data. It may change *how* they sign in. Where a door would leave somebody
with no way in, a way stays open or comes back, and the person is told.

**Why.** A person's work lives in Hubtask; losing the way in is losing the work, whoever caused it.
The owner set the rule on 2026-10-03 ([ADR-0077](../adr/ADR-0077-nobody-is-locked-out.md) §4,
[ADR-0078](../adr/ADR-0078-the-ways-back-in.md)).

**Broken when** any state of an account that a change can produce — a kind of account, a kind of
workspace, a deployment — is left with no way in that works, and nobody told the person what to do.
Its limits, where P-02 wins because no proof is left: a lost second factor with lost recovery codes,
which an administrator answers.

---

## Replaced

A principle that changes moves here with the date and the reason.

| Date | Principle | Was | Why it changed |
|---|---|---|---|
| 2026-10-01 | P-14 | *AI is optional, and consent is a person's* — the rule already said the workspace consents | Since [data-protection.md](../architecture/data-protection.md) §4.1 (2026-09-30) a workspace may make AI part of everybody's work on a named legal basis, where no person consents at all. The title now names what holds at every level: no operator or plan consents for a workspace, and no workspace overrides a person without a basis it names. Decided by the owner. |
