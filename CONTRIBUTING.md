# Contributing

Thanks for your interest. This project has a fully documented architecture — that is not an
accident, it is the basis you work from.

## Before you write code

1. Read `CLAUDE.md`. The thirteen rules it lists apply to humans just the same.
2. Read the document under `docs/architecture/` that matches what you are doing, and the ADRs it
   links to.
3. Open an issue before starting anything substantial. For architectural changes open an ADR
   issue, not a pull request presenting a decision as already made.

## The loop

```bash
make tools
make db-up && make migrate
# work
make verify          # must be green
```

Branch names: `feat/short-description`, `fix/…`, `docs/…`, `chore/…`.

**Conventional Commits**, in English:

```
feat(workmanagement): add container archiving

Archived containers remain restorable indefinitely (F-10).
Refs #42
```

Scopes correspond to the bounded contexts in `docs/architecture/arc42.md` §5.

## What makes a pull request acceptable

The template lists the Definition of Done. Two items are missed most often:

* **A cross-tenant negative test** for every new repository method. Without it, gate SG-3 fails.
* **A merge rule** for every new field on `WorkItem` (LWW, OR-set, fractional index, or
  server-side) — otherwise the behaviour on offline conflicts is undefined.

## Language

Everything is in English: documentation, code, identifiers, code comments, commit titles, and
commit bodies. Message codes and `locales/en.json` are the source for translations — the backend
never contains display text.

## Licence and CLA

Contributions are published under the project licence ([LICENSE](LICENSE), BSL 1.1, converting to
Apache-2.0 three years after each release). Contributors sign a
[Contributor License Agreement](CLA.md) on their first pull request; a bot posts the link
automatically. It exists because the conversion to Apache-2.0 and the sale of commercial licences
both require the Licensor to hold sufficient rights in the whole codebase. You keep full ownership
of your work. The reasoning, and what it costs you, is in [ADR-0013](docs/adr/ADR-0013-licensing.md).

## Security

Please do not report vulnerabilities as issues — see [SECURITY.md](SECURITY.md).
