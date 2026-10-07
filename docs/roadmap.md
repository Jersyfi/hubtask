# Roadmap

What is still ahead on the way to `1.0.0`. What is done is in the closed milestones under
[`archive/backlog/`](./archive/backlog/); what runs now is in [`backlog/`](./backlog/README.md).

Versioning follows [SemVer](./architecture/versioning-release.md). Before `1.0.0`, minor releases
may contain breaks; with `1.0.0`, API v1 is promised stable. There is **one** version for the
product and every first-party client ([ADR-0035](./adr/ADR-0035-one-product-version.md)).

The core track (`0.1.0` to `0.9.0`) is built, and so are the client milestones F1 to F6 and F8 to
F10 and the sign-in milestone SI. Two tracks remain: the client track, which still owes the
installed shells, and the convergence and stabilisation that close the major. Both meet in
`0.9.5`, and `1.0.0` is released when the server, the clients and the website are finished
together.

---

## Running now

| Milestone | Builds | Backlog |
|---|---|---|
| **SC — Signing in and the installation, made coherent** | One rule in one place for the second factor, the first start and the way back into an installation without SQL; operators and machines on the screen; terms of use agreed and re-agreed; AI offered by the installation ([ADR-0072](./adr/ADR-0072-ai-at-the-installation-level.md)); budgets per source; invitations without mail; the brand marks cleared; the walk by use case | [`backlog/milestone-SC.md`](./backlog/milestone-SC.md) |
| **PH — Privacy and the household** | A legal hold that wins over an erasure as far as it reaches; a deadline extended once; a person's objection to AI and a workspace's choice to make AI part of the work on a named legal basis ([`data-protection.md`](./architecture/data-protection.md) §4.1); private hubs with a transparent emergency access ([ADR-0073](./adr/ADR-0073-private-hubs.md)); accounts without a mail address for a child or a grandparent ([ADR-0074](./adr/ADR-0074-managed-accounts.md)) | [`backlog/milestone-PH.md`](./backlog/milestone-PH.md) |

**After SC and PH**, in this order: passkeys, then plans, then custom domains. SI prepared all
three without building any. Each is cut against its use cases like every milestone
([`backlog/README.md`](./backlog/README.md)).

---

## Phase 5 — The client track

The stack is decided: Svelte 5 with the webapp as a plain Vite SPA
([ADR-0030](./adr/ADR-0030-svelte-frontend-framework.md)), Tauri 2 shells for desktop and mobile
with the PWA path closed ([ADR-0031](./adr/ADR-0031-tauri-app-shell.md)), parity by default with
tenant administration reached via the web on mobile
([ADR-0032](./adr/ADR-0032-client-capability-matrix.md)), and one product UI plus a
framework-agnostic sync engine ([ADR-0033](./adr/ADR-0033-shared-client-architecture.md)).

Four rules govern the track. They are what make a client built alongside a moving core affordable
rather than a source of permanent rework.

**One version, and no second one.** [ADR-0035](./adr/ADR-0035-one-product-version.md): the client
is not versioned separately — the web app is embedded in the binary and released with it, the
shells carry the product version plus a platform build counter, the website is unversioned. What a
second number would have expressed is expressed by a maturity stage instead: `experimental`, then
`preview` (where the web app is now), then `stable` at convergence.

**The client works against a contract that has settled.** A client milestone builds the surface
for core work that has already shipped, so the cost of the parallelism stays bounded to the window
it occurs in.

**Incomplete is normal; broken is a defect.** A use case without a screen is an expected state
before convergence, and the maturity stage says so. A red client lane is not the frontend catching
up: build, lint, typecheck and test are green at every commit, on the same terms as the Go gates.
And because `packages/api-client` is generated from the specification, a contract change turns the
client red in the pull request that makes it — which is why that pull request carries the client
fix (ADR-0035 §4).

**Client milestones are letters, not versions.** They are planning buckets holding the client
issues; nothing is released by them.

