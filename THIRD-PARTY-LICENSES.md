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
| [cel.dev/cel-go](https://pkg.go.dev/cel.dev/cel-go?tab=licenses) | Apache-2.0 |
| [cel.dev/expr](https://pkg.go.dev/cel.dev/expr?tab=licenses) | Apache-2.0 |
| [github.com/Azure/go-ntlmssp](https://pkg.go.dev/github.com/Azure/go-ntlmssp?tab=licenses) | MIT |
| [github.com/antlr4-go/antlr/v4](https://pkg.go.dev/github.com/antlr4-go/antlr/v4?tab=licenses) | BSD-3-Clause |
| [github.com/apapsch/go-jsonmerge/v2](https://pkg.go.dev/github.com/apapsch/go-jsonmerge/v2?tab=licenses) | MIT |
| [github.com/beevik/etree](https://pkg.go.dev/github.com/beevik/etree?tab=licenses) | BSD-2-Clause |
| [github.com/beorn7/perks/quantile](https://pkg.go.dev/github.com/beorn7/perks/quantile?tab=licenses) | MIT |
| [github.com/cenkalti/backoff/v5](https://pkg.go.dev/github.com/cenkalti/backoff/v5?tab=licenses) | MIT |
| [github.com/cespare/xxhash/v2](https://pkg.go.dev/github.com/cespare/xxhash/v2?tab=licenses) | MIT |
| [github.com/coder/websocket](https://pkg.go.dev/github.com/coder/websocket?tab=licenses) | ISC |
| [github.com/coreos/go-oidc/v3/oidc](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc?tab=licenses) | Apache-2.0 |
| [github.com/go-asn1-ber/asn1-ber](https://pkg.go.dev/github.com/go-asn1-ber/asn1-ber?tab=licenses) | MIT |
| [github.com/go-jose/go-jose/v4](https://pkg.go.dev/github.com/go-jose/go-jose/v4?tab=licenses) | Apache-2.0 |
| [github.com/go-jose/go-jose/v4/json](https://pkg.go.dev/github.com/go-jose/go-jose/v4/json?tab=licenses) | BSD-3-Clause |
| [github.com/go-ldap/ldap/v3](https://pkg.go.dev/github.com/go-ldap/ldap/v3?tab=licenses) | MIT |
| [github.com/go-logr/logr](https://pkg.go.dev/github.com/go-logr/logr?tab=licenses) | Apache-2.0 |
| [github.com/go-logr/stdr](https://pkg.go.dev/github.com/go-logr/stdr?tab=licenses) | Apache-2.0 |
| [github.com/google/uuid](https://pkg.go.dev/github.com/google/uuid?tab=licenses) | BSD-3-Clause |
| [github.com/grpc-ecosystem/grpc-gateway/v2](https://pkg.go.dev/github.com/grpc-ecosystem/grpc-gateway/v2?tab=licenses) | BSD-3-Clause |
| [github.com/jackc/pgpassfile](https://pkg.go.dev/github.com/jackc/pgpassfile?tab=licenses) | MIT |
| [github.com/jackc/pgservicefile](https://pkg.go.dev/github.com/jackc/pgservicefile?tab=licenses) | MIT |
| [github.com/jackc/pgx/v5](https://pkg.go.dev/github.com/jackc/pgx/v5?tab=licenses) | MIT |
| [github.com/jackc/puddle/v2](https://pkg.go.dev/github.com/jackc/puddle/v2?tab=licenses) | MIT |
| [github.com/jonboulle/clockwork](https://pkg.go.dev/github.com/jonboulle/clockwork?tab=licenses) | Apache-2.0 |
| [github.com/klauspost/compress](https://pkg.go.dev/github.com/klauspost/compress?tab=licenses) | Apache-2.0 |
| [github.com/mfridman/interpolate](https://pkg.go.dev/github.com/mfridman/interpolate?tab=licenses) | MIT |
| [github.com/munnerz/goautoneg](https://pkg.go.dev/github.com/munnerz/goautoneg?tab=licenses) | BSD-3-Clause |
| [github.com/nats-io/nats.go](https://pkg.go.dev/github.com/nats-io/nats.go?tab=licenses) | Apache-2.0 |
| [github.com/nats-io/nkeys](https://pkg.go.dev/github.com/nats-io/nkeys?tab=licenses) | Apache-2.0 |
| [github.com/nats-io/nuid](https://pkg.go.dev/github.com/nats-io/nuid?tab=licenses) | Apache-2.0 |
| [github.com/oapi-codegen/runtime](https://pkg.go.dev/github.com/oapi-codegen/runtime?tab=licenses) | Apache-2.0 |
| [github.com/pressly/goose/v3](https://pkg.go.dev/github.com/pressly/goose/v3?tab=licenses) | MIT |
| [github.com/prometheus/client_golang/internal/github.com/golang/gddo/httputil](https://pkg.go.dev/github.com/prometheus/client_golang/internal/github.com/golang/gddo/httputil?tab=licenses) | BSD-3-Clause |
| [github.com/prometheus/client_golang/prometheus](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus?tab=licenses) | Apache-2.0 |
| [github.com/prometheus/client_model/go](https://pkg.go.dev/github.com/prometheus/client_model/go?tab=licenses) | Apache-2.0 |
| [github.com/prometheus/common](https://pkg.go.dev/github.com/prometheus/common?tab=licenses) | Apache-2.0 |
| [github.com/prometheus/otlptranslator](https://pkg.go.dev/github.com/prometheus/otlptranslator?tab=licenses) | Apache-2.0 |
| [github.com/prometheus/procfs](https://pkg.go.dev/github.com/prometheus/procfs?tab=licenses) | Apache-2.0 |
| [github.com/russellhaering/goxmldsig](https://pkg.go.dev/github.com/russellhaering/goxmldsig?tab=licenses) | Apache-2.0 |
| [github.com/sethvargo/go-retry](https://pkg.go.dev/github.com/sethvargo/go-retry?tab=licenses) | Apache-2.0 |
| [github.com/teambition/rrule-go](https://pkg.go.dev/github.com/teambition/rrule-go?tab=licenses) | MIT |
| [go.opentelemetry.io/auto/sdk](https://pkg.go.dev/go.opentelemetry.io/auto/sdk?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel](https://pkg.go.dev/go.opentelemetry.io/otel?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/prometheus](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/prometheus?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/log](https://pkg.go.dev/go.opentelemetry.io/otel/log?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/metric](https://pkg.go.dev/go.opentelemetry.io/otel/metric?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/sdk](https://pkg.go.dev/go.opentelemetry.io/otel/sdk?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/sdk/metric](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/trace](https://pkg.go.dev/go.opentelemetry.io/otel/trace?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/proto/otlp](https://pkg.go.dev/go.opentelemetry.io/proto/otlp?tab=licenses) | Apache-2.0 |
| [go.uber.org/multierr](https://pkg.go.dev/go.uber.org/multierr?tab=licenses) | MIT |
| [go.yaml.in/yaml/v3](https://pkg.go.dev/go.yaml.in/yaml/v3?tab=licenses) | MIT |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto?tab=licenses) | BSD-3-Clause |
| [golang.org/x/exp/slices](https://pkg.go.dev/golang.org/x/exp/slices?tab=licenses) | BSD-3-Clause |
| [golang.org/x/net](https://pkg.go.dev/golang.org/x/net?tab=licenses) | BSD-3-Clause |
| [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2?tab=licenses) | BSD-3-Clause |
| [golang.org/x/sync](https://pkg.go.dev/golang.org/x/sync?tab=licenses) | BSD-3-Clause |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys?tab=licenses) | BSD-3-Clause |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text?tab=licenses) | BSD-3-Clause |
| [google.golang.org/genproto/googleapis/api](https://pkg.go.dev/google.golang.org/genproto/googleapis/api?tab=licenses) | Apache-2.0 |
| [google.golang.org/genproto/googleapis/rpc/status](https://pkg.go.dev/google.golang.org/genproto/googleapis/rpc/status?tab=licenses) | Apache-2.0 |
| [google.golang.org/grpc](https://pkg.go.dev/google.golang.org/grpc?tab=licenses) | Apache-2.0 |
| [google.golang.org/protobuf](https://pkg.go.dev/google.golang.org/protobuf?tab=licenses) | BSD-3-Clause |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3?tab=licenses) | MIT |

