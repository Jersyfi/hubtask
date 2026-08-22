# presentation/ — the inbound adapters

Every way into the product: `rest`, `mcp`, `sse`, `calendar`, `intake`, `worker`, `admin`, and
`webui`. They differ in who is asking, not in what happens next.

## What must not happen here

* **No business logic.** An adapter translates and dispatches. A rule that lives here is a rule
  the other five adapters do not have.
* **No authorisation decision.** That belongs to the application layer, always (rule 2, ADR-0005).
  An adapter may authenticate; it never decides whether the actor may proceed.
* **No hand-edited generated code.** `presentation/openapi` comes from `api/openapi.yaml`
  (rule 11). Change the specification, run `make generate`, then implement.
* **No display text.** Errors are RFC 9457 problems with a stable `code` and a message code —
  never a sentence (ADR-0011, ADR-0025).
* **No route invented here.** The contract declares the routes; `/mcp` and the web UI are the two
  documented exceptions and both are mounted deliberately.

## `webui` is the newest and the thinnest

It serves the embedded bundle and reaches nothing — no actor, no tenant, no transaction. Three
things about it are decisions rather than details ([ADR-0028](../docs/adr/ADR-0028-embedded-web-ui.md)):

* `dist/index.html` is **committed** as a placeholder, so that `//go:embed` resolves and
  `go build ./...` works without Node.js. Everything else under `dist/` is ignored.
* `/api/*` is never shadowed. `rest.Fallback` gives the API every path it owns and the interface
  what is left.
* The UI origin has its own content security policy, with no `'unsafe-inline'` and no
  `'unsafe-eval'`. That is a constraint on the frontend framework, not a consequence of one.

## How to check a change

```bash
make gate-unit
make gate-architecture   # includes the use case parity check across REST, MCP and automation
make gate-e2e            # a person's first hour through hubctl, against a real stack
```

A new use case reaches this layer through the registry, not through a new controller method that
somebody remembered to write. If REST has it and MCP does not, the parity test is what says so.
