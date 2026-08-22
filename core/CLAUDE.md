# core/ — the domain and the application layer

This is the part of the product that would still be true if HTTP, PostgreSQL and the browser were
all replaced tomorrow. Everything else in the repository exists to carry it somewhere.

## What must not happen here

* **No third-party import in `core/domain` and `core/port`**, and nothing from `infrastructure/`
  or `presentation/`. Dependencies point inwards (rule 1, ADR-0001).
* **No `net/http`, no `database/sql`, no `encoding/json` tags on domain types.** A domain type
  that knows how it is serialised is a domain type shaped by its adapter.
* **No `time.Now()`, no `math/rand`, no UUID generation.** Only the `Clock`, `RandomSource` and
  `IDGenerator` ports (rule 4). A test that cannot fix the time is a test that will flake.
* **No display text.** Message codes plus parameters (rule 8, ADR-0011). The same argument covers
  colour: `Label` and `cover` store a `colorToken`, never a hex value.
* **No authorisation in an adapter, and none skipped here.** Whether an actor may do something is
  decided in the application layer, always (rule 2, ADR-0005).
* **No bare `go` statement.** Concurrency only through `core/shared/concurrency.SafeGo` (rule 5).
* **`core/` does not learn that a frontend exists.** The web UI is an adapter in
  `presentation/webui`; nothing here references it (ADR-0028).

## The one generated file

`core/domain/model/shared/LabelTokens.go` is generated from
`packages/design-system/tokens/tokens.json` by `make tokens`, and it is **committed**. Never edit
it: CI regenerates it and fails on any difference.

It carries the ten label token *names* and no colour value, which is the point — the core keeps
the vocabulary `domain-model.md` §4 asks it to validate while staying unable to say what any of it
looks like ([ADR-0029](../docs/adr/ADR-0029-design-system-tokens.md)).

## How to check a change

```bash
make gate-unit          # domain: table tests, no mocks. application: fakes of the ports
make gate-architecture  # layer boundaries, the goroutine ban, mandatory authorisation, parity
make verify             # before the pull request leaves draft
```

A new use case is not finished when it compiles. It is finished when it is in
`core/application/usecase.Registry` — that is what makes it available over REST, MCP *and*
automation, and the parity test fails if the three disagree.
