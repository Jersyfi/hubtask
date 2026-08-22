{{- /* The template for THIRD-PARTY-LICENSES.md (make licenses, ADR-0013).

It lists what the binary links, with the licence each dependency carries and a link to the text.
The point is not ceremony: BSL 1.1 converts to Apache-2.0 after three years, and a dependency
whose licence forbids that would make the conversion impossible - so the list is also the record
that none of them does. */ -}}
# Third-party licences

Hubtask itself is licensed under BUSL-1.1 (see [LICENSE](./LICENSE)) and converts to Apache-2.0
three years after each version's first public distribution ([ADR-0013](./docs/adr/ADR-0013-licensing.md)).

The dependencies below are what the binaries link. None of them carries a copyleft licence, which
is what the `gate-licenses` build gate enforces on every pull request: a GPL or AGPL dependency
would make both the commercial licence and the promised conversion impossible.

This file is generated - run `make licenses` rather than editing it.

## Bundled assets

Not linked into the binary, but shipped with it: the typeface the interface is set in. It is
served from this repository rather than from Google Fonts, so that a self-hosted Hubtask contacts
no foreign domain when it loads ([ADR-0029](./docs/adr/ADR-0029-design-system-tokens.md),
`docs/design/design-system.md` §3).

| Asset | Licence |
|---|---|
| [IBM Plex Sans](https://github.com/IBM/plex/blob/master/LICENSE.txt) (variable) | OFL-1.1 |
| [IBM Plex Sans Condensed](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Mono](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |

The font files come from the `@fontsource` packages the pnpm lockfile pins; those packages
repackage the upstream release and are themselves MIT, while the typeface stays under the SIL Open
Font License 1.1. OFL is copyleft for *fonts* - a modified font must keep the licence and change
its name - and it places no condition whatever on software that merely embeds or displays one, so
it does not touch the conversion the paragraph above is about.

## Linked dependencies

| Dependency | Licence |
|---|---|
{{ range . }}{{ if ne .Name "github.com/Jersyfi/hubtask" }}| [{{ .Name }}]({{ .LicenseURL }}) | {{ .LicenseName }} |
{{ end }}{{ end }}
