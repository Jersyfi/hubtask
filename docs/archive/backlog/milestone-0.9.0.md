# Milestone 0.9.0 — Ecosystem

The goal: the product stops being reachable only through the two clients this repository builds.
The contract has been the source since `0.1.0` and every use case has been REST, MCP and
automation since the day it was registered; what nobody outside this repository has been given is
a way *in* that does not start with reading `api/openapi.yaml`. This milestone gives five: a
reference a person reads, three SDKs a program imports, two connector packages an automation
platform installs, a CalDAV surface a calendar client subscribes to and writes back through, and
importers that take what somebody built elsewhere. Beside them, the seven points earlier
milestones parked with `0.9.0` written on them — template generation (`ai-first.md` §2), the
snapshot for a large initial synchronisation (SY-C), external audit chain anchoring (A-2), the
trial restore's scope (B-4), the capacity model (O-2), image signature enforcement at deployment
(CI-3) and GitOps for the integration environment (D-3) — are answered, each in one task, so that
no open-points table still names a milestone that has closed.

Cutting it read every row that names `0.9.0` against the code before the first task, which is the
lesson of `0.7.5` and the habit of `0.8.5`, and found the milestone in three states at once. **Built
and waiting**: `POST /oauth/authorize` with PKCE has been served since H-05 for the Zapier
marketplace that needs it, the trigger polling endpoint and the REST hooks pattern since G-04 and
G-03, `audit_anchor` has had its columns since `0001_init` and `Verify.go` already reads
`LatestAnchor` — and nothing writes one. **Half-built**: the ICS feed renders VTODO-free VEVENTs
and `project-structure.md` §1 has named `CalDavController.go` since phase 0 without a line of it
existing; `packages/api-client` is the TypeScript SDK in every respect but the one that matters to
a third party, a call. **Unbuilt**: no importer, no connector package, no reference rendered from
the specification, no `sdk/` directory, and `ai-first.md`'s template row still says *Draft* in its
result column because nothing produces one.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: **publishing** anything — to npm, to PyPI, to the
n8n community node registry, to Zapier's marketplace, to `pkg.go.dev` as a module of its own. Each
of those is an account somebody owns, a name somebody registers and a licence somebody decides,
and the milestone's job is to make every one of them a *publication* rather than a build: the
packages exist, are generated, are tested against the running product, and carry the manifest a
publisher fills in. **The SDK extraction** into a separately licensed repository, which ADR-0027
defers to before `1.0.0` and which ADR-0057 — written with this backlog, carried by P-02 — asks the owner about rather than
deciding.
**An external search index**, which `0.7.0` declined and which nothing since has asked for.
**Microsoft To Do through the Graph API as a live connection** — the importer reads the JSON the
Graph API answers, fetched by the person, because an OAuth app registration at Microsoft is an
account somebody owns (see above). **SAML, SCIM and a SIEM connection**, which `security.md` §15
and A-3 keep after `1.0.0`. **The public status page** (O-3, after `1.0.0`). **Cron notation** for
`SCHEDULE` triggers, sugar in `0.5.0` and sugar still. And every **screen** that renders any of
this — an import wizard, the anchoring settings, a CalDAV address shown beside the ICS one —
which is the client track's, one window behind, and specifically `F6`'s or later: `F5` builds
the surface for `0.7.0` and `0.8.0`, exactly as `roadmap.md` says it should.

Fourteen decisions taken while writing this backlog, so that nobody re-derives them:

* **The letter is P, and O is skipped on purpose.** N was `0.8.5`. O is the letter of
  `observability-reliability.md`'s open points, and `O-2` is one of this milestone's own tasks: a
  backlog in which `O-02` builds `O-2` is a backlog somebody misreads in an issue title for
  years. I and L were skipped for the same kind of reason; one more letter costs nothing.
* **The reference is rendered from the document, and the document travels through the generated
  package.** `apps/website` may import from `packages/design-system` and nothing else
  (`project-structure.md` §2.1), and a build script that reached across the tree for
  `api/openapi.yaml` would be the deep import the workspace lint exists to catch. So
  `make api-client` writes the document itself — `openapi.json`, and the event schemas beside it
  — into `packages/api-client/dist/`, the package exports it, and the map gains one edge:
  `apps/website → packages/api-client`, read-only, at build time. The reference is prerendered
  with zero scripts like every other page of the site, one page per tag, and wave 4's `CodeBlock`,
  `ApiEndpointCard`, `ParameterTable` and `Callout` are what it is made of. It is documentation
  rather than marketing, which is why the website lane's brief — still open — does not block it.
* **Three SDKs, two generators, no new dependency, and the licence is asked rather than decided.**
  The Go client comes from `oapi-codegen`, the tool that already generates the server's types,
  into `sdk/go/`; it depends on `oapi-codegen/runtime`, which `go.mod` already carries. The
  TypeScript and Python clients come from one generator of this repository's own,
  `tools/sdkgen`, which reads the document and emits a thin typed client over `fetch` and over
  `urllib` respectively — no runtime dependency in either, because a library that pulls a tree
  into somebody's project is a library they audit before they adopt. The TypeScript client lands
  in `packages/api-client/dist/` as generated output, which is what that package is allowed to
  hold; the Python one in `sdk/python/`. All three sit under the repository licence, and
  **ADR-0057** (proposed, written with this backlog; P-02 carries it) puts to the owner what ADR-0027 deferred: whether the SDKs are
  extracted, under which licence, and under which names. Nothing in this milestone changes the
  licence model; the ADR is where that decision is made if it is made.
* **The connector packages carry no dependency this repository installs.** An n8n community node
  imports `n8n-workflow`; a Zapier app imports `zapier-platform-core`. Both are installed by the
  platform that loads the package — n8n resolves a community node against its own tree, Zapier's
  CLI against the app's — so both packages name them as peer dependencies and this workspace's
  lockfile gains nothing. What that costs is a typecheck against the platform's own types, which
  cannot run here; what replaces it is a schema of each platform's package format written into
  the generator's tests, and the e2e walk in P-17 that loads the node into a real n8n. **ADR-0058**
  (proposed, written with this backlog; P-04 carries it) records the shape and asks the owner the one question it cannot answer alone:
  whether to take `n8n-workflow` as a development dependency for the sake of that typecheck.
