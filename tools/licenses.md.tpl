{{- /* The template for THIRD-PARTY-LICENSES.md (make licenses, ADR-0080).

It lists what the binary links, with the licence each dependency carries and a link to the text.
The point is not ceremony: the binary is Apache-2.0, and a copyleft dependency would change the
terms on which it may be passed on - so the list is also the record that none of them does.

The link is **derived from the module path** rather than taken from `{{ .LicenseURL }}`, and that
is the difference between a generated file that can be checked and one that cannot.
`go-licenses` resolves that URL over the network while it writes, and emits `Unknown` when the
lookup fails - so two runs of the same command produced two different files, and the gate that
compares the result byte for byte went red on pull requests that touched no Go file at all. Every
other field here comes from the module graph and is deterministic; this one was the only reason
the file was not. `pkg.go.dev` shows the licence text for any public module, needs no lookup to
address, and survives a repository move, which the previous URL did not. */ -}}
# Third-party licences

Hubtask itself is licensed under the Apache License 2.0 (see [LICENSE](./LICENSE) and
[ADR-0080](./docs/adr/ADR-0080-hubtask-is-apache-2-0.md)).

The dependencies below are what the binaries link. None of them carries a copyleft licence, which
is what the `gate-licenses` build gate enforces on every pull request: a GPL or AGPL dependency
would change the terms on which an Apache-2.0 binary may be passed on.

This file is generated - run `make licenses` rather than editing it. The Go SDK under `sdk/go`
carries a LICENSE file of its own ([ADR-0057](./docs/adr/ADR-0057-sdk-licence-and-extraction.md)),
which `go-licenses` reads as a second licensed unit of this module; it is this project's own
code, not a dependency, and is left out of the table below.

## Bundled assets

Not linked into the binary, but shipped with it: the typeface the interface is set in, and the
icons it is drawn with. Both are served from this repository rather than from a foreign domain, so
that a self-hosted Hubtask contacts nobody when it loads
([ADR-0029](./docs/adr/ADR-0029-design-system-tokens.md),
`docs/design/design-system.md` §3, [ADR-0041](./docs/adr/ADR-0041-icon-set.md)).

| Asset | Licence |
|---|---|
| [IBM Plex Sans](https://github.com/IBM/plex/blob/master/LICENSE.txt) (variable) | OFL-1.1 |
| [IBM Plex Sans Condensed](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Mono](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [Lucide](https://github.com/lucide-icons/lucide/blob/main/LICENSE) (the declared subset) | ISC |

The font files come from the `@fontsource` packages the pnpm lockfile pins; those packages
repackage the upstream release and are themselves MIT, while the typeface stays under the SIL Open
Font License 1.1. OFL is copyleft for *fonts* - a modified font must keep the licence and change
its name - and it places no condition whatever on software that merely embeds or displays one, so
it does not touch the terms the paragraph above is about.

The icons come from `lucide-static`, which is a *development* dependency: `packages/design-system/build/icons.js`
generates the declared subset into the repository, so what ships is those shapes and not the
package. ISC is permissive with one obligation - keep the notice, which is this row. Lucide is
itself a fork of Feather (MIT), and its LICENSE carries both notices.


## Linked dependencies

| Dependency | Licence |
|---|---|
{{ range . }}{{ if and (ne .Name "github.com/Jersyfi/hubtask") (ne .Name "github.com/Jersyfi/hubtask/sdk/go") }}| [{{ .Name }}](https://pkg.go.dev/{{ .Name }}?tab=licenses) | {{ .LicenseName }} |
{{ end }}{{ end }}
