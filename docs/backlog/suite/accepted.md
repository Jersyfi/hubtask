# The owner's acceptance — 2026-10-28

**The owner's word (2026-10-28, translated):** "Regarding the decisions — you can take them out,
because I am working out the concept together with you and therefore also accept it." Together
with: "I want this session to implement the complete concept at the end and to steer its
subagents for it."

Recorded as: the concept now in `docs/backlog/suite/` (suite, configuration model, products,
foundation) is accepted as a whole and is to be built in concept mode (`AGENTS.md`, "Working with
the owner"). Each question below ([`decisions.md`](./decisions.md)) is answered by the option the concept
proposes.

| # | Decided as proposed |
|---|---|
| D-01 | Hubtask becomes a work suite; vision sentence, promise 6, P-17 *Depth is switched on and starts from a default that works* |
| D-02 | In a workflow collection, completion follows the status category |
| D-03 | Modules per workspace and hub; plans offer and lock modules; without a plan everything is available |
| D-04 | A Yjs-compatible CRDT for live editing (dependencies listed in `foundation/README.md` §3) |
| D-05 | Custom roles from a permission catalogue; nobody grants a permission they do not hold |
| D-06 | New non-goals: chat, meetings, invoicing, in-process plugins, spreadsheet engine, sales CRM |
| D-07 | Types without completion (records) exist — realised as the *record* behaviour of F-1 |
| D-08 | Dependencies become planned (`UC-WRK-03` *Where it ends* corrected) |
| D-09 | Personas `PE-planner`, `PE-requester`, `PE-handler` added |
| D-10 | Migration from issue trackers and wikis (configuration, history, attachments) in the first wave |
| D-11 | P-18 *One feature, one building block, every door* |
| D-12 | P-19 *Everything is an object you can point at* |
| D-13 | A type's form binds every door, capture and the API included |
| D-14 | One settings catalogue for every level |
| D-15 | The configuration document with plan and apply for installation, workspace, hub |
| D-16 | An agent's plan waits for a person's approval by default |
| D-17 | Templates are copied with provenance (`template@version`) and upgraded by a three-way comparison — the owner's copy semantics of #1619–#1622 kept |
| D-18 | The administration regrouped into six groups |

**Still the owner's at the moment it arises** (`AGENTS.md`, "What you do not decide yourself"),
because no concept can settle them in advance: a lifecycle policy that would irrecoverably delete
records, the retention of agent run traces, a change to the security gates, a dependency that fails
the licence gate, and merging each pull request.

**Cyber Resilience Act role:** the project stays without a paid edition (ADR-0080), so it acts as an
open-source steward; providers who sell it are manufacturers themselves and receive from the
project what they need (SBOM, advisories, support period, CVD) — F-13.

## The owner's answers — 2026-10-30

The workers collected what they could not decide themselves. The owner accepted every proposal
below as a whole ("Accept all", 2026-10-30). Each item is built in the step that owns it; the
pull request names them.

**A. New dependencies.** SAML (XML signature verification) and an LDAP client: yes, MIT/BSD
libraries that pass the licence gate. A spreadsheet library for XLSX export: yes. A PDF renderer:
not now — reports by mail send CSV and a link. An external search index: no — PostgreSQL search
behind the existing search port. A Go CRDT library: no — clients merge (Yjs); the server keeps the
update log. `@marcbachmann/cel-js` 8.0.0 (MIT) for the browser evaluator: confirmed, its licence
joins `THIRD-PARTY-LICENSES.md`.

**B. Irrecoverable deletion and retention.**
- A hub's or workspace's purge erases the live documents of its pages and boards.
- Page versions: named versions and the newest version of each day are kept; others go after
  90 days. Trashed pages are purged after the trash period (30 days) with their live documents.
- A live document's update history: 30 days (`DOCUMENT_HISTORY`); the document itself stays.
- Agents: the action ledger (`agent_action`) 13 months; held requests, jobs and run traces 90 days.
- Erasing a person pseudonymises approved time records, capacities, absences and own cost rates
  (the sums are kept); nothing of them is deleted.
- Requesters without an account (mail, public forms): a data-catalogue entry, deletion 24 months
  after the last contact, and an erasure path.
- CIs and connectors are switched off, never deleted; baselines go with their plan (as built).

**C. Use cases.** A use case file for the configuration document (hub scope, sandbox, reach,
previews); a check naming the aggregation in UC-WRK-17; UC-ID-26 check 5 names READ at the place;
UC-DOC-11, UC-DOC-12, UC-AGT-07 and UC-AST-01 check 9 as written; UC-AGT-02's correction.

**D. Product.**
- Stable keys for hubs and collections, so rules and saved views become sections of the
  configuration document.
- The installation shows plan and history; applying stays the instance file's (ADR-0070 §5).
- An *organisation* for the portal: a group of requesters assigned by email domain.
- *View as*: a read-only, audited, online-only view of the person's screens.
- `agents.registered_only`: a workspace setting refusing unregistered agents on MCP, off by default.
- The remaining hand-written REST operations and `hubctl` commands move to the generated doors
  before the suite's pull request.
- SHOULD connectors (Snipe-IT, Jamf, cloud and others) come in the following release.
- Capacity runs: 10⁶ entries on the owner's workstation, 10⁷ on the integration server.

**E. The workers' smaller choices** recorded in the pull request notes are accepted; the owner may
still overrule each on the pull request.

## The owner's second answers — 2026-10-30

The completion round raised eleven further questions; the owner accepted every proposal
("Accept all", 2026-10-30).

1. `POST /containers/{containerId}:summarize` gets a body without `apply` (a summary has nowhere to
   be applied); the field leaves that operation's request schema.
2. `module` on `PUT /admin/settings` and `enabled` on `POST /backup-schedules` are declared by their
   use cases and take effect; both doors then move to the generated mapping.
3. Embedding in openDesk: `HUBTASK_UI_FRAME_ANCESTORS`, a configured list of origins, empty by
   default, on the user interface's CSP only, never the API.
4. Imports and forms: the one ingestion path stays; only an import into an existing collection's
   types is held to their form, and the result is reported per entry.
5. UC-GOL-04 gets check 12: a widget's summary sums the time recorded on its entries and counts the
   live pages of its bound spaces, as the reader may see them.
6. Whiteboard history: a retention kind `WHITEBOARD_HISTORY`, keeping everything by default; a
   workspace may age out unnamed versions and resolved threads.
7. An installation's business calendars: a `business_calendars` block in the instance file, which a
   workspace calendar of the same key falls back to.
8. Billing rates per client or project: an optional rate with validity dates on a hub or
   collection, read only with `MANAGE_COSTS`, and a billable-value column in the export; UC-TIM-08
   gets the check for it.
9. Flow measures for entries from before the status history: none; no backfill.
10. Ticket routing by offering (G11): the triage takes the offering a ticket names, else the nearest
    one a CI it names affects (none if two are equally near), and proposes `AssignWorkItem` to the
    member of that offering's support group with the fewest open entries in the hub, held for a
    person to approve, only for a ticket nobody holds. UC-AGT-07 gets check 9, and its *Where it
    ends* says a group itself holds no ticket and skills are not weighed.
11. Searching inside attached files: not now; search stays by file name (no text extractor).

## The owner's third answers — 2026-10-31

The decisions round raised ten further questions; the owner accepted every proposal
("Accept all", 2026-10-31).

1. Board versions (`whiteboard_version`) are thinned like page versions: named versions and the
   newest of each day are kept, the others go after 90 days.
2. On a requester's erasure, and on `REQUESTER` expiry, the intake-written first line
   "From: name <address>" of their entries' notes and comments is removed through the ordinary
   update and comment use cases.
3. UC-PRV-01 (and the mail intake's use case) get a check: a case naming only an address erases the
   requester record, the receipts and the intake of that address; a workspace hold keeps it.
4. goxmldsig (Apache-2.0) is accepted as the XML signature library for SAML.
5. The changed *Where it ends* of UC-ID-11 and UC-ID-28 (SAML is UC-ID-30) is confirmed.
6. LDAP synchronisation (`ConfigureLdapDirectory`, `SyncLdapDirectory`) belongs to the
   `OWN_SIGN_IN_PROVIDERS` plan entitlement (UC-INS-13 checks 4 and 5, multi-tenancy.md §4.2).
7. `hubctl` flags renamed to the contract's names keep their old names as aliases, so existing
   scripts do not break.
8. `POST /items:export` gets its own query schema without `page`, `count`, `group_by` and `expand`.
9. The 10⁷ capacity run: the owner provides the host; the command is in `test/load/README`.
   The owner named it the same day: the owner's workstation (14 cores, 36 GB, the strongest
   machine available), which also runs the 10⁶ run.
10. The retention sweeps of kinds with a default period (`DOCUMENT_HISTORY`, `PAGE_VERSION`,
    `AGENT_*`, `REQUESTER`, …) read the workspace's TENANT-scope rule, as `WHITEBOARD_HISTORY` does,
    keeping each kind's lower bound.

## The owner's word on the performance tests — 2026-10-31

- Every error seen during the performance runs — server log entries at warn or error,
  PostgreSQL log errors, failed requests, browser console errors — is collected, deduplicated and
  afterwards fixed.
- Several hours of load on the workstation is too much. First a sensible plan is worked out for
  testing as well as possible, with how long it really takes (measured by short probes), in
  `test/load/PLAN.md`; it is shown to the owner before the long runs start.

## The owner's word for the remaining runs — 2026-10-31

For the rest of the coverage runs and every step after them: stick to the plan; do not
hallucinate; never invent or fake information. A worker reports only what it checked in the code
or measured, says plainly what it did not run or could not verify, and records a gap as open
rather than closed when it is not proven by a test or a measurement.

## The owner's word on the user experience — 2026-10-31

The owner walked a snapshot of the suite (391f55fae) and found it hard to use: not friendly, no
clear place to find things, hard to understand how each product works, the layout not fitting how
a person or an administrator thinks, and many bugs that stopped clicking through. A person new to
Hubtask would be overwhelmed by what must be set up before a product can be used.

The bar for the role reviews and the UI/UX run:

- Not a plain, conventional interface: Hubtask sets a standard — modern, lively, quick to
  understand, pleasant to use. Inert, dull or complex screens are defects.
- The same patterns in every product (navigation, creating, finding, configuring, empty states,
  details, settings), so that people and administrators recognise them everywhere and want to
  work with it. The leading work-management suites are the reference for this consistency; their
  limited flexibility is not — Hubtask keeps the customer's freedom of use.
- Starting is easy: a product works with sensible defaults and templates on its first use, and a
  newcomer is guided rather than handed every setting at once (progressive disclosure).
- Every bug found by clicking through is fixed.
- A workspace pre-filled with realistic data in every product, so the owner can see what exists,
  and a design direction with mockups written into the repository and then built. The owner
  said the same day that the direction is not presented for approval but built as planned.

## The owner's fourth answers — 2026-10-31

The coverage runs raised further owner items. The owner asked the orchestrating session for
sensible proposals and accepted them (2026-10-31). Each takes the option that keeps data safest
and changes least for people who do not use it.

1. **Leaving a shared installation.** A workspace owner can always download the workspace's full
   encrypted archive, streamed and never stored on a target, independent of backup targets:
   once per 24 hours, bounded by the workspace's quota, audited. UC-BAK-07's *Where it ends*
   changes by this decision.
2. **Editor blocks.** New MIT dependencies, each through the licence gate and loaded lazily in its
   own chunk: `@tiptap/extension-table` and `@tiptap/extension-details` at the TipTap core's
   version, `katex`, `mermaid`, `lowlight`, and `emojibase-data` for an emoji picker. Every block
   round-trips through Markdown; the wiki import maps tables and details onto them.
3. **Anonymous public intake form.** A catalogue switch, off by default; a rate limit per address
   and per network origin; a proof-of-work challenge (no CAPTCHA); the requester recorded as in B.
4. **Upload scanning.** An optional scan port: a no-op by default, a clamd adapter configured in
   the instance file and dialled through a guarded dialer with the same allowlist and timeouts as
   `GuardedClient`. A file is held back until scanned; an alert fires when files stay unscanned.
   UC-MED-01's *Where it ends* becomes "scanning only where an operator configures one".
5. **ITIL auto-close.** A relative-date trigger may name a business calendar; the ITIL template
   then closes after 5 business days, as templates.md says.
6. **Notifying a role.** An automation action `NOTIFY_ROLE` (a role key; the role's holders along
   the entry's path; category SLA, mutable like any other category).
7. **Merged portal requests.** A requester whose request was merged reads the surviving one
   through the `duplicates` link and is told of its replies (UC-SVC-05 by this decision).
8. **Nextcloud.** Hubtask serves the two read endpoints a Nextcloud dashboard widget and unified
   search provider need, through the registry with the reader's own permissions. The PHP app goes
   into its own repository after the suite; this repository gets no new language.
9. **Public roadmap link.** A share token per portfolio, read-only, revocable, audited, opened
   without an account, behind a workspace setting off by default (as UC-DOC-08); a check in
   UC-GOL-03.
10. **Publishing the n8n node and the Zapier app.** A release job that publishes on a version tag
    when `NPM_TOKEN` and `ZAPIER_DEPLOY_KEY` exist and is skipped otherwise. The owner creates the
    accounts and secrets; UC-INT-11 check 4 is met once the first listing is live.
11. **`hubctl jumble submit --body <text>`.** The old text form is refused, not guessed; the
    release notes name `--raw-body`.
12. **Embeds.** `HUBTASK_UI_FRAME_SOURCES`, complete https origins, empty by default, on the user
    interface's policy only; only those origins are framed (sandboxed), others stay a link card.
13. **The performance plan** (`test/load/PLAN.md`) as written, with R3 (the browser walk counting
    console errors) added: P2 at 10⁶ (about 14 minutes), R1 at 10⁷ (about 50 to 60 minutes), R3
    (about 40 minutes plus the walk), R2 only after a fix.

## The owner's fifth answers — 2026-11-01

Owner items from the third coverage round and the role reviews. The owner asked the orchestrating
session for sensible proposals and accepted them (2026-10-31); each keeps data safest and changes
least.

1. **The owner's workspace download** stays in the documented, openable export format, protected
   by TLS in transit (UC-BAK-07 check 2). An optional passphrase encrypts it (Argon2id, from the
   `x/crypto` already in `go.mod`, the manifest's KDF fields), asked again at restore.
2. **The browser's replica in a very large workspace.** Above 100 000 entries the browser
   replicates the hubs the person is a member of, opened in the last 30 days or pinned, up to
   about 100 000 entries, and reads the rest online; the app shows which hubs are available
   offline. The shells keep the whole replica. offline-sync.md §1 changes by this decision, and
   the largest workspace's snapshot joins `test/load/PLAN.md`.
3. **A figure under steady writes** shows the last kept count for up to 60 seconds after a
   change, marked "as of <time>", while the late count runs (UC-WRK-29 by this decision).
4. **Use case changes made under concept mode** are confirmed: UC-SVC-03 check 9, UC-SVC-01
   check 5's wording, UC-JUM-11 check 10 and its *Where it ends* on IDLE.
5. **A self-hoster locked out without mail.** A machine-side command beside `hubtask operator add`,
   `hubtask account reset-link --workspace <short> --email <addr>`, prints a short-lived one-time
   reset link (hash stored, journalled as the machine's); the no-mail sign-in text names it.
6. **Lost second factor and recovery codes.** An administrator with `MANAGE_MEMBERS`, after their
   own step-up and within the role ceiling, can reset a person's second factor: audited, the
   person is mailed, their sessions end, and enrolment is forced at the next sign-in where the
   workspace requires a factor. An owner's factor is reset only by another owner. No sign-in ever
   skips a factor (P-02); identity.md changes by this decision.

## The owner's sixth answers — 2026-11-05

Owner items from the final runs (coverage, role reviews, walks, functional tests and the leftover
rounds), merged and checked against the code at `cba0b7278`; items already answered above are not
repeated. The owner asked the orchestrating session for sensible proposals and accepted them
(2026-10-31); each keeps data safest and changes least.

1. **Mailboxes as a quota.** The fixed ceiling of five mailboxes (Mailbox.go) becomes the quota
   `mailboxes`, resolved as every quota (multi-tenancy.md §4), 5 by default in both modes; past it
   `422 capacity.mailboxes`; connected mailboxes keep being read. Where it goes: UC-JUM-11 *Where
   it ends*, multi-tenancy.md §4, the quota enum in `api/openapi.yaml` (additive).
2. **A fresh installation's first backup.** Hubtask still creates no target or schedule by itself.
   The administration overview's first steps show "Backups: none yet — choose where they go", and
   `/meta/health` warns while a workspace has no backup target or no backup in 7 days. Where it
   goes: a check in UC-BAK-01, observability-reliability.md's health table.
3. **Who reads a request catalogue.** Unchanged: only with REQUEST (UC-SVC-05 check 1); the
   explanation shown to a team view stays. Where it goes: nothing.
4. **Agents' held acts and Approvals.** Two places stay; `/approvals` links to *Profile → Agents →
   Requests* when the agents module is on. Where it goes: a UI step only.
5. **Erasing a person whom switched-off integrations name.** The erasure moves `run_as` and
   `owner_id` of switched-off connectors, code hooks and retired agents to the workspace owner,
   audited, and they stay off; then foreign keys added NOT VALID and validated in a later
   migration. Active ones keep blocking (`privacy.erasure_blocked_by_integration`). Where it goes:
   the erasure section of the privacy subject document, a check in UC-PRV-01, a migration pair.
6. **Uploads held without a scanner.** Held files are never released unscanned, by any switch or
   command; the operator configures a scanner again or an administrator trashes the file; the
   critical warning `media.uploads_held_without_scanner` stays. Where it goes: security.md §7.
7. **Copying a restricted page.** The copy keeps the source's restriction, which the copier may
   lift; versions, comments, shares and public links are still not copied. Where it goes:
   UC-DOC-01 check 8, the knowledge copy service.
8. **Withheld minutes in an aggregated time report.** When the withheld groups together cover fewer
   than three people, their minutes leave both the total and `withheld_minutes`; `withheld_groups`
   still counts them and an additive `withheld_minutes_hidden: true` says so. Where it goes:
   UC-TIM-04 check 3, the report schema (additive), the time subject document.
9. **View as after the viewer loses their rights.** The view also ends at authentication when the
   viewer no longer holds MANAGE_MEMBERS or the viewed person's role is now above the viewer's.
   Where it goes: UC-ID-29 check 3, identity.md's view-as section.
10. **Use case wording for built behaviour.** UC-TIM-03 check 3 names `time.correction_below_zero`;
    UC-ID-18 gets a check that a workspace's support contact and documentation show in *Help*,
    never in the sign-in footer. Where it goes: those use cases, `checked_by:`.
11. **Page text on a requester's erasure or expiry.** Third answers item 2 extends to pages: only
    the intake-written "From:" line is removed from the live page, its draft, every version and
    the live document (rebuilt from the stripped body); a workspace hold keeps everything. Where
    it goes: the erasure section, a check in UC-PRV-01.
12. **Request rate and live connections per plan.** Stay per-process configuration in this
    release (multi-tenancy.md §4); plan-resolved rate, connection, `stored_bytes` and object-count
    quotas come in a following release as additive quota names. Where it goes: nothing now.
13. **The per-hub trash listings.** `GET /hubs/{hubId}/trashed-pages` and `listWhiteboards?trashed`
    are marked `deprecated: true` now, pointing to `GET /trash?kind=…`, and removed in the next
    major. Where it goes: `api/openapi.yaml`, the release notes.
14. **A delegate counted at the same level.** Unchanged: one person fills one seat per level, so
    four eyes hold. Where it goes: UC-SVC-10 *Where it ends* says so; no code change.
15. **The milestone after new entries.** The sentence reads "All done on 2 Nov · 5 open since",
    counting open entries created after `reachedAt`; the clause "not drawn while entries are open"
    is dropped as the contradiction. Where it goes: user-experience.md §7.5, an additive
    `open_since` on the container's milestone, UC-WRK-29 check 4.

## The owner's word on the second performance round — 2026-11-05

The owner accepted test/load/PLAN.md "The second round" as planned: S1 to S7 at 10⁶ and S1, S2,
S3 and S7 at 10⁷ on the workstation, about 1 h 50 min of load in two invocations, after its six
harness pieces are built. Every collected error is fixed afterwards, as the plan says.

On the evening of 2026-11-05 the owner said to start the round once it is ready, without asking
again ("run the performance test when you are ready; you need not ask me about it any more").
