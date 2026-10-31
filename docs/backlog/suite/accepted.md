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
