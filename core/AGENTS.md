# core/ — the domain and the application layer

The part of the product that would still be true if HTTP, PostgreSQL and the browser were replaced
tomorrow. `core/domain` holds the model, `core/application` the use cases and the one place
authorisation happens, `core/port` the interfaces the adapters implement.

## What must not happen here

* **No third-party import in `core/domain` or `core/port`**, and nothing from `infrastructure/` or
  `presentation/` anywhere in `core/` (rule 1). `[gate: gate-architecture, gate-quick]`
* **No adapter's shape on a domain type:** no `net/http`, no `database/sql`, no `encoding/json`
  tags. A domain type that knows how it is serialised is shaped by its adapter.
  `[partial: gate-architecture, gate-quick; open: a json struct tag, which needs no import]`
* **No `time.Now()`, `math/rand` or UUID generation** — only the `Clock`, `RandomSource` and
  `IDGenerator` ports (rule 4).
  `[partial: gate-architecture, gate-quick; open: UUID generation and crypto/rand in core/application, an aliased import]`
* **No display text, and no colour.** Message codes plus parameters (rule 8); `Label` and `cover`
  store a `colorToken`, never a hex value.
  `[partial: gate-architecture; open: prose inside an error string, a colour value in Go]`
* **No use case without its authorisation check.** Adapters authenticate; whether the actor may
  proceed is decided here (rule 2). `[partial: gate-security; open: a use case that never asks]`
* **No bare `go` statement** — `core/shared/concurrency.SafeGo` (rule 5).
  `[gate: gate-architecture]`
* **Nothing that knows a frontend exists** (rule 14).
  `[partial: gate-architecture, gate-quick; open: frontend knowledge without an import]`
* **No hand edit of `core/domain/model/shared/LabelTokens.go`.** It is generated from
  `packages/design-system/tokens/tokens.json` by `make tokens` and committed; it carries the ten
  label token names and no colour value. CI regenerates it and fails on a difference.
  `[gate: ci:tokens-drift]`

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
