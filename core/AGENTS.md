# core/ — the domain and the application layer

The part of the product that would still be true if HTTP, PostgreSQL and the browser were replaced
tomorrow. `core/domain` holds the model, `core/application` the use cases and the one place
authorisation happens, `core/port` the interfaces the adapters implement.

## What must not happen here

* **No third-party import in `core/domain` or `core/port`**, and nothing from `infrastructure/` or
  `presentation/` anywhere in `core/` (rule 1).
* **No adapter's shape on a domain type:** no `net/http`, no `database/sql`, no `encoding/json`
  tags. A domain type that knows how it is serialised is shaped by its adapter.
* **No `time.Now()`, `math/rand` or UUID generation** — only the `Clock`, `RandomSource` and
  `IDGenerator` ports (rule 4).
* **No display text, and no colour.** Message codes plus parameters (rule 8); `Label` and `cover`
  store a `colorToken`, never a hex value.
* **No use case without its authorisation check.** Adapters authenticate; whether the actor may
  proceed is decided here (rule 2).
* **No bare `go` statement** — `core/shared/concurrency.SafeGo` (rule 5).
* **Nothing that knows a frontend exists** (rule 14).
* **No hand edit of `core/domain/model/shared/LabelTokens.go`.** It is generated from
  `packages/design-system/tokens/tokens.json` by `make tokens` and committed; it carries the ten
  label token names and no colour value. CI regenerates it and fails on a difference.

## How to check a change

```bash
make gate-unit          # domain: table tests, no infrastructure; application: fakes of the ports
make gate-architecture  # layer boundaries, the goroutine ban, mandatory authorisation, parity
make verify             # while working
make verify-pr          # before the pull request leaves draft
```

A use case is finished when it is registered in `core/application/usecase.Registry`, not when it
compiles: the registry is what puts it on REST, MCP and automation, and the parity test fails when
the three disagree.
