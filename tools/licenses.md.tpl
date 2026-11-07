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
| [IBM Plex Sans Arabic](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Sans Hebrew](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Sans Devanagari](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Sans Thai](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [IBM Plex Sans SC](https://github.com/IBM/plex/blob/master/LICENSE.txt) | OFL-1.1 |
| [Noto Sans Bengali](https://github.com/notofonts/bengali/blob/main/OFL.txt) (variable) | OFL-1.1 |
| [Lucide](https://github.com/lucide-icons/lucide/blob/main/LICENSE) (the declared subset) | ISC |

The font files come from the packages the pnpm lockfile pins: the `@fontsource` packages, which
repackage the upstream release and are themselves MIT, and for IBM Plex Sans SC IBM's own
`@ibm/plex-sans-sc`, which is OFL itself. Every typeface stays under the SIL Open Font License 1.1,
and each one's text with its own copyright line is under `third-party/licenses/_bundled/`. OFL is copyleft for *fonts* - a modified font must keep the licence and change
its name - and it places no condition whatever on software that merely embeds or displays one, so
it does not touch the terms the paragraph above is about.

The icons come from `lucide-static`, which is a *development* dependency: `packages/design-system/build/icons.js`
generates the declared subset into the repository, so what ships is those shapes and not the
package. ISC is permissive with one obligation - keep the notice, which is this row. Lucide is
itself a fork of Feather (MIT), and its LICENSE carries both notices.

## Libraries bundled into the web app

Not linked into the binary, but shipped inside the web bundle it serves. The list grows with each
library a step accepts; each notice is under `third-party/licenses/_bundled/`.

| Library | Licence | For |
|---|---|---|
| [@marcbachmann/cel-js](https://github.com/marcbachmann/cel-js/blob/main/LICENSE) 8.0.0 | MIT | The browser's evaluator of forms and calculated fields offline (`packages/expression`) |
| [KaTeX](https://github.com/KaTeX/KaTeX/blob/main/LICENSE) 0.19.0 | MIT | A page's formulas, drawn as MathML (`packages/editor`) |
| [Mermaid](https://github.com/mermaid-js/mermaid/blob/develop/LICENSE) 12.2.0 | MIT | A page's diagrams (`packages/editor`); it brings its own KaTeX 0.18.11 (MIT) |
| [lowlight](https://github.com/wooorm/lowlight/blob/main/license) 3.3.0 | MIT | The colours of a page's code blocks (`packages/editor`) |
| [highlight.js](https://github.com/highlightjs/highlight.js/blob/main/LICENSE) 11.11.2 | BSD-3-Clause | The grammars lowlight colours with |
| [emojibase-data](https://github.com/milesj/emojibase/blob/master/LICENSE) 18.0.0 | MIT | The page editor's emoji picker, in the reader's language (`apps/webapp`) |
| [TipTap](https://github.com/ueberdosis/tiptap/blob/main/LICENSE.md) 3.32.0, with @tiptap/y-tiptap 3.0.10 | MIT | The page editor: its core, starter kit, lists, tables and collaboration extensions (`packages/editor`) |
| [ProseMirror](https://github.com/ProseMirror/prosemirror/blob/master/LICENSE) (its modules, re-exported by @tiptap/pm 3.32.0) | MIT | TipTap's document model, state and view |
| [Yjs](https://github.com/yjs/yjs/blob/main/LICENSE) 13.6.33 | MIT | The shared document of a page and a whiteboard, edited together and offline (`packages/editor`, `packages/whiteboard`) |
| [y-protocols](https://github.com/yjs/y-protocols/blob/master/LICENSE) 1.0.7 | MIT | The live editor's sync and awareness messages (`packages/editor`) |
| [lib0](https://github.com/dmonad/lib0/blob/main/LICENSE) 0.2.119 | MIT | The binary encoding of those messages, Yjs's own as well (`packages/editor`) |
| [Chevrotain](https://github.com/Chevrotain/chevrotain/blob/master/LICENSE.txt) 13.2.0 | Apache-2.0 | Mermaid's parser |
| [DOMPurify](https://github.com/cure53/DOMPurify/blob/main/LICENSE) 3.4.17 | Apache-2.0 (chosen of MPL-2.0 OR Apache-2.0) | Mermaid's sanitiser of a diagram's text |
| [D3](https://github.com/d3/d3/blob/main/LICENSE) 3 (its modules) | ISC; BSD-3-Clause for d3-array 2, d3-path 1, d3-shape 1, d3-sankey | Mermaid's drawing |
| [khroma](https://github.com/fabiospampinato/khroma/blob/master/license) 2.1.0 | MIT (its licence file; the package names none) | Mermaid's colour arithmetic |
| [elkjs](https://github.com/kieler/elkjs/blob/master/LICENSE.md) 0.9.3 | EPL-2.0 | Mermaid's ELK layout, a chunk loaded only by a diagram that asks for that layout. Weak copyleft on elkjs's own files, shipped unmodified; it places no condition on the code around it |

The other parts these libraries bring (Cytoscape, dagre-d3-es, roughjs, marked, lodash-es, dayjs,
stylis and smaller ones) are MIT; the pnpm lockfile pins each. The notice of every library in
the table is under `third-party/licenses/_bundled/`.

## Brand marks

The sign-in buttons carry the mark of a provider a workspace can pick as a preset. A mark is not
ours and is not licensed to us under any of the terms above: its owner's guideline permits it for
a sign-in button, and only in the form the row names. Each is drawn inline from the bundle, in the
neutral button every one of these guidelines allows
([ADR-0069](./docs/adr/ADR-0069-third-party-brand-marks.md), `docs/design/design-system.md` §12).
`make gate-licenses` refuses a mark drawn in `apps/webapp/src/lib/signin/ProviderMark.svelte`
without a row here, a row without a mark, and a mark for a preset the server does not have.

| Mark | Preset | Source and guideline | Permitted use |
|---|---|---|---|
| Google "G", from `Android + Web/SVG/Light/Theme=Light, Show text=No, Shape=Square, Platform=Android+Web.svg` in [`signin-assets.zip`](https://developers.google.com/static/identity/images/signin-assets.zip) | `GOOGLE` | [Sign in with Google branding guidelines](https://developers.google.com/identity/branding-guidelines) | The standard "G", unaltered and in its own colours, at the start of a neutral sign-in button |
| Microsoft logo, [`ms-symbollockup_mssymbol_19.svg`](https://learn.microsoft.com/en-us/entra/identity-platform/media/howto-add-branding-in-apps/ms-symbollockup_mssymbol_19.svg) | `MICROSOFT` | [Microsoft identity platform branding guidelines](https://learn.microsoft.com/en-us/entra/identity-platform/howto-add-branding-in-apps) | The four-square logo, unaltered and in its own colours, at the start of a neutral sign-in button |


## Linked dependencies

| Dependency | Licence |
|---|---|
{{ range . }}{{ if and (ne .Name "github.com/Jersyfi/hubtask") (ne .Name "github.com/Jersyfi/hubtask/sdk/go") }}| [{{ .Name }}](https://pkg.go.dev/{{ .Name }}?tab=licenses) | {{ .LicenseName }} |
{{ end }}{{ end }}