* **The n8n node is declarative and generated; the Zapier app is generated and thin.** n8n's
  declarative style is a JSON description with routing — no code per operation — and a generator
  that reads the document and the capability manifest's vocabulary produces it whole, which is
  what `automation.md` §3.3 promised ("so that they stay complete automatically"). Zapier's
  format is code per trigger and action, so the generator emits one file per operation over one
  shared request helper, and the triggers are the REST hooks pattern (subscribe, unsubscribe,
  `performList` through trigger polling) that `0.5.0` built for exactly this.
* **CalDAV is one collection per calendar feed, and it is served by the `api` role under the
  feed's owner.** A calendar feed already names a view, an owner and a token; a CalDAV calendar is
  that feed with a second transport. The principal is the account, the calendar home lists the
  account's feeds, each feed is a calendar of `VTODO` components — an entry with a due date is a
  todo, not an event, and a calendar client that speaks CalDAV is a client that shows todos —
  authenticated by HTTP Basic with the account's address and a personal access token as the
  password, because that is what every CalDAV client can send and no feed token is a bearer.
  `PROPFIND`, `REPORT` (`calendar-query`, `calendar-multiget`), `GET` with `ETag` and a
  collection `getctag` in P-06; `PUT` and `DELETE` in P-07, each performed through the ordinary
  use cases as the token's account. No `MKCALENDAR`: a calendar is made by creating a feed.
* **The importers write archive records and the restore applies them.** `backup-restore.md` §9
  decided this in phase 0 — "the same internal intermediate form as an archive … one ingestion
  path, not two" — and the `0.5.0` backlog's line about "the jumble's ingestion path" is the one
  that was wrong and is corrected. A converter turns a Trello board, a Google Tasks list or a CSV
  file into `archive.Record`s under freshly minted identities, the applier runs them in `MERGE`
  with `skip` into the hub the person named, and the report is the restore's report. The file
  arrives through the three-step media upload the product already has, bounded by the same
  size and type gates, because an import is a file and the product has one way to take a file.
* **Microsoft To Do is imported from the Graph API's JSON, fetched by the person.** Microsoft To Do
  has no export; the only door is `https://graph.microsoft.com/v1.0/me/todo/lists` and each list's
  `tasks`, and the shape of both is documented and stable. A live connection would need an app
  registration, a consent screen and a token this product would have to hold — an account
  somebody owns. The importer reads the JSON the Graph Explorer or a script hands back, and
  `hubctl import todo` shows the two requests to make.
* **Template generation is a suggestion of a new kind, and accepting it creates the template.** The
  shape J-06 and J-07 settled: a prompt, a provider call as a job, a stored suggestion with its
  provenance, and acceptance as the ordinary use case performed by the accepting person —
  `CreateTemplate` with the tree the payload carries. `SuggestionKind` gains `TEMPLATE`
  additively, the target is the collection the template will belong to, and the draft is a
  `Template` in the contract's own shape rather than a shape invented for the prompt, so that a
  client renders it with the template editor it already has.
