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
| [UC-ID-02](./identity/UC-ID-02-prove-it-is-me-with-a-second-factor.md) | Prove it is me with a second factor when I sign in | identity | built |
| [UC-ID-03](./identity/UC-ID-03-set-up-and-keep-my-second-factor.md) | Set up my second factor and keep my recovery codes | identity | built |
| [UC-ID-04](./identity/UC-ID-04-reset-a-forgotten-password.md) | Reset a forgotten password | identity | built |
| [UC-ID-05](./identity/UC-ID-05-change-my-password.md) | Change my password | identity | built |
| [UC-ID-06](./identity/UC-ID-06-see-and-end-my-sessions.md) | See where I am signed in and end a session | identity | built |
| [UC-ID-07](./identity/UC-ID-07-accept-an-invitation.md) | Accept an invitation and set up my account | identity | partial |
| [UC-ID-08](./identity/UC-ID-08-sign-in-with-my-organisations-directory.md) | Sign in with my organisation's directory | identity | partial |
| [UC-ID-09](./identity/UC-ID-09-sign-in-with-google-or-microsoft.md) | Sign in with my Google or Microsoft account | identity | built |
| [UC-ID-10](./identity/UC-ID-10-connect-a-provider-to-my-existing-account.md) | Connect a sign-in provider to the account I already have | identity | built |
| [UC-ID-11](./identity/UC-ID-11-set-up-a-sign-in-provider-for-the-workspace.md) | Set up a sign-in provider for our workspace | identity | partial |
| [UC-ID-12](./identity/UC-ID-12-set-the-workspaces-sign-in-rules-in-one-place.md) | Set how people in our workspace sign in, in one place | identity | built |
| [UC-ID-13](./identity/UC-ID-13-require-a-new-password-from-everyone.md) | Require a new password from everyone after a breach | identity | built |
| [UC-ID-14](./identity/UC-ID-14-invite-people-with-a-role.md) | Invite people and give them a role | identity | partial |
| [UC-ID-15](./identity/UC-ID-15-limit-a-person-to-part-of-the-workspace.md) | Let a person see only part of the workspace | identity | partial |
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
| [UC-INS-17](./admin/UC-INS-17-run-hubtask-on-my-own-hardware.md) | Run Hubtask on the hardware I have | admin | built |
| [UC-WRK-01](./work/UC-WRK-01-organise-my-work-into-hubs-and-collections.md) | Organise my work into hubs and collections | work | partial |
| [UC-WRK-02](./work/UC-WRK-02-write-down-a-task.md) | Write down a task | work | built |
| [UC-WRK-03](./work/UC-WRK-03-break-a-task-down-into-smaller-steps.md) | Break a task down into smaller steps | work | built |
| [UC-WRK-04](./work/UC-WRK-04-tick-work-off-and-take-it-back.md) | Tick work off, and take it back | work | built |
| [UC-WRK-05](./work/UC-WRK-05-run-a-collection-as-a-board.md) | Run a collection as a board | work | partial |
| [UC-WRK-06](./work/UC-WRK-06-put-work-in-order-and-move-it-where-it-belongs.md) | Put work in order and move it where it belongs | work | built |
| [UC-WRK-07](./work/UC-WRK-07-duplicate-a-task.md) | Duplicate a task | work | partial |
| [UC-WRK-08](./work/UC-WRK-08-change-many-entries-at-once.md) | Change many entries at once | work | built |
| [UC-WRK-09](./work/UC-WRK-09-make-work-recognisable-at-a-glance.md) | Make work recognisable at a glance | work | partial |
| [UC-WRK-10](./work/UC-WRK-10-hand-work-to-a-person.md) | Hand work to a person | work | built |
| [UC-WRK-11](./work/UC-WRK-11-let-a-collection-hand-out-new-work.md) | Let a collection hand out new work automatically | work | built |
| [UC-WRK-12](./work/UC-WRK-12-record-the-facts-a-team-tracks.md) | Record the facts a team tracks, with custom fields | work | built |
| [UC-WRK-13](./work/UC-WRK-13-give-work-a-due-date.md) | Give work a due date | work | built |
| [UC-WRK-14](./work/UC-WRK-14-be-reminded-before-something-is-due.md) | Be reminded before something is due | work | partial |
| [UC-WRK-15](./work/UC-WRK-15-repeat-a-task.md) | Repeat a task | work | partial |
| [UC-WRK-16](./work/UC-WRK-16-start-a-routine-from-a-template.md) | Start a routine from a template | work | built |
| [UC-WRK-17](./work/UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md) | Filter, sort and save a view of my work | work | partial |
| [UC-WRK-18](./work/UC-WRK-18-find-an-entry-again.md) | Find an entry again by searching | work | built |
| [UC-WRK-19](./work/UC-WRK-19-discuss-an-entry-and-see-what-happened-to-it.md) | Discuss an entry and see what happened to it | work | partial |
| [UC-WRK-20](./work/UC-WRK-20-see-what-is-on-me-when-i-arrive.md) | See what is on me when I arrive | work | partial |
| [UC-WRK-21](./work/UC-WRK-21-share-a-hub-a-collection-or-one-entry.md) | Share a hub, a collection or one entry with somebody | work | partial |
| [UC-WRK-22](./work/UC-WRK-22-work-within-what-my-role-allows.md) | Work within what my role allows | work | partial |
| [UC-JUM-01](./jumble/UC-JUM-01-jot-something-down-before-deciding-what-it-is.md) | Jot something down before deciding what it is | jumble | partial |
| [UC-JUM-02](./jumble/UC-JUM-02-give-the-jumble-an-address-and-replace-it.md) | Give the jumble an address, and replace it when it leaks | jumble | partial |
| [UC-JUM-03](./jumble/UC-JUM-03-forward-mail-into-the-jumble.md) | Forward mail into the jumble | jumble | built |
| [UC-JUM-04](./jumble/UC-JUM-04-let-another-system-post-into-the-jumble.md) | Let another system post into the jumble | jumble | built |
| [UC-JUM-05](./jumble/UC-JUM-05-turn-a-jumble-entry-into-work.md) | Turn a jumble entry into work | jumble | built |
| [UC-JUM-06](./jumble/UC-JUM-06-dismiss-what-does-not-need-doing.md) | Dismiss what does not need doing | jumble | built |
| [UC-JUM-07](./jumble/UC-JUM-07-ask-ai-what-an-entry-should-become.md) | Ask AI what a jumble entry should become | jumble | built |
| [UC-JUM-08](./jumble/UC-JUM-08-sort-arrivals-automatically-with-a-rule.md) | Sort arrivals automatically with a rule | jumble | built |
| [UC-AUT-01](./automation/UC-AUT-01-write-a-rule-and-switch-it-on.md) | Write a rule and switch it on | automation | built |
| [UC-AUT-02](./automation/UC-AUT-02-try-a-rule-before-it-acts.md) | Try a rule before it acts | automation | built |
| [UC-AUT-03](./automation/UC-AUT-03-run-a-rule-on-demand.md) | Run a rule when I press a button | automation | built |
| [UC-AUT-04](./automation/UC-AUT-04-run-a-rule-on-a-schedule-or-before-a-date.md) | Run a rule on a schedule, or a set time before or after a date | automation | built |
| [UC-AUT-05](./automation/UC-AUT-05-start-a-rule-from-another-system.md) | Start a rule from another system | automation | built |
| [UC-AUT-06](./automation/UC-AUT-06-see-what-my-rules-did-and-replay-a-failed-run.md) | See what my rules did, and replay a run that failed | automation | built |
| [UC-AUT-07](./automation/UC-AUT-07-keep-rules-from-running-away.md) | Keep rules from running away | automation | built |
| [UC-AUT-08](./automation/UC-AUT-08-find-rules-that-need-attention-before-they-fail.md) | Find rules that need attention before they fail | automation | built |
| [UC-AUT-09](./automation/UC-AUT-09-have-a-rule-tell-people.md) | Have a rule tell people something | automation | specified |
| [UC-INT-01](./integration/UC-INT-01-receive-workspace-events-on-my-server.md) | Receive the workspace's events on my own server | integration | partial |
| [UC-INT-02](./integration/UC-INT-02-recover-deliveries-my-server-missed.md) | Recover the deliveries my server missed | integration | built |
| [UC-INT-03](./integration/UC-INT-03-replace-a-webhook-secret-without-dropping-deliveries.md) | Replace a webhook secret without dropping deliveries | integration | built |
| [UC-INT-04](./integration/UC-INT-04-poll-for-events-instead-of-receiving-them.md) | Poll for events instead of receiving them | integration | built |
| [UC-INT-05](./integration/UC-INT-05-call-an-outside-service-from-a-rule.md) | Call an outside service from a rule | integration | built |
| [UC-INT-06](./integration/UC-INT-06-see-my-tasks-in-my-calendar-app.md) | See my tasks in my calendar app | integration | built |
| [UC-INT-07](./integration/UC-INT-07-tick-off-and-reschedule-tasks-from-my-calendar-app.md) | Tick off and reschedule tasks from my calendar app | integration | built |
| [UC-INT-08](./integration/UC-INT-08-let-a-third-party-app-act-for-me.md) | Let a third-party app act for me, and take it back | integration | built |
| [UC-INT-09](./integration/UC-INT-09-let-an-ai-agent-work-in-my-workspace.md) | Let an AI agent work in my workspace | integration | built |
| [UC-INT-10](./integration/UC-INT-10-script-my-workspace-with-a-token.md) | Script my workspace with hubctl or the API | integration | built |
| [UC-INT-11](./integration/UC-INT-11-connect-hubtask-to-n8n-or-zapier.md) | Connect Hubtask to n8n or Zapier | integration | partial |
| [UC-NOT-01](./notification/UC-NOT-01-hear-by-mail-when-work-lands-on-me.md) | Hear by mail when work lands on me | notification | built |
| [UC-NOT-02](./notification/UC-NOT-02-get-my-reminders-by-mail.md) | Get my reminders by mail | notification | built |
| [UC-NOT-03](./notification/UC-NOT-03-choose-which-mails-i-get.md) | Choose which mails I get | notification | built |
| [UC-NOT-04](./notification/UC-NOT-04-hear-when-something-i-set-up-stops-working.md) | Hear when something I set up stops working | notification | built |
| [UC-NOT-05](./notification/UC-NOT-05-connect-the-installation-to-a-mail-server.md) | Connect the installation to a mail server | notification | built |
| [UC-NOT-06](./notification/UC-NOT-06-be-told-somewhere-other-than-mail.md) | Be told somewhere other than my mailbox | notification | specified |
| [UC-MED-01](./media/UC-MED-01-attach-a-file-to-an-entry.md) | Attach a file to an entry | media | built |
| [UC-MED-02](./media/UC-MED-02-give-an-entry-a-cover.md) | Give an entry a cover | media | built |
| [UC-MED-03](./media/UC-MED-03-remove-files-nobody-needs.md) | Remove files nobody needs any more | media | built |
| [UC-MED-04](./media/UC-MED-04-set-how-much-may-be-uploaded.md) | Set how much may be uploaded | media | built |
| [UC-LIF-01](./lifecycle/UC-LIF-01-get-something-back-from-the-trash.md) | Get something I deleted back from the trash | lifecycle | partial |
| [UC-LIF-02](./lifecycle/UC-LIF-02-remove-something-for-good.md) | Remove something for good | lifecycle | partial |
| [UC-LIF-03](./lifecycle/UC-LIF-03-archive-finished-work.md) | Archive finished work and bring it back when needed | lifecycle | partial |
| [UC-LIF-04](./lifecycle/UC-LIF-04-set-a-retention-rule.md) | Set a retention rule and see what it would do before it acts | lifecycle | partial |
| [UC-LIF-05](./lifecycle/UC-LIF-05-be-warned-and-keep-my-work.md) | Be warned before a rule removes my work, and keep it | lifecycle | partial |
| [UC-LIF-06](./lifecycle/UC-LIF-06-place-a-legal-hold.md) | Place a legal hold so that nothing is destroyed | lifecycle | partial |
| [UC-BAK-01](./backup/UC-BAK-01-choose-where-backups-go.md) | Choose where the workspace's backups go | backup | partial |
| [UC-BAK-02](./backup/UC-BAK-02-back-up-on-a-schedule.md) | Back up on a schedule and keep the right generations | backup | partial |
| [UC-BAK-03](./backup/UC-BAK-03-back-up-now-and-check-it-opens.md) | Back up now and check that the backup opens | backup | built |
| [UC-BAK-04](./backup/UC-BAK-04-keep-backups-readable-only-by-us.md) | Keep backups unreadable to the target, and readable to us after a loss | backup | partial |
| [UC-BAK-05](./backup/UC-BAK-05-reset-the-workspace-to-a-backup.md) | Reset the whole workspace to an earlier backup | backup | built |
| [UC-BAK-06](./backup/UC-BAK-06-get-one-thing-back-from-a-backup.md) | Get one lost collection or task back from a backup | backup | partial |
| [UC-BAK-07](./backup/UC-BAK-07-take-the-workspace-to-another-installation.md) | Take the whole workspace to another installation | backup | partial |
| [UC-BAK-08](./backup/UC-BAK-08-recover-the-whole-installation.md) | Recover the whole installation after losing the server | backup | partial |
| [UC-PRV-01](./privacy/UC-PRV-01-answer-a-data-subject-request-in-time.md) | Record a data subject request and answer it in time | privacy | partial |
| [UC-PRV-02](./privacy/UC-PRV-02-give-a-person-a-copy-of-their-data.md) | Give a person a copy of their data | privacy | built |
| [UC-PRV-03](./privacy/UC-PRV-03-erase-a-person-on-request.md) | Erase a person on request | privacy | partial |
| [UC-PRV-04](./privacy/UC-PRV-04-restrict-the-processing-of-a-persons-data.md) | Restrict the processing of a person's data | privacy | partial |
| [UC-PRV-05](./privacy/UC-PRV-05-withdraw-consent-to-optional-processing.md) | Object to an optional use of my data | privacy | partial |
| [UC-PRV-06](./privacy/UC-PRV-06-answer-a-request-across-the-whole-installation.md) | Answer a request across every workspace of the installation | privacy | partial |
| [UC-PRV-07](./privacy/UC-PRV-07-correct-what-we-hold-about-a-person.md) | Correct what the workspace holds about a person | privacy | built |
| [UC-PRV-08](./privacy/UC-PRV-08-know-what-the-installation-stores-about-people.md) | Know what the installation stores about people, and how each piece is deleted | privacy | built |
| [UC-AUD-01](./audit/UC-AUD-01-look-up-what-happened-in-the-workspace.md) | Look up what happened in the workspace | audit | built |
| [UC-AUD-02](./audit/UC-AUD-02-prove-the-trail-is-intact.md) | Prove the trail has not been changed | audit | verified |
| [UC-AUD-03](./audit/UC-AUD-03-anchor-the-trail-outside-the-database.md) | Anchor the trail outside the database | audit | partial |
| [UC-AUD-04](./audit/UC-AUD-04-hand-the-trail-to-an-auditor.md) | Hand a period of the trail to somebody outside | audit | built |
| [UC-AUD-05](./audit/UC-AUD-05-check-a-workspace-without-seeing-its-content.md) | Check a workspace without seeing its content | audit | built |
| [UC-AUD-06](./audit/UC-AUD-06-see-what-the-workspace-recorded-about-me.md) | See what the workspace recorded about me | audit | partial |
| [UC-AUD-07](./audit/UC-AUD-07-keep-the-trail-for-its-period-and-no-longer.md) | Keep the trail for its period, and no longer | audit | partial |
| [UC-AI-01](./suggestion/UC-AI-01-work-fully-without-ai.md) | Use all of Hubtask with AI switched off | suggestion | built |
| [UC-AI-02](./suggestion/UC-AI-02-connect-our-own-ai-provider.md) | Connect our workspace's own AI model | suggestion | partial |
| [UC-AI-03](./suggestion/UC-AI-03-consent-to-ai-processing.md) | Decide whether our content may be sent to a model, and take it back | suggestion | built |
| [UC-AI-04](./suggestion/UC-AI-04-get-suggestions-and-decide.md) | Get suggestions and decide on each one | suggestion | built |
| [UC-AI-05](./suggestion/UC-AI-05-use-the-model-our-provider-offers.md) | Use the AI model our provider offers, without setting anything up | suggestion | specified |
| [UC-AI-06](./suggestion/UC-AI-06-decide-which-ai-workspaces-may-use.md) | Decide as a provider which AI our workspaces may use | suggestion | specified |
| [UC-AI-07](./suggestion/UC-AI-07-keep-ai-within-a-budget.md) | Keep AI use within a budget | suggestion | partial |
| [UC-SYN-01](./sync/UC-SYN-01-keep-working-when-the-connection-drops.md) | Keep working when the connection drops | sync | partial |
| [UC-SYN-02](./sync/UC-SYN-02-edit-on-two-devices-without-losing-either-change.md) | Edit on two devices without losing either change | sync | built |
| [UC-SYN-03](./sync/UC-SYN-03-see-and-forget-my-devices.md) | See and forget my devices | sync | built |
| [UC-SYN-04](./sync/UC-SYN-04-lose-access-and-have-the-copy-follow.md) | Lose access, and have my device's copy follow | sync | built |
| [UC-SYN-05](./sync/UC-SYN-05-come-back-after-a-long-time-away.md) | Come back after a long time away, or after a restore | sync | built |
| [UC-SYN-06](./sync/UC-SYN-06-prove-my-own-client-follows-the-sync-contract.md) | Prove that my own client follows the sync contract | sync | built |
| [UC-IMP-01](./importer/UC-IMP-01-import-entries-from-a-spreadsheet.md) | Import entries from a spreadsheet | importer | built |
| [UC-IMP-02](./importer/UC-IMP-02-move-over-from-another-task-app.md) | Move over from Trello, Google Tasks or Microsoft To Do | importer | built |
| [UC-IMP-03](./importer/UC-IMP-03-see-what-an-import-did.md) | See what an import did | importer | built |
