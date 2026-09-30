# Use cases — what Hubtask does, for whom, and how we know

A use case says, in plain language, **what a person can do with Hubtask and what must be true when
they do it**. It is written from the person's side: no table names, no routes, no framework. The
technical operations that implement it — `CreateWorkItem`, `POST /auth/sessions` — are in the
[domain model's catalogue](../architecture/domain-model.md#5-use-case-catalogue-application-layer);
this folder is the layer above them.

The *why* behind every use case is in [`../vision/`](../vision/README.md): the principles it serves,
the personas who act in it, the deployments it must work in.

## Where a use case lives

Filed by **bounded context**, under the same name as the code that implements it in
`core/application/service/`. A session working on sign-in finds the requirements in
`usecases/identity/` and the code in `service/identity/`.

| Folder | Prefix | Context | Code |
|---|---|---|---|
| [`identity/`](./identity/) | `UC-ID` | Accounts, signing in, members, roles, groups, tokens, sign-in providers, sign-in rules | `service/identity`, `service/access` |
| [`admin/`](./admin/) | `UC-INS` | The installation: operators, provisioning workspaces, defaults and locks, limits, plans | `service/admin`, `service/quota` |
| [`work/`](./work/) | `UC-WRK` | Hubs, collections, tasks, work packages, activities; buckets, labels, fields; due dates, reminders, recurrence; templates; views and search | `service/work` |
| [`jumble/`](./jumble/) | `UC-JUM` | The inbox for unstructured arrivals | `service/jumble` |
| [`automation/`](./automation/) | `UC-AUT` | Rules: trigger, condition, action | `service/automation` |
| [`integration/`](./integration/) | `UC-INT` | Webhooks, calendar feeds and CalDAV, OAuth apps, MCP, the API for scripts | `service/integration` |
| [`notification/`](./notification/) | `UC-NOT` | Mail and other channels, preferences | `service/notification` |
| [`media/`](./media/) | `UC-MED` | Covers and attachments | `service/media` |
| [`lifecycle/`](./lifecycle/) | `UC-LIF` | Trash, archive, retention rules, legal holds | `service/lifecycle` |
| [`backup/`](./backup/) | `UC-BAK` | Backup targets, schedules, restore, export | `service/backup` |
| [`privacy/`](./privacy/) | `UC-PRV` | Data subject requests, consent, erasure | `service/privacy` |
| [`audit/`](./audit/) | `UC-AUD` | The trail: reading, verifying, anchoring, exporting | `service/audit` |
| [`suggestion/`](./suggestion/) | `UC-AI` | AI providers and suggestions | `service/suggestion` |
| [`sync/`](./sync/) | `UC-SYN` | Working offline and merging | `service/sync` |
| [`importer/`](./importer/) | `UC-IMP` | Bringing work in from elsewhere | `service/importer` |

## What a use case looks like

One file per use case, named `UC-XX-nn-short-title.md`:

```markdown
---
id: UC-ID-04
title: Reset a forgotten password
context: identity
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-02, P-11, P-13]
state: partial
tasks: [SI-04, SI-15]
checked_by: [apps/webapp/e2e/signin.spec.ts]
---

# Reset a forgotten password

## Goal
What must be true for the person — the outcome, not the route.

## Story
The same thing as a short walk-through in everyday language. Where deployments differ, one
sentence each.

## How to check
1. Numbered, observable facts. A reviewer confirms each one without interpreting it.
2. …

## Where it ends
What is deliberately not asked for — so nobody builds too little, and nobody builds too much.

## Today
Only while the state is `specified` or `partial`: which checks are not met, and where that is
tracked (task, issue).
```

**The front matter**, every field required:

| Field | Meaning | Checked by the gate |
|---|---|---|
| `id` | `UC-<prefix>-<nn>`; never reused, never renumbered | unique, prefix matches the folder, file name starts with it |
| `title` | What the person does, as they would say it | present |
| `context` | The folder name | equals the folder |
| `actors` | Who acts in it | every entry is a persona in [personas.md](../vision/personas.md) |
| `deployments` | Where it must work | every entry is `D1`–`D7` from [deployments.md](../vision/deployments.md) |
| `serves` | The principles it serves | every entry is a principle in [principles.md](../vision/principles.md) |
| `state` | See below | one of the five |
| `tasks` | Backlog tasks that built or will build it, may be empty | — |
| `checked_by` | Tests, walks or evidence files that prove the checks | every path exists; required for `verified` |

**The five states:**

| State | Means |
|---|---|
| `specified` | Agreed; not built. *Today* says what is missing. |
| `partial` | Some checks hold, some do not. *Today* names the ones that do not, and where they are tracked. |
| `built` | Every check has code behind it; not every check has a test or a walk that proves it. |
| `verified` | Every check is proven — `checked_by` names the tests or the walk evidence. |
| `retired` | No longer applies; a line says why. The ID is not reused. |

The gate also refuses a `partial` or `specified` use case without a *Today* section, a use case
without *Goal*, *How to check* or *Where it ends*, and a `UC-…` cited anywhere in the repository
that does not exist.

## How use cases are used

1. **Cutting a milestone:** every task names the use cases it serves in a
   `**Use cases:** UC-ID-04 (1, 3), UC-ID-05` line, with the check numbers it is meant to make true.
   A milestone file that uses the line anywhere must use it in every task.
2. **Starting a task:** the session reads those use cases completely, before the code
   ([`CLAUDE.md`](../../CLAUDE.md), the loop, step 1).
3. **Leaving draft:** the pull request body carries a *Use cases* section — each named check, met or
   not, and how it was confirmed. A check that cannot be met is reported, not rewritten.
4. **Merging:** the same pull request moves `state:` and adds `checked_by:`.
5. **Reviewing:** `/usecase-check` (a project skill) walks a branch, a pull request or a whole
   milestone against its use cases and reports every check met, unmet or overstepped.

## Index

The index lists every use case; the gate refuses one that is missing or one that has no file.

| ID | Title | Context | State |
|---|---|---|---|
| [UC-ID-01](./identity/UC-ID-01-sign-in-with-a-password.md) | Sign in with my address and password | identity | built |
| [UC-ID-02](./identity/UC-ID-02-prove-it-is-me-with-a-second-factor.md) | Prove it is me with a second factor when I sign in | identity | partial |
| [UC-ID-03](./identity/UC-ID-03-set-up-and-keep-my-second-factor.md) | Set up my second factor and keep my recovery codes | identity | partial |
| [UC-ID-04](./identity/UC-ID-04-reset-a-forgotten-password.md) | Reset a forgotten password | identity | partial |
| [UC-ID-05](./identity/UC-ID-05-change-my-password.md) | Change my password | identity | partial |
| [UC-ID-06](./identity/UC-ID-06-see-and-end-my-sessions.md) | See where I am signed in and end a session | identity | partial |
| [UC-ID-07](./identity/UC-ID-07-accept-an-invitation.md) | Accept an invitation and set up my account | identity | partial |
| [UC-ID-08](./identity/UC-ID-08-sign-in-with-my-organisations-directory.md) | Sign in with my organisation's directory | identity | partial |
| [UC-ID-09](./identity/UC-ID-09-sign-in-with-google-or-microsoft.md) | Sign in with my Google or Microsoft account | identity | built |
| [UC-ID-10](./identity/UC-ID-10-connect-a-provider-to-my-existing-account.md) | Connect a sign-in provider to the account I already have | identity | specified |
| [UC-ID-11](./identity/UC-ID-11-set-up-a-sign-in-provider-for-the-workspace.md) | Set up a sign-in provider for our workspace | identity | partial |
| [UC-ID-12](./identity/UC-ID-12-set-the-workspaces-sign-in-rules-in-one-place.md) | Set how people in our workspace sign in, in one place | identity | partial |
| [UC-ID-13](./identity/UC-ID-13-require-a-new-password-from-everyone.md) | Require a new password from everyone after a breach | identity | built |
| [UC-ID-14](./identity/UC-ID-14-invite-people-with-a-role.md) | Invite people and give them a role | identity | partial |
| [UC-ID-15](./identity/UC-ID-15-limit-a-person-to-part-of-the-workspace.md) | Let a person see only part of the workspace | identity | built |
| [UC-ID-16](./identity/UC-ID-16-keep-a-hub-private.md) | Keep a hub private, even from another administrator | identity | specified |
| [UC-ID-17](./identity/UC-ID-17-give-a-machine-its-own-credential.md) | Give a script, an app or a platform its own credential | identity | partial |
| [UC-ID-18](./identity/UC-ID-18-show-the-legal-information-before-sign-in.md) | Show the legal information people are owed before they sign in | identity | partial |
| [UC-ID-19](./identity/UC-ID-19-agree-to-the-terms.md) | Agree to the terms, and again when they change | identity | specified |
| [UC-ID-20](./identity/UC-ID-20-give-someone-an-account-without-an-address.md) | Give a family member an account without their own mail address | identity | specified |
| [UC-INS-01](./admin/UC-INS-01-start-a-fresh-installation.md) | Start a fresh installation and create my workspace in the browser | admin | specified |
| [UC-INS-02](./admin/UC-INS-02-bring-an-installation-up-from-a-file.md) | Bring an installation up from a file, operators included | admin | partial |
| [UC-INS-03](./admin/UC-INS-03-get-back-into-a-locked-installation.md) | Get back into an installation nobody can run | admin | specified |
| [UC-INS-04](./admin/UC-INS-04-work-on-the-installation-for-an-hour.md) | Work on the installation for an hour after proving it is me | admin | partial |
| [UC-INS-05](./admin/UC-INS-05-appoint-and-remove-operators.md) | Appoint and remove the people who run the installation | admin | partial |
| [UC-INS-06](./admin/UC-INS-06-let-a-platform-run-the-installation.md) | Let a purchase platform provision workspaces with its own credential | admin | partial |
| [UC-INS-07](./admin/UC-INS-07-provision-a-workspace.md) | Create a workspace for a customer | admin | partial |
| [UC-INS-08](./admin/UC-INS-08-suspend-resume-and-delete-a-workspace.md) | Suspend, resume and delete a workspace | admin | partial |
| [UC-INS-09](./admin/UC-INS-09-set-defaults-and-locks-for-every-workspace.md) | Decide defaults and locks for every workspace | admin | partial |
| [UC-INS-10](./admin/UC-INS-10-set-limits-for-workspaces.md) | Set limits for workspaces, and see them before they hurt | admin | built |
| [UC-INS-11](./admin/UC-INS-11-offer-sign-in-providers-to-every-workspace.md) | Offer sign-in providers to every workspace | admin | partial |
| [UC-INS-12](./admin/UC-INS-12-see-the-installation-at-a-glance.md) | See the installation at a glance and read what happened | admin | partial |
| [UC-INS-13](./admin/UC-INS-13-put-workspaces-on-plans.md) | Put workspaces on plans with their own limits, features and locks | admin | specified |
| [UC-INS-14](./admin/UC-INS-14-tell-a-platform-what-happened.md) | Tell the purchase platform what happened, without it asking | admin | specified |
| [UC-INS-15](./admin/UC-INS-15-give-a-workspace-its-own-domain.md) | Give a workspace its own domain | admin | specified |
| [UC-INS-16](./admin/UC-INS-16-rotate-the-installations-keys.md) | Rotate the installation's keys and know every secret moved | admin | partial |
| [UC-AI-01](./suggestion/UC-AI-01-work-fully-without-ai.md) | Use all of Hubtask with AI switched off | suggestion | built |
| [UC-AI-02](./suggestion/UC-AI-02-connect-our-own-ai-provider.md) | Connect our workspace's own AI model | suggestion | partial |
| [UC-AI-03](./suggestion/UC-AI-03-consent-to-ai-processing.md) | Decide whether our content may be sent to a model, and take it back | suggestion | built |
| [UC-AI-04](./suggestion/UC-AI-04-get-suggestions-and-decide.md) | Get suggestions and decide on each one | suggestion | built |
| [UC-AI-05](./suggestion/UC-AI-05-use-the-model-our-provider-offers.md) | Use the AI model our provider offers, without setting anything up | suggestion | specified |
| [UC-AI-06](./suggestion/UC-AI-06-decide-which-ai-workspaces-may-use.md) | Decide as a provider which AI our workspaces may use | suggestion | specified |
| [UC-AI-07](./suggestion/UC-AI-07-keep-ai-within-a-budget.md) | Keep AI use within a budget | suggestion | partial |