* **SY-C is a stream, not a file at a target.** The initial synchronisation N-02 built is a page
  sequence; a workspace of two million entries joining from a phone needs the same records as one
  response the device can write straight to disk. `POST /sync:snapshot` answers newline-delimited
  JSON, one record per line in the walk's order, with the same permission check per record and
  the delta cursor as the last line. A target the device cannot reach (F4's rule about exports)
  would be no use to a device, and a job the device polls would be a page sequence with extra
  steps.
* **The anchor goes to a backup target, as a file, once a day.** `audit.md` §3 leaves the target
  open between a WORM bucket, a log service and a signed email; the product already has a target
  abstraction with an object-lock recommendation on it (B-3), and a log service or a mail would be
  a second transport for one small JSON object. `ConfigureAuditAnchoring` names a target, the
  self-seeded daily job writes `hubtask-anchor-<tenant>-<date>.json` — the sequence, the chain
  hash, the moment — and records the receipt in `audit_anchor`; `:verify` reports the last anchor
  and, when asked, reads the file back and compares. Under object lock the file cannot be shortened
  by the credential that wrote it, which is the whole point.
* **The trial restore in the default schedule is `INSPECT` of the archive just written.** B-4 asked
  for its scope. A `NEW_TENANT` trial after every backup would mint a workspace a day in a single
  mode installation that has exactly one; an `INSPECT` reads the whole archive back, verifies every
  checksum, decrypts every member and produces the difference report, which is the proof that the
  archive is restorable by the only code that will ever restore it. The quarterly `NEW_TENANT`
  drill stays the operator's (`backup-restore.md` §11), and the chart's restore-drill job is where
  it runs.
* **CI-3 lands as a policy the chart can render and a step the integration deploy runs; D-3 closes
  by saying what it has said since ADR-0046.** Kyverno is a cluster operator's choice and a
  `ClusterPolicy` needs cluster scope, so the chart renders one behind `imagePolicy.enabled`
  (default off) with the release workflow's keyless identity written in, and the platform that
  runs production decides whether to apply it. The integration deploy verifies the image's
  signature with `cosign verify` before `helm upgrade`, which is enforcement at the one deployment
  this repository performs. D-3 is closed for the integration environment with its own sentence:
  one cluster, one operator, push stays.
* **The capacity model is a section written from the evidence that exists.** O-2 asks for real load
  data and there are three files of it — RT-6's overload run, O-1's first ramp and the nightly
  regression baseline. P-16 writes `observability-reliability.md` §13 from those numbers: items
  per tenant against memory, connections and disk, the two knees the runs showed, and the
  configuration values a provider sets per size. It names the run each number came from and says
  what has not been measured, because a capacity model that hides its provenance is a guess with
  a table around it.
* **The `0.9.0` roadmap row is rewritten to what this file builds, and the milestone walks itself.**
  The row promises six things in twelve words; P-17 says what each became, closes the seven
  parked points in their tables, corrects `automation.md` §3.3's "milestone 0.7+", the
  `0.5.0` backlog's ingestion line and arc42's "CalDAV later", and files the walk: Reminders on
  macOS subscribed over CalDAV, a Trello export imported, each SDK's example run, the n8n node
  loaded into a real n8n against the Compose stack.

---

## P-01 — The public API reference **[L]**

*Depends on: nothing.*

`hubtask.eu/developers/` says the API is the product and links to a YAML file. This task renders
the file. `make api-client` grows a second output beside the types: `packages/api-client/dist/`
gains `openapi.json` (the document, dereferenced enough to render, `$ref`s within components
kept) and `events/` (the CloudEvents schemas from `api/events/`), the package exports both, and
`apps/website` gains the one edge decision 2 allows — read at build time, prerendered, zero
scripts, like every page of the site.

The reference is one page per tag under `/developers/api/<tag>/`, an index of tags, and one page
for the events. Each operation is an `ApiEndpointCard` — method, path, summary, the description
rendered from the specification's Markdown, the parameters as a `ParameterTable`, the request and
response schemas as `ParameterTable`s over the resolved schema, the message codes the operation
names, and a `CodeBlock` with a `curl` example built from the operation (a bearer placeholder,
the path with its parameters named, a body that is the schema's example where it has one). The
security schemes and the problem document get a page of their own, written once, because every
operation refers to them. Wave 4's four components are built here with their stories —
`CodeBlock` (a `<pre>` with a copy control and a language label, no highlighter: a highlighter is
a dependency and the site ships no script), `ApiEndpointCard`, `ParameterTable`, `Callout` — and
`design-system.md` §4's wave 4 row records them as built.

The workspace map gains `apps/website → packages/api-client` in `project-structure.md` §2.1 and
in `build/lint-workspace-map.mjs`, and the lint's selftest still proves every forbidden edge is
caught.

**Acceptance:** `pnpm --filter @hubtask/website build` produces a page per tag and a page for the
events with zero scripts (`build/check-static.js` still passes); every operation in
`api/openapi.yaml` appears exactly once, proved by a test that counts them against the document;
a `curl` example exists per operation and names every path parameter; the four components have
stories and `pnpm test` in the design system is green; `make api-client` produces `openapi.json`
and the event schemas and the api-client lint still refuses hand-written code; the workspace lint
accepts the new edge and its selftest is green; `pnpm -r build lint typecheck test` green.

**Read:** `api-guidelines.md` §1–§6; `design-system.md` §4 (wave 4); `project-structure.md` §2.1;
`packages/api-client/CLAUDE.md` and `scripts/generate.js`; `apps/website/src/routes/developers/`;
`build/lint-workspace-map.mjs`; `docs/marketing/website-1.0.md` §5

---

## P-02 — ADR-0057, and the Go SDK **[L]**

*Depends on: nothing. Carries a proposed ADR that waits on the owner; the code does not.*

Two halves. **The ADR first**: ADR-0027 deferred "extracting the generated SDKs into a separately
licensed repository" to before `1.0.0` because a client library under BSL 1.1 is one nobody may
use in commercial production. ADR-0057, written with this backlog, lays the question out for the owner: which
packages are SDKs (the Go client, the TypeScript client, the Python client — and not
`packages/api-client`'s types, which the first-party apps use), the candidate licences (Apache-2.0,
which is the Change License already named; MIT), the names they would be published under, and
what extraction costs (a repository per language, a release step per product release, a
`go.mod` of its own for Go). It recommends Apache-2.0 for the three and says why; it decides
nothing, and the packages land under the repository licence with an SPDX header until it is
accepted. The task re-reads it against what it built and amends it where the building taught
something the cut could not know.

**The Go SDK**: `sdk/go/hubtask/` generated by `oapi-codegen` in client mode from
`api/openapi.yaml` — `client.gen.go` with `ClientWithResponses`, the types shared with what the
server already generates — through a new `make sdk-go` target that `make generate` calls, so a
contract change regenerates it and CI's no-diff check covers it. Beside the generated file, and
the only hand-written code: a `doc.go` naming the module path, the authentication (a bearer
`RequestEditorFn`), the idempotency key helper, and the problem document as a typed error the
caller can `errors.As` — under thirty lines, because every line here is one the extraction has to
carry. `examples/go/` holds a program that lists a hub's collections and creates an entry, and
the contract test runs it against the in-process server.

**Acceptance:** ADR-0057 is current against what landed and the pull request names it as
waiting on the owner; `make generate` produces `sdk/go/hubtask/client.gen.go`
and produces no diff when run twice; `go build ./...` and `go vet ./...` include the SDK; the
example compiles and the contract test drives it through a create and a read; the problem error
round-trips a `422` with its field errors; `project-structure.md` §1 names `sdk/`; `make verify`
is green.

**Read:** ADR-0027 (the deferral); ADR-0013 and `licensing-editions.md`; ADR-0004; `Makefile`
(the `generate` target and `OAPI_CODEGEN_VERSION`); `api/oapi-codegen.yaml` or wherever the
server's generation is configured; `test/contract/`; `cmd/hubctl/Client.go` (the client the
product already writes by hand, and what the SDK replaces for a third party)

---

## P-03 — `tools/sdkgen`: the TypeScript and Python SDKs **[L]**

*Depends on: P-02 (the ADR's list of packages; the directory).*

One generator, two targets, no dependency in either output. `tools/sdkgen` is a Go program that
reads `api/openapi.yaml` (through the YAML parser already in the module graph, into a small model
of paths, operations, parameters and named schemas — not a general OpenAPI library, which would
be a dependency for a tool that needs a fifth of one) and emits:

* **TypeScript**, into `packages/api-client/dist/client.ts`: a `HubtaskClient` class over `fetch`
  — base URL, bearer, one method per `operationId` typed against the `paths` types
  `openapi-typescript` already generates, path parameters as arguments, query parameters and the
  body as typed objects, `Idempotency-Key` and `If-Match` as options where the operation takes
  them, and a `ProblemError` carrying the problem document. Generated output in a package that
  holds generated output only; the package's CLAUDE.md sentence about "no fetch layer yet" is
  rewritten to say what is there now and that the first-party apps still go through the engine.
* **Python**, into `sdk/python/hubtask/`: a `Client` class over `urllib.request`, one method per
  operation with keyword arguments, `TypedDict`s for the named schemas, `ProblemError`, and a
  `pyproject.toml` with no dependencies and the SPDX classifier ADR-0057 will settle. Python 3.11
  as the floor, for `typing.NotRequired`.

`make sdk` runs the generator for both; `make generate` calls it; the no-diff check covers
`sdk/python/` (committed) and the api-client lint covers `dist/client.ts` (generated, ignored,
reproducible). Each SDK has an example that does what the Go one does, and each is driven once
against the in-process server: the TypeScript one from the api-client test, the Python one from
the contract test through `python3` where the runner has it and skipped by name where it does not.

**Acceptance:** `make sdk` produces both and produces no diff twice; every `operationId` in the
document has a method in each client, proved by a test that counts; a `422` becomes a
`ProblemError` with field errors in both; the TypeScript client typechecks against the generated
`paths`; the Python package imports under `python3 -m compileall` and the example runs against the
in-process server; `tools/sdkgen` has table tests over a fixture document covering a path
parameter, a query parameter, a body, an action route (`:complete`) and an enum;
`project-structure.md` §1 names `tools/sdkgen` and `sdk/python`; `make verify` is green.

**Read:** P-02's ADR; `packages/api-client/scripts/generate.js` and `check-generated.js`;
`api/openapi.yaml` (`securitySchemes`, `Problem`, the `Idempotency-Key` and `If-Match` parameters);
`tools/checkdocs` (the shape of a tool in this repository); `api-guidelines.md` §5, §6

---

## P-04 — ADR-0058, and the n8n community node **[L]**

*Depends on: P-01 (the document in the package).*

The ADR first: ADR-0058, written with this backlog, records decision 4 — two connector packages, both generated,
both naming their platform library as a peer dependency this workspace never installs, the
typecheck against the platform's types traded for a format schema in the generator's tests and
the walk in P-17 — and asks the owner the one question: whether `n8n-workflow` should be taken as
a development dependency of `packages/n8n-nodes-hubtask` so that the node's description is
typechecked here. It decides nothing else, and the task amends it where building the node
taught something.

The node: `packages/n8n-nodes-hubtask`, a pnpm workspace member whose `scripts/generate.js`
reads `@hubtask/api-client`'s document and emits `nodes/Hubtask/Hubtask.node.json` (the
declarative description: one resource per tag, one operation per `operationId` with its
parameters as properties and its `routing`), `credentials/HubtaskApi.credentials.json` (base URL
plus a personal access token as a bearer — auth phase 1 of `automation.md` §3.3; OAuth2 is the
Zapier app's, where the marketplace demands it), and `HubtaskTrigger.node.json` — the webhook
trigger over the REST hooks pattern, with `event_types` offered from the manifest's list. The
`package.json` carries the `n8n` block the registry reads (`n8nNodesApiVersion`, `nodes`,
`credentials`) and `n8n-workflow` as a peer dependency. The generator's test holds the output to
a schema of n8n's declarative node format written from its documentation, and asserts that every
operation in the document is reachable.

**Acceptance:** ADR-0058 is current and named in the pull request as waiting on the owner; the package builds from the document with no dependency added to the lockfile; every
`operationId` is an operation of the node and every event type of the manifest is offered by the
trigger, proved by tests; the description passes the format schema; `pnpm -r build lint typecheck
test` green; the workspace map records the package (`packages/n8n-nodes-hubtask → packages/api-client`).

**Read:** `automation.md` §3.1–§3.3; `api-guidelines.md` §7 (tokens), §11; G-03 and G-04 in
`milestone-0.5.0.md` (the REST hooks pattern and trigger polling); H-05 in `milestone-0.6.0.md`;
ADR-0027 §2.1; n8n's declarative-style node documentation (read, not vendored)

---

## P-05 — The Zapier app **[L]**

*Depends on: P-04 (the ADR, the generator's model of the document).*

`packages/zapier-app`, generated the same way from the same document: `index.js` with the app
definition, `authentication.js` for OAuth2 authorization code with PKCE against `/oauth/authorize`
and `/oauth/token` (H-05 built the provider side for exactly this; the app's client is registered
with `POST /oauth/clients` by the workspace that installs it, and the app carries the redirect
URI Zapier documents), `triggers/<event>.js` per event type — REST hooks: `performSubscribe`
creates the webhook subscription, `performUnsubscribe` deletes it, `performList` reads
`/integrations/triggers/{eventType}` for the sample — `creates/<operation>.js` for the writes
the marketplace's review expects (create an entry, update one, complete one, add a comment,
create a collection) and `searches/<operation>.js` over `POST /search` and `POST /items:query`.
`zapier-platform-core` is a peer dependency; the generator's test holds the output to a schema of
Zapier's app definition written from its documentation, including the `sample` every trigger and
create must carry, taken from the specification's examples.

**Acceptance:** the package builds from the document with no dependency added; every event type
is a trigger, the five creates and two searches exist, and each carries a sample, proved by tests;
the app definition passes the format schema; authentication names the two endpoints and PKCE;
`pnpm -r build lint typecheck test` green; the workspace map records the package.

**Read:** P-04; `api-guidelines.md` §11 (auth phase 2); H-05 in `milestone-0.6.0.md` and
`core/application/service/identity/OAuth*.go`; `automation.md` §3.1 (the hooks pattern); Zapier's
platform CLI schema documentation (read, not vendored)

---

## P-06 — CalDAV, the read half **[L]**

*Depends on: nothing.*

`presentation/calendar` grows the second transport decision 6 describes. The tree:
`/caldav/` (the root, `current-user-principal`), `/caldav/principals/<account>/` (the principal;
`calendar-home-set`), `/caldav/calendars/<account>/` (the home; one child per calendar feed the
account owns), `/caldav/calendars/<account>/<feed>/` (the calendar: `VTODO` components, one per
entry the feed's view answers, each at `<item-id>.ics`). `PROPFIND` at depths 0 and 1 with the
properties the common clients ask for — `resourcetype`, `displayname`,
`supported-calendar-component-set` (`VTODO`), `getctag`, `getetag`, `sync-token` — `REPORT` with
`calendar-query` (a filter on `VTODO`, the time range applied to `DUE`) and `calendar-multiget`,
`GET` of one todo with its `ETag`, and `OPTIONS` answering `DAV: 1, calendar-access`. The
`getctag` and `sync-token` are the view's result fingerprinted — the same value, so a client using
either sees a change when any entry moved.

The renderer gains `VTODO`: `UID`, `SUMMARY`, `DUE` (a `DATE` for an all-day entry, a UTC
`DATE-TIME` for a timed one — the ICS feed's convention), `STATUS` (`NEEDS-ACTION` / `COMPLETED`),
`COMPLETED`, `PERCENT-COMPLETE` for a parent with completed children (the roll-up the domain
already computes), `LAST-MODIFIED`, `URL`; and no notes, no assignee, no comment, for D-08's
reason. Authentication is HTTP Basic: the address and a personal access token as the password,
resolved by the token path the API already has, the feed's owner and the token's account having
to be the same person — a token grants nothing the feed's owner cannot see. A calendar client
sends Basic on every request and stores the password, which is why it is a revocable token and
never the account password.

The controller lives in `presentation/calendar/CalDav*.go`, its routes are registered by the `api`
role where the feed's are, every response is XML written by `encoding/xml` and never by string
concatenation, and the XML the client sends is parsed with the decoder's entity expansion off.
`security.md` §4 gains the threat row: a WebDAV surface parsing client XML.

**Acceptance:** a `PROPFIND` walk from the root to a todo answers what RFC 4791 §5 and §7 require
and what Apple Reminders and Thunderbird ask for (the request bodies recorded as golden fixtures);
`calendar-query` with a time range answers only the entries due in it; `ETag` changes when the
entry changes and `getctag` when any entry of the view does; a token of another account is
refused; a feed the token's account does not own is `404` in the same shape as none; an XML
bomb is refused before it expands; the threat row is in `security.md`; the calendar tests hold
the `VTODO` golden file; `make verify` is green and the contract test still passes (the CalDAV
tree is outside `openapi.yaml` the way the ICS feed's token route is inside it — the task decides
and records whether the routes are declared in the specification, and the answer is yes for the
paths and no for the WebDAV methods it cannot express).

**Read:** `presentation/calendar/Ics.go` and `presentation/rest/CalendarController.go`; D-08 in
`milestone-0.4.0.md`; `security.md` §4, §5; `core/domain/service/Completion.go`; RFC 4791 §5, §7,
§9; RFC 5545 §3.6.2 (`VTODO`); RFC 6578 (`sync-token`)

---

## P-07 — CalDAV, the write half **[L]**

*Depends on: P-06.*

A todo ticked in Reminders is `PUT` back with `STATUS:COMPLETED`; one whose date was dragged is
`PUT` with a new `DUE`; one deleted is `DELETE`. The controller reads the `VTODO`, diffs it
against the entry, and performs the ordinary use cases as the token's account through the
registry: `CompleteWorkItem` or `ReopenWorkItem` for `STATUS`, `SetDueDate` or `ClearDueDate`
for `DUE`, `UpdateWorkItem` for `SUMMARY`, `TrashWorkItem` for `DELETE`. `If-Match` against the
`ETag` is honoured and its absence is not a licence: a `PUT` without one is refused with `428`,
because a calendar client that lost the race would otherwise overwrite a change it never saw. A
`PUT` to a new `UID` creates an entry in the view's collection where the view names exactly one
and is refused where it does not, because a todo made in a calendar has to land somewhere the
person meant. Every property the renderer does not write is preserved by ignoring it — a client's
`X-APPLE-…` lines are its own — and every property the product does not model (`PRIORITY`,
`CATEGORIES`, `DESCRIPTION`) is refused in a `PUT` by name in the response rather than dropped,
which is the contract's rule about silent ignoring applied to a wire format.

**Acceptance:** completing, reopening, redating and renaming through `PUT` land through their use
cases and appear in the activity and the change log exactly as an API write does; a `PUT` without
`If-Match` is `428`, with a stale one `412`; a new `UID` creates in a single-collection view and
is refused in a hub-wide one with a code; `DELETE` trashes; an unmodelled property is refused by
name; a token without the write scope is refused; the golden fixtures cover a Reminders round
trip; `make verify` is green.

**Read:** P-06; `core/application/service/work/CompleteWorkItem.go`, `SetDueDate.go`,
`UpdateWorkItem.go`, `TrashWorkItem.go`; `core/application/registry` (performing a use case as a
caller — the sync push's precedent in `core/application/service/sync/PushChanges.go`);
`api-guidelines.md` §5 (`If-Match`); RFC 4791 §5.3.2

---

## P-08 — Import, the frame, and CSV **[L]**

*Depends on: nothing.*

`ImportEntries`, in decision 7's shape. The file arrives
through `RequestMediaUpload` and `ConfirmMediaUpload` like any other file, under a new media
purpose `IMPORT` that the confirmation accepts for the four content types the converters read and
nothing attaches. `POST /imports` names the media object, the `kind` (`CSV` here; `TRELLO`,
`GOOGLE_TASKS`, `MICROSOFT_TODO` land in P-09 and P-10 and are refused by name until then) and the
hub the result goes into, needs `STRUCTURE` on that hub, and answers a job. The job reads the
object, hands it to the converter for its kind, receives `archive.Record`s — containers under the
hub, buckets, labels, entries, sub-entries, comments where the source has them — under identities
minted from the source's own identifiers through the same derivation `MERGE`'s `duplicate` uses,
so that a second import of the same file produces the same identifiers and `skip` makes it a
no-op rather than a second copy. The applier runs them as a restore in `MERGE` with `skip`,
inside the job, and the job's result is the restore's report: created, skipped, refused, each
by entity. `GET /imports/{importId}` is the job. A converter is a port in `core/port/importer`
with one implementation per kind in `infrastructure/importer`, the CSV one here: a header row
mapped by name to `title`, `notes`, `due`, `completed`, `labels`, `bucket`, `parent` (a title or
a row number), with the mapping declared in the request where the header does not match, and
the encoding detected as UTF-8 or refused.

`hubctl import <kind> <file> --hub <id>` performs the upload, the confirmation and the request
and follows the job. The data catalogue gains nothing: an import writes rows the catalogue already
describes, and the media object is deleted when the job ends, success or failure, because a file
somebody imported is not a file they attached.

**Acceptance:** a CSV of two hundred rows with parents, labels and buckets lands as one collection
under the named hub with the tree intact; importing it twice creates nothing the second time; a
row with an unparseable date is refused by row number and the rest land; the media object is
gone when the job ends; a caller without `STRUCTURE` on the hub is refused; `ImportEntries` is a
registered use case with metric, span and audit row; the archive's cross-tenant suite covers the
applier's new caller; the contract test passes; `hubctl import csv` runs in the e2e script;
`make verify` is green.

**Read:** `backup-restore.md` §8.2, §8.3, §9; `tenant-export.md` §5, §6; `core/application/archive/`
(`Record`, `Writer`); `core/application/service/backup/Apply.go` and `Restore.go` (the `MERGE`
path and the derived identity); `core/application/service/media/RequestMediaUpload.go` and
`ConfirmMediaUpload.go` (purposes); `core/application/service/job/`; ADR-0019; ADR-0020

---

## P-09 — Import: Trello **[L]**

*Depends on: P-08.*

Trello's board export is one JSON document: `lists` become buckets of one collection named after
the board, `cards` become entries with their `desc` as notes, `due` and `dueComplete`, their
`labels` — each a colour Trello names and a name the person gave — mapped to labels of the
collection with the nearest of the ten label tokens, `checklists` become sub-entries under the
card (one `WORK_PACKAGE` per checklist where there are several, `ACTIVITY` per check item, the
capability profile permitting), `actions` of type `commentCard` become comments attributed to a
system author with the commenter's name in the text, and `closed` cards land archived rather than
dropped. Attachments are URLs Trello hosts behind its own authentication and are recorded as a
line in the notes rather than fetched — the product makes no outbound call on behalf of an
import. Members are not mapped: a Trello member is not an account here, and the assignment is
lost with a count in the report.

**Acceptance:** a real board export (the fixture is an anonymised one under `infrastructure/importer/testdata/`)
lands with its lists, cards, labels, checklists and comments; the counts in the report match the
fixture; a card with three checklists becomes three work packages; an archived card is archived;
the unmapped members are counted; `hubctl import trello` runs in the e2e script; `make verify` is
green.

**Read:** P-08; `core/domain/model/work/CapabilityProfile.go` (which types may hold which);
`core/domain/model/shared/LabelTokens.go`; Trello's export format (read from a real export)

---

## P-10 — Import: Google Tasks and Microsoft To Do **[L]**

*Depends on: P-08.*

Two converters over two JSON shapes. **Google Tasks** arrives as Takeout's `Tasks.json`: a list
of task lists, each with `items` carrying `title`, `notes`, `due` (a date, never a time), `status`
(`needsAction` / `completed`), `completed`, and `parent` for one level of nesting — one collection
per list, one entry per item, a child under its parent. **Microsoft To Do** arrives as the Graph
API's JSON, decision 8: the `todoTaskList` collection and each list's `todoTask` items with
`title`, `body.content`, `dueDateTime` with its zone, `status`, `importance`, `isReminderOn` with
`reminderDateTime`, `checklistItems` and `linkedResources` — one collection per list, an entry
per task with its due date in the zone Graph names, a reminder where one was set, a child per
checklist item, and `importance: high` recorded as nothing because the product has no priority
(`ai-first.md` §2's answer, again). Both refuse a file whose shape is not theirs by name rather
than importing nothing.

**Acceptance:** each fixture lands with its lists and nesting; a Graph due date in `Pacific
Standard Time` lands as that zone's date; a reminder lands as a reminder; the wrong file for a
kind is refused with a code; both run in the e2e script; `make verify` is green.

**Read:** P-08; `core/domain/model/scheduling/` (due dates with zones, reminders);
`infrastructure/i18n` (Windows zone names are not IANA — the mapping the converter needs is a
table, and where it comes from is the task's decision to record); Google Takeout's Tasks format
and the Graph `todoTask` resource (read, not vendored)

---

## P-11 — Template generation **[L]**

*Depends on: nothing.*

The last of `ai-first.md` §2's seven rows. `AiGenerateTemplate` takes a description in the
caller's words and the collection the template would belong to, asks the workspace's provider —
as a job, through the budget, the consent and the breaker exactly as `SuggestFields` does — for a
template in the contract's own shape: a name, a tree of nodes each with a type, a title, optional
notes and a relative due offset, held to the collection's capability profile in the prompt and
checked against it in the narrowing so that a node the profile refuses is dropped rather than
stored. The answer is a suggestion of the new kind `TEMPLATE` with the collection as its target,
its provenance as every suggestion has, and `AcceptSuggestion` on it is `CreateTemplate`
performed as the accepting person; dismissing closes it. The suggestion strip a client renders
needs nothing new: a `TEMPLATE` payload is a `Template`.

**Acceptance:** `POST /templates:generate` answers a job and the job records a `TEMPLATE`
suggestion whose payload validates as a `Template`; a node of a type the profile refuses is absent
from the payload and the fact is in the job's result count; accepting creates the template
through `CreateTemplate` with the accepting person's rights and an audit entry; the fake provider
test covers a well-formed answer, an answer with a refused node, and no answer (`ai.unavailable`);
`ai-first.md` §2's row is rewritten to *shipped in P-11*; `SuggestionKind` gains `TEMPLATE`
additively and the contract test passes; the parity test is green; `make verify` is green.

**Read:** `ai-first.md` §2, §3; J-06, J-07 in `milestone-0.7.0.md`; K-01…K-03 in
`milestone-0.7.5.md` (the narrowing, and the allow list — the lesson of #529: a key that passes
the gate can still be discarded before the write);
`core/application/service/suggestion/Producing.go`, `Actions.go`, `Asking.go`;
`core/application/service/work/Template.go`, `CreateTemplate` in `ChangeTemplate.go`;
`core/domain/model/work/Template.go`

---

## P-12 — SY-C: the snapshot **[L]**

*Depends on: nothing (N-02 landed).*

`POST /sync:snapshot` takes what `:pull` takes without a cursor — `device_id`, `scopes` — and
answers `application/x-ndjson`: the initial synchronisation's records in the walk's order, one
per line, each the same `SyncChange` a page would carry, written as they are read so that the
first byte arrives before the last row is counted, and the delta cursor as the last line in a
record of its own (`{"cursor": …}`), minted from the log position read before the first row
exactly as N-02 does. Permission is checked per record, a scope narrows the walk, the device is
registered by turning up, and the response is bounded the way the stream is — a deadline, a
byte budget per connection, and the connection counted in `hubtask_stream_connections` because
it is one. A device that lost the connection halfway has no cursor and starts again, which is
what a page sequence offers it too; the difference is one request rather than a thousand.

`hubctl sync snapshot` writes the stream to a file and, on `--apply`, loads it into the
profile's store; the conformance runner's check 4 gains a second half that walks the snapshot
and compares its record count with the page sequence's. `offline-sync.md` §3.1 and §12 say what
SY-C became.

**Acceptance:** the snapshot of a seeded workspace and its page sequence answer the same records
in the same order, proved by a test; the last line is a cursor a following `:pull` accepts
without a gap (the N-02 test, again, against the stream); a member with partial access gets
exactly what they may read; the connection is counted and bounded; the cursor minted mid-stream
is not answered when the stream dies before the last line; `offline-sync.md` §12's SY-C row is
closed; `make verify` is green and the contract test passes.

**Read:** `offline-sync.md` §3.1, §12; N-02 in `milestone-0.8.5.md`;
`core/application/service/sync/InitialSync.go`, `PullChanges.go`; `presentation/rest/SyncController.go`
and `presentation/stream/` (the bounded long-lived response); `cmd/hubctl` (`sync`)

---

## P-13 — A-2: the chain anchored outside the database **[L]**

*Depends on: nothing.*

`audit.md` §3 promises "a job and a documented configuration point" for anchoring and
`audit_anchor` has waited for it since the first migration. `ConfigureAuditAnchoring` names a
backup target of the workspace (or none, which switches it off), needs `STRUCTURE` and is
audited; the self-seeded daily job — seeded by the configuration's write, rescheduling itself
the way every per-tenant job does — reads the chain's end (`last_seq`, the hash), writes
`hubtask-anchor-<tenant>-<YYYYMMDD>.json` to the target with the sequence, the hash, the moment
and the product version, and records the row with the target's identifier as `destination` and
the object's digest as `receipt`. `AuditVerify` gains `anchors: true`: beside the chain check it
reads the last anchor's file back from the target, compares the hash it holds with the chain at
that sequence, and reports `anchored_until` and whether the external copy agrees. The
recommendation B-3 made for a tenant's own target — object lock, retention equal to the backup
window — is restated in `audit.md` §3 for this target with the arithmetic that applies to it: an
anchor is small, so the lock can be long.

**Acceptance:** with anchoring configured the job writes one file a day and one `audit_anchor`
row, proved with the clock controlled; `:verify` with `anchors: true` reads it back and reports
agreement, and reports disagreement when the test rewrites the file; a chain rewritten below the
anchor is reported at the anchor as well as at the break; a workspace without a target anchors
nothing and `:verify` says so; the configuration is audited with before and after; the use case
is registered with metric, span and parity; `audit.md` §3 and §12 (A-2) are current; the data
catalogue's `audit_anchor` row is checked; `make verify` is green.

**Read:** `audit.md` §3, §12; `core/application/service/audit/Verify.go`;
`core/application/repository/audit/Port.go` (`Anchor`, `LatestAnchor`);
`core/application/service/backup/Target.go` and `core/port/backupstorage/Port.go`;
`core/application/service/lifecycle/RetentionSweep.go` (a self-seeded per-tenant job);
`backup-restore.md` §12 (B-3)

---

## P-14 — B-4: the trial restore in the default schedule **[L]**

*Depends on: nothing.*

Decision 12. A backup schedule gains `trial_restore` (default `true` for a new schedule; existing
rows keep `false`, and the migration says so): when it is on, the run that wrote a `FULL` archive
is followed, in the same job, by an `INSPECT` restore of that archive — every member read, every
checksum verified, every encrypted member decrypted with the key the schedule names, the
difference report produced against the workspace and stored on the run. A trial that fails fails
the run, with `backup.trial_restore_failed` and the member that failed, because an archive the
product cannot read is not a backup. `notify_on` covers it as it covers any failure.
`backup-restore.md` §5 documents the field and §11's evidence table gains the row; §12's B-4 is
closed with the sentence the header carries; the chart's restore-drill job stays what it is, the
quarterly `NEW_TENANT` drill.

**Acceptance:** a full run with the flag on inspects its own archive and the run carries the
report; a run whose archive was damaged between write and trial fails with the code; the flag is
settable through `PATCH /backup-schedules/{id}` and shown by the listing; the migration leaves
existing schedules off; `hubctl backup-schedule` shows the flag; `backup-restore.md` is current;
`make verify` is green and the contract test passes.

**Read:** `backup-restore.md` §5, §8.2 (`INSPECT`), §11, §12; `core/application/service/backup/Run.go`,
`Restore.go`; F4-02 in `milestone-F4.md` (the schedule's lifecycle); `db/migrations/` (the
backup schedule's table)

---

## P-15 — CI-3 and D-3: the signature at the door **[L]**

*Depends on: nothing.*

Decision 13. The chart renders `templates/imagepolicy.yaml` behind `imagePolicy.enabled`
(default `false`): a Kyverno `ClusterPolicy` with `verifyImages` for `ghcr.io/jersyfi/hubtask`,
keyless, the issuer `https://token.actions.githubusercontent.com` and the subject the release
workflow signs under, `failurePolicy: Fail`, and the chart's README says what it needs (Kyverno
installed, cluster scope) and what it refuses when it is on. `.github/workflows/deploy.yml`'s
`helm upgrade` step is preceded by one that verifies the image
digest with `cosign verify` under the same identity before the upgrade and stops on a failure;
the workflow pins `cosign-installer` as the release workflow does. `ci-cd.md` §8's CI-3 is
closed, and `deployment.md` §12's D-3 is closed for the integration environment with decision
13's sentence.

**Acceptance:** `helm template` with the policy on renders the `ClusterPolicy` with the identity
and off renders nothing; `make gate-chart` is green; the deploy
workflow verifies before upgrading and `actionlint` is green; `gate-selftest` proves the chart
check catches a policy without a subject; both open-point rows are closed; `make gate-docs` is
green.

**Read:** `.github/workflows/release.yml` (the signing identity), `deploy.yml`;
`k8s/templates/`, `k8s/README.md`; `ci-cd.md` §8; `deployment.md` §12; ADR-0046

---

## P-16 — O-2: the capacity model **[L]**

*Depends on: nothing.*

Decision 14. `observability-reliability.md` gains §13 *Capacity*: from RT-6's overload run
(`docs/evidence/RT-6-2026-09-02.md`), O-1's ramp (`O-1-2026-09-01.md`) and the nightly's stored
baseline, a table of items per tenant against the four resources the runs measured — resident
memory per `api` and `worker` process, database connections, storage per item with and without
media, and the request rate at which shedding engaged — with the knee each run showed and the
configuration a provider sets per size (`HUBTASK_DB_POOL_*`, the HPA's targets, the shedder's
thresholds, the partition horizon). Each number names its run; a number no run produced is
written as *not measured* rather than estimated. The chart's `values-*.yaml` sizes are checked
against the table and corrected where they disagree; §12's O-2 is closed.

**Acceptance:** §13 exists with its provenance column; every value names an evidence file or is
marked unmeasured; the chart's size presets agree with it; `make gate-docs` is green.

**Read:** `observability-reliability.md` §3, §7, §12; the three evidence files; `k8s/values*.yaml`;
H-11 in `milestone-0.6.0.md` (no published figure — this section is internal like the runs it
reads, by the owner's decision of 2026-08-21)

---

## P-17 — The ecosystem walked, and the documents current **[L]**

*Depends on: everything above.*

The walk, filed as `docs/evidence/ECO-<date>.md` in the shape the others use: Reminders on macOS
subscribed to a CalDAV calendar of the integration environment's `demo` workspace, a todo ticked
there and read back through the API, one redated, one made; a real Trello export imported and
its counts compared; each SDK's example run against the same workspace; the n8n node loaded into
`n8nio/n8n` in Docker against the Compose stack, a workflow created from a trigger and an action,
and an entry created by it; the Zapier app validated as far as it can be without the CLI (the
schema test) and the fact stated. Every defect the walk finds is an issue, never a fix in this
pull request.

Then the documents: the roadmap's `0.9.0` row rewritten to what this file built; the seven
open-point rows checked closed (SY-C, A-2, B-4, O-2, CI-3, D-3 — and `ai-first.md` §2's template
row); `automation.md` §3.3's "milestone 0.7+" and its last paragraph; `backup-restore.md` §9;
arc42 §5's "CalDAV later" and §7.1's interfaces table; the `0.5.0` backlog's ingestion line, with
a note that it was corrected here; `project-structure.md` §1 for `sdk/`, `tools/sdkgen` and the
two connector packages; `docs/marketing/website-1.0.md` §5.2's list, marking what is now true
and what still waits on a publication; `hubctl`'s README for the new verbs; and `roadmap.md`'s
"`0.9.0` is built" paragraph in the shape `0.8.5`'s has.

**Acceptance:** the evidence file exists with every step and its result; every gap has an issue
with a label; the roadmap row is rewritten and the paragraph written; every open-point row above
reads closed and names its task; `make gate-docs` is green; no code change is in this pull request.

**Read:** `docs/evidence/README.md`, `SY-2026-09-16.md` (the shape); `docs/roadmap.md`;
`deploy/integration/README.md`; every subject document named above

---

## The order at a glance

```
P-01 ──┬── P-04 ── P-05 ─────────┐
       │                         │
P-02 ──┴── P-03 ─────────────────┤
P-06 ── P-07 ────────────────────┤
P-08 ──┬── P-09 ─────────────────┼── P-17
       └── P-10 ─────────────────┤
P-11, P-12, P-13, P-14, P-15, P-16 ┘
```

Ten tasks depend on nothing and can start at once: P-01, P-02, P-06, P-08 and the six answers
to parked points, P-11…P-16. P-02 and P-04 each carry a proposed ADR — written with this backlog — that waits on the owner
rather than on code, and neither blocks its own implementation. P-17 is last by definition.

**Definition of Done for the milestone:** the reference is rendered from the document on the
website, one page per tag, with zero scripts; three SDKs exist, are generated on `make generate`,
carry an example each that runs against the product, and ADR-0057 has put their licence to the
owner; the n8n node and the Zapier app are generated from the document, complete by test, and
ADR-0058 has recorded their shape; a calendar client subscribes over CalDAV, reads todos and
writes completions, dates and names back through the use cases; a CSV, a Trello board, a Google
Tasks export and a Microsoft To Do dump land as collections under a hub through one ingestion
path, idempotently; a template is generated from a description and accepted into a real one; a
device receives its initial synchronisation as one stream; the audit chain is anchored daily
outside the database and `:verify` reads the anchor back; every full backup is inspected by the
code that would restore it; the chart can enforce the image's signature and the integration
deploy does; the capacity model is written from the runs; every parked `0.9.0` row is closed;
`make verify` is green, `make generate` produces no diff; and the walk is filed.
