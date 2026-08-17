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

| Dependency | Licence |
|---|---|
{{ range . }}{{ if ne .Name "github.com/Jersyfi/hubtask" }}| [{{ .Name }}]({{ .LicenseURL }}) | {{ .LicenseName }} |
{{ end }}{{ end }}