### F7 — The shells

Not cut. The owner cuts it when the developer accounts and the signing certificates exist, because
no shell task can close without them.

* **The Tauri desktop shell**: SQLite and the keystore behind the `Storage` port, the updater,
  signed distribution, the webview smoke matrix (`1.0.0` prerequisite 18).
* **The mobile shell** after it: signing, the store pipeline, what is left of platform adaptation
  once the web app's phone layout exists ([`design-system.md`](./design/design-system.md) §9: the
  system conventions only a shell can follow), and the capability matrix made real in the build —
  the administration routes and the navigation row tagged for it are excluded, and each appears as
  the affordance ADR-0032 asks for, named and linked to the web app of the server the client is
  signed into.
* **One core task**: the mutation kinds [`offline-sync.md`](./architecture/offline-sync.md) §1's
  left column promises and the push frame does not carry — reminders, recurrence, template
  instantiation, the structure — because the offline *promise* is the installed clients'
  (ADR-0031), and the browser's cache did not need them.
* **SY-B**, the default synchronisation scope on a mobile shell, is set here, with a phone's
  storage to measure against ([`offline-sync.md`](./architecture/offline-sync.md) §12).

### What binds every client

* The contract: an OpenAPI-generated SDK, `/meta/capabilities` for configuration, saved views with
  a `layout` hint, the stream for live updates.
