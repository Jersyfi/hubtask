# Third-party licences

Hubtask itself is licensed under BUSL-1.1 (see [LICENSE](./LICENSE)) and converts to Apache-2.0
three years after each version's first public distribution ([ADR-0013](./docs/adr/ADR-0013-licensing.md)).

The dependencies below are what the binaries link. None of them carries a copyleft licence, which
is what the `gate-licenses` build gate enforces on every pull request: a GPL or AGPL dependency
would make both the commercial licence and the promised conversion impossible.

This file is generated - run `make licenses` rather than editing it.

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
it does not touch the conversion the paragraph above is about.

The icons come from `lucide-static`, which is a *development* dependency: `packages/design-system/build/icons.js`
generates the declared subset into the repository, so what ships is those shapes and not the
package. ISC is permissive with one obligation - keep the notice, which is this row. Lucide is
itself a fork of Feather (MIT), and its LICENSE carries both notices.


## Linked dependencies

| Dependency | Licence |
|---|---|
| [cel.dev/cel-go](https://pkg.go.dev/cel.dev/cel-go?tab=licenses) | Apache-2.0 |
| [cel.dev/expr](https://pkg.go.dev/cel.dev/expr?tab=licenses) | Apache-2.0 |
| [github.com/antlr4-go/antlr/v4](https://pkg.go.dev/github.com/antlr4-go/antlr/v4?tab=licenses) | BSD-3-Clause |
| [github.com/apapsch/go-jsonmerge/v2](https://pkg.go.dev/github.com/apapsch/go-jsonmerge/v2?tab=licenses) | MIT |
| [github.com/beorn7/perks/quantile](https://pkg.go.dev/github.com/beorn7/perks/quantile?tab=licenses) | MIT |
| [github.com/cenkalti/backoff/v5](https://pkg.go.dev/github.com/cenkalti/backoff/v5?tab=licenses) | MIT |
| [github.com/cespare/xxhash/v2](https://pkg.go.dev/github.com/cespare/xxhash/v2?tab=licenses) | MIT |
| [github.com/coreos/go-oidc/v3/oidc](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc?tab=licenses) | Apache-2.0 |
| [github.com/go-jose/go-jose/v4](https://pkg.go.dev/github.com/go-jose/go-jose/v4?tab=licenses) | Apache-2.0 |
| [github.com/go-jose/go-jose/v4/json](https://pkg.go.dev/github.com/go-jose/go-jose/v4/json?tab=licenses) | BSD-3-Clause |
| [github.com/go-logr/logr](https://pkg.go.dev/github.com/go-logr/logr?tab=licenses) | Apache-2.0 |
| [github.com/go-logr/stdr](https://pkg.go.dev/github.com/go-logr/stdr?tab=licenses) | Apache-2.0 |
| [github.com/google/uuid](https://pkg.go.dev/github.com/google/uuid?tab=licenses) | BSD-3-Clause |
| [github.com/grpc-ecosystem/grpc-gateway/v2](https://pkg.go.dev/github.com/grpc-ecosystem/grpc-gateway/v2?tab=licenses) | BSD-3-Clause |
| [github.com/jackc/pgpassfile](https://pkg.go.dev/github.com/jackc/pgpassfile?tab=licenses) | MIT |
| [github.com/jackc/pgservicefile](https://pkg.go.dev/github.com/jackc/pgservicefile?tab=licenses) | MIT |
| [github.com/jackc/pgx/v5](https://pkg.go.dev/github.com/jackc/pgx/v5?tab=licenses) | MIT |
| [github.com/jackc/puddle/v2](https://pkg.go.dev/github.com/jackc/puddle/v2?tab=licenses) | MIT |
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
| [github.com/sethvargo/go-retry](https://pkg.go.dev/github.com/sethvargo/go-retry?tab=licenses) | Apache-2.0 |
| [github.com/teambition/rrule-go](https://pkg.go.dev/github.com/teambition/rrule-go?tab=licenses) | MIT |
| [go.opentelemetry.io/auto/sdk](https://pkg.go.dev/go.opentelemetry.io/auto/sdk?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel](https://pkg.go.dev/go.opentelemetry.io/otel?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp?tab=licenses) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/prometheus](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/prometheus?tab=licenses) | Apache-2.0 |
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

