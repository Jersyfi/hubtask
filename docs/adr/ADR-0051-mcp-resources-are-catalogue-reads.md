# ADR-0051 — An MCP resource is a catalogue read with a URI for an argument

**Status:** proposed · **Date:** 2026-09-09

## Context

`ai-first.md` §1.1 has promised MCP resources since the file was written — "containers, items, and
views are additionally readable as MCP resources (`hubtask://items/{id}`)" — and
`presentation/mcp/McpServer.go` declares no resource capability on `initialize`, with a comment
saying why: a client that believes in a capability and finds nothing behind it has no way to
recover. `0.7.0` is the milestone that makes good on the promise (J-11).

The tools half of the server was easy, and it is worth being precise about why. `tools/list` is
generated from `core/application/usecase.Registry`: a new use case is a new tool the moment it is
registered, and no code in `presentation/mcp` changes. That property is the reason the agent
interface cannot fall behind the API, which is the failure this project set out to avoid.

**A resource list is not a use case list**, and that is the design question. MCP's resources are
*addressable content*: each has a URI, a name, a MIME type, and is fetched by `resources/read`
without arguments. Use cases are *named operations with typed inputs*. Something has to bridge the
two, and the shape of that bridge decides whether this package keeps its one good property or
acquires a second, hand-maintained list of things an agent can see.

Two shapes were available:

* **A resource registry of its own** — an interface a use case may implement, or a table in
  `presentation/mcp` naming which reads are resources and how their URIs are built. It is the
  obvious shape, and it is the one that falls behind: a read added in `core/application` is a
  resource nobody publishes until somebody remembers this file.
* **A resource is a read that is already in the catalogue**, addressed by URI instead of by
  argument. Then `resources/read` is one more path into `Catalogue.Invoke` and carries no
  authorisation of its own, exactly as `tools/call` carries none.

## Decision

**1. A resource is a catalogue read, and the URI is its argument.** Three shapes, each naming one
use case and one input field:

| URI | Use case | Input |
|---|---|---|
| `hubtask://items/{id}` | `GetWorkItem` | `item_id` |
| `hubtask://containers/{id}` | `GetContainer` | `container_id` |
| `hubtask://views/{id}` | `GetSavedView` | `view_id` |

`resources/read` parses the URI, calls the use case, and renders the result. Nothing else happens
in this package: no permission is checked here, no tenant is resolved here, and no read reaches a
repository except through the application layer (ADR-0005, rule 2).

**2. The table is small, in code, and deliberately not derived.** It is the same decision J-05 made
for the appliers of a suggestion, for the same reason: a *stored* or *derived* mapping would make
"is this read addressable as a resource" a property something outside the catalogue decides.
Deriving it from a naming convention — every `Get*` use case is a resource — would publish
`GetRecurrence` and `GetTemplate` as content, which they are not: they are attributes of something
else. Three entries that a person chose are a smaller thing to keep right than a rule that is wrong
for two of the cases it covers.

The cost is named rather than hidden: **a fourth resource kind is a change to this file.** That is
the opposite of the tools half, and it is the honest trade — a resource is a product decision about
what an agent may browse, and the tools list is not.

**3. `resources/list` publishes what can be enumerated, and items are not in it.** Containers and
saved views have tenant-wide, paged lists in the catalogue (`ListContainers`, `ListSavedViews`);
items do not — `ListWorkItems` requires a collection, and `SearchItems` requires words. An agent
that wants an item asks for one, by URI, or finds it with `search_items` or `query_items`.

That is not a gap. `resources/templates/list` exists precisely so a client can learn the URI shape
of content it cannot enumerate, and enumerating every entry in a workspace is a tenant asking for
everything — the thing every list in this system is paged to prevent (`api-guidelines.md` §4). So
`resources/list` is paged with the same cursor the underlying reads use, and its page size is
bounded by the same ceiling.

**4. A URI naming something the actor may not see is a not-found.** This is the property that would
be easiest to lose and worst to lose. `GetWorkItem` already answers `ErrNotFound` where an entry
belongs to another tenant, because row level security has made "not yours" and "not there" the same
answer (multi-tenancy.md §2); and `resources/read` must not turn that into anything that confirms
existence. A refused read is therefore reported exactly as `tools/call` reports one — a *result*,
not a JSON-RPC error — while an unknown URI **scheme** is a protocol error, because that is the
client having called wrongly rather than the server saying no.

**5. The capability is declared because the methods exist.** `initialize` answers
`resources: {subscribe: false, listChanged: false}`. Both are false and both are honest: this
server initiates nothing (the GET half of the streamable transport is J-13), so it cannot notify a
client that a list changed, and a client that believed otherwise would wait for a message that never
comes.

## Consequences

* An agent can browse a workspace's structure — its hubs, collections and saved views — and read any
  addressable entry, without a second interface to keep in step for anything but the three shapes.
* Every resource read is audited exactly as the same read through REST is, as `AI_AGENT`, by the
  application layer and nowhere else. `GetContainer` and `GetWorkItem` declare their audit as *not
  required*, so an ordinary read writes nothing and a refused one writes a denial — which is the
  behaviour the channel parity test asserts for the new methods.
* Adding a fourth resource kind is a deliberate change here, and `resources/templates/list` is the
  place a client learns the shapes. Both are covered by a test that reads the table rather than a
  copy of it.
* Prompts (J-12) and the streaming half of the transport (J-13) stay unimplemented and undeclared,
  which is the rule `McpServer.go` already states about itself.

## Alternatives considered

**Every `Get*` use case is a resource, derived.** Keeps the generated property of the tools half and
publishes the wrong things: a recurrence rule and a template are attributes of an entry rather than
content an agent browses, and `GetCapabilities` is a manifest. A derivation that needs an opt-out
list is a table with extra steps.

**A `Resource()` method on the descriptor.** Puts the URI shape in `core/application`, which means
`core` learning that MCP exists — the same mistake as `core` learning that a frontend does
(ADR-0028). The descriptor already carries `MCPTool()`, but that is a *name*, derived mechanically
from the use case name and meaningful to any protocol; a URI template is one protocol's addressing
scheme.

**`resources/list` enumerates items too, paged.** Would need an unanchored item list that does not
exist and should not: the one unanchored read of entries in this product is the search, and it is
unanchored because it is narrowed to what the actor may see afterwards, row by row (C-08). A
resource list built on that would be a full table scan of a workspace, per page, for an agent that
has not said what it is looking for.