* **Localisation** as listed in
  [`i18n-l10n.md` §6](./architecture/i18n-l10n.md#6-text-direction-and-presentation) — the
  negotiated locale, the catalogue with its fallback, `Intl` from the account's preference
  (locale and time zone), moments and due dates, the week, the writing direction, the 40 % rule,
  grapheme clusters, an entry's language.
* **Accessibility** as listed in
  [`design-system.md` §10](./design/design-system.md#10-accessibility) — WCAG 2.2 AA by criterion,
  the two walks, and an accessibility statement published for the released clients, demonstrated
  for `1.0.0` rather than asserted ([`data-protection.md`](./architecture/data-protection.md) §7
  names it as where the European Accessibility Act lands).
* **Tolerant behaviour towards unknown fields.**
* **Offline-tolerant writing** with an `Idempotency-Key`.
* **No non-essential cookies without consent**, the website included. The backend uses bearer
  tokens rather than tracking cookies; nothing a client adds may quietly reintroduce them.
* **Offline conformance** per [`offline-sync.md`](./architecture/offline-sync.md) §9:
  client-assigned UUIDv7, an `op_id` per mutation, an HLC per field change, local deletion on
  `ACCESS_REVOKED` and `sync.gone`, a full resynchronisation on `sync.cursor_too_old`, and encrypted
  local storage with complete deletion on sign-out — verifiable through `hubctl sync-conformance`,
  which applies to third-party implementations too.
* The offline promise is carried by the installed clients; the browser app holds a best-effort
  cache, and no client merges — merging belongs on the server
  ([ADR-0031](./adr/ADR-0031-tauri-app-shell.md),
  [ADR-0033](./adr/ADR-0033-shared-client-architecture.md)).

A dedicated arc42 client-architecture chapter is current by `0.9.5` rather than promised.

### The website

`hubtask.eu` is live and true to what is built today; what the site may and may not claim is in
[`apps/website/AGENTS.md`](../apps/website/AGENTS.md). Its second stop is the **1.0 site** at
convergence: documentation, the licence notice, downloads for the shells, the accessibility
statement. Because it is unversioned and continuously deployed
([ADR-0035](./adr/ADR-0035-one-product-version.md)), its content can move as often as the message
does without touching a release.

What switches on at `0.9.5`, and not before it has happened: the desktop and mobile clients with
their signed installers and store listings, full offline operation, the formal accessibility
statement, and the SDKs and connectors as *available* — until they are published, the site says
they exist and are not yet published.

Open, and the owner's:

* the wordmark, which `design-system.md` §9 still lists as unfinished — the site uses the
  workbench's placeholder;
* which mailbox: the site uses `info@hubtask.eu`, while [`TRADEMARK.md`](../TRADEMARK.md) names
  `licensing@hubtask.eu`;
* whether the colour mode persists across a navigation — it costs a script on a site that loads
  none, so it is a decision with an ADR, not a commit;
* a social preview image, which needs a raster tool the repository does not have;
* a German accessibility statement, which the BFSG may expect for a German-operated service;
* showing the product interface on the site, and with it whether a scoped script is allowed;
* how much of the roadmap `/roadmap/` shows;
* whether there is a waiting list, a newsletter or an early-access signup — each collects personal
  data and therefore needs a data-catalogue entry with a legal basis and a deletion path, and
  consent for anything non-essential.

---

## Requirements that arrive late

New requirements arrive while this plan runs, and some change the core and the client at the same
time. That is ordinary work rather than an exception, and it is handled like this:

1. **The contract moves first.** `api/openapi.yaml`, then `make generate`, then `make api-client`,
   then the implementation ([ADR-0004](./adr/ADR-0004-api-first-openapi.md)); a change to the data
   model brings its migration in the same order, expand before contract.
2. **The pull request that changes the contract carries the client fix.** The generated client
   makes a break visible at typecheck time inside that very change. Fixing it there is what keeps
   `main` green, and what makes the real cost of a rename visible while reconsidering it is still
   cheap ([ADR-0035](./adr/ADR-0035-one-product-version.md) §4).
3. **Both sides get an issue, and the two are linked.** Where the change is additive they are
   separate pull requests. Where it removes or renames something a client already ships, they land
   together.
4. **The window closes when `0.9.5` opens.** Until that day a new requirement is cut into a
   milestone like any other. After it there are only defects: a new requirement waits for `1.1.0`,
   or it is an exception with its own ADR that says what it costs and why the freeze does not apply
   to it.

Rule 4 is what makes the freeze real. A stabilisation phase that still accepts features is not a
stabilisation phase, and every date after it is a guess.

---

## Phase 4 — Convergence and stabilisation (`0.9.5` – `1.0.0`)

A major is finished in three movements: parallel development, a **convergence milestone that
freezes the scope**, then stabilisation ([ADR-0035](./adr/ADR-0035-one-product-version.md) §5).
`1.0.0` is the first time the project runs them; `2.0` and every major after it follow the same
shape, which is why the rule lives in
[versioning-release.md](./architecture/versioning-release.md) rather than only here.

### `0.9.5` Convergence — where the two tracks arrive

Not a feature milestone. It holds exactly the work that can be done neither earlier nor later:

* **The coverage report, completed.** Every use case of the catalogue, where it is reachable in
  each client, and every deliberate omission with its reason. The web column exists
  ([`evidence/COVERAGE-2026-09-18.md`](./evidence/COVERAGE-2026-09-18.md), held to the catalogue by
  `tools/checkdocs`); this milestone adds the two shells' columns to the same table and closes the
  rows that name an issue. The capability matrix
  ([ADR-0032](./adr/ADR-0032-client-capability-matrix.md)) is either met or amended by supersede;
  it is not quietly missed.
* **The maturity stage goes to `stable`.** The preview banner comes off, and from that moment a
  client regression blocks a release exactly as an API regression does (ADR-0035 §2).
* **The scope window closes**, per rule 4 above, on the day this milestone opens.
* **Everything with external lead time starts here**: the shells cut release candidates, installers
  are signed, store listings are submitted. Store review is a queue somebody else owns, which is
  precisely why it cannot be a week inside `1.0.0`.
* **The website switches to its 1.0 content**, and the arc42 client-architecture chapter is
  current rather than promised.
* **The two backlogs become one.** From here there is no core track and no client track, only a
  product being stabilised.

### `1.0.0` Stabilisation

Prerequisites for `1.0.0`:

1. API v1 frozen, the OpenAPI diff clean, the deprecation process exercised.
2. Event schemas v1 stable and documented.
3. Every quality scenario QS-01 to QS-27 demonstrated (test reports in the repository).
4. Load test results against the target figures published.
5. A security review including an external pentest or code audit of the tenant boundary and the webhook/SSRF paths (open point S-1); the threat model T-01…T-20 complete with test evidence; every gate SG-1…SG-12 permanently green.
6. The upgrade path from `0.x` documented and tested.
7. *(Moved to the owner's items below.)*
8. Operating documentation complete: backup, restore (with a logged drill), monitoring, the alert catalogue with a runbook per alert, an SLO report over at least 30 days, resilience tests RT-1…RT-12 green, `hubtask_panics_recovered_total` at 0 over the period.
9. Reference deployments (Compose and Helm) tested reproducibly.
10. The data catalogue `docs/privacy/data-catalog.md` and the DPA template in place (open point S-3).
11. The audit demonstrably complete and immutable: AT-1…AT-7 green, `:verify` over a production period with no findings.
12. Backup and restore verified: BK-1…BK-10 green, a documented restore drill from every released target type, and the golden archives of every major version importable.
13. Retention rules exercised: RE-1…RE-9 green, and at least one complete run of a multi-stage chain in a production environment.
14. Offline synchronisation accepted: SY-1…SY-12 green, and the conformance test passed against at least one real client.
15. Client parity demonstrated: end-user features and profile configuration on web, desktop and mobile, administration on web and desktop; the mobile administration exclusion is the only restriction, and it behaves as [ADR-0032](./adr/ADR-0032-client-capability-matrix.md) describes — the capability is named and linked to the web app, never silently absent.
16. Accessibility demonstrated: WCAG 2.2 AA for the web app and the shells that render it, with the accessibility statement published (European Accessibility Act, [data-protection.md](./architecture/data-protection.md) §7).
17. Offline conformance from a first-party client: `hubctl sync-conformance` passed by `packages/sync-engine` against a real instance — criterion 14's "at least one real client" is named rather than hoped for.
18. The webview matrix green: the smoke suite passes on WebView2, WKWebView and WebKitGTK. One codebase across three engines is where a rendering defect hides.
19. The clients distributed: signed installers for Windows, macOS and Linux, live store listings for iOS and Android, and one updater run exercised end to end from a released version to its successor.
20. The website deployed from the release commit, carrying the 1.0 content, the licence notice and the download links.
21. The design system holds: no literal colour, spacing, radius or duration value anywhere (the lint proves it), contrast measured in CI rather than asserted, and waves 1 to 3 complete or the gap named with its reason.

### After `1.0.0`

SAML and SCIM ([`security.md`](./architecture/security.md)), and every requirement that arrives
after the `0.9.5` window closes, wait for `1.1.0`.

---

## The owner's items — not a version

Hubtask is Apache-2.0 ([ADR-0080](./adr/ADR-0080-hubtask-is-apache-2-0.md)): nothing is sold,
there is no commercial edition, and nothing about maintenance or continuation is promised. What is
left outside the version-bound milestones is the owner's, with no version and no issue:

| Item | Where it is written down |
|---|---|
| Trademark registration of the name and the logo at the EUIPO, so that [TRADEMARK.md](../TRADEMARK.md) rests on a registration rather than on use alone | (was `1.0.0` prerequisite 7) |
| Sponsorship tiers that offer no consideration in return | [licensing-editions.md](./architecture/licensing-editions.md) §5 |
| Publishing the SDKs to their registries and the n8n node and the Zapier app to their marketplaces; the SDKs' names and extraction | [ADR-0057](./adr/ADR-0057-sdk-licence-and-extraction.md), [ADR-0058](./adr/ADR-0058-connector-packages.md) |

None of them is a task in `docs/backlog/`, and none blocks a release.
