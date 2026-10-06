# presentation/ — the inbound adapters

Every way into the product: `rest`, `mcp`, `stream`, `calendar`, `intake`, `worker` and `webui`,
plus `openapi`, the code generated from the contract.

## What must not happen here

* **No business logic.** An adapter translates and dispatches; a rule written here is missing from
  the other adapters.
* **No authorisation decision** (rule 2). An adapter may authenticate — validate a bearer, verify a
  signed token — and decides nothing after that. A value travels to the use case as it arrived;
  coercing it here would be deciding a rule.
* **No import from `infrastructure/`.** What an adapter needs from it arrives as an interface
  declared here and wired in `cmd/`.
* **No hand edit under `presentation/openapi`** (rule 11): change `api/openapi.yaml`, run
  `make generate`, then implement.
* **No display text** (rule 8). An error is an RFC 9457 problem with a stable `code` and a message
  code, never a sentence (`api-guidelines.md`).
* **No route the contract does not declare.** `/mcp` and the web UI are the two documented
  exceptions.
* **`webui` reaches nothing** — no actor, no tenant, no transaction. `dist/index.html` is a
  committed placeholder so that `go build ./...` works without Node; everything else under `dist/`
  is ignored. `/api/*` is never shadowed: `rest.Fallback` gives the API every path it owns and the
  interface what is left. The UI's content security policy has no `'unsafe-inline'` and no
  `'unsafe-eval'` (`project-structure.md` §7, `security.md` §9).

## How to check a change

```bash
make gate-unit
make gate-architecture   # includes the use case parity check across REST, MCP and automation
make gate-e2e            # a person's first hour through hubctl, against a real stack
```

A use case reaches this layer through the registry, not through a controller method somebody
remembered to write.
