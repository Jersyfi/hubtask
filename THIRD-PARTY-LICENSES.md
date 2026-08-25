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
| [github.com/apapsch/go-jsonmerge/v2](https://github.com/apapsch/go-jsonmerge/blob/v2.0.0/LICENSE) | MIT |
| [github.com/beorn7/perks/quantile](https://github.com/beorn7/perks/blob/v1.0.1/LICENSE) | MIT |
| [github.com/cenkalti/backoff/v5](https://github.com/cenkalti/backoff/blob/v5.0.3/LICENSE) | MIT |
| [github.com/cespare/xxhash/v2](https://github.com/cespare/xxhash/blob/v2.3.0/LICENSE.txt) | MIT |
| [github.com/go-logr/logr](https://github.com/go-logr/logr/blob/v1.4.4/LICENSE) | Apache-2.0 |
| [github.com/go-logr/stdr](https://github.com/go-logr/stdr/blob/v1.2.2/LICENSE) | Apache-2.0 |
| [github.com/google/uuid](https://github.com/google/uuid/blob/v1.6.0/LICENSE) | BSD-3-Clause |
| [github.com/grpc-ecosystem/grpc-gateway/v2](https://github.com/grpc-ecosystem/grpc-gateway/blob/v2.29.0/LICENSE) | BSD-3-Clause |
| [github.com/jackc/pgpassfile](https://github.com/jackc/pgpassfile/blob/v1.0.0/LICENSE) | MIT |
| [github.com/jackc/pgservicefile](https://github.com/jackc/pgservicefile/blob/5a60cdf6a761/LICENSE) | MIT |
| [github.com/jackc/pgx/v5](https://github.com/jackc/pgx/blob/v5.10.0/LICENSE) | MIT |
| [github.com/jackc/puddle/v2](https://github.com/jackc/puddle/blob/v2.2.2/LICENSE) | MIT |
| [github.com/mfridman/interpolate](https://github.com/mfridman/interpolate/blob/v0.0.2/LICENSE.txt) | MIT |
| [github.com/munnerz/goautoneg](https://github.com/munnerz/goautoneg/blob/a7dc8b61c822/LICENSE) | BSD-3-Clause |
| [github.com/oapi-codegen/runtime](https://github.com/oapi-codegen/runtime/blob/v1.7.0/LICENSE) | Apache-2.0 |
| [github.com/pressly/goose/v3](https://github.com/pressly/goose/blob/v3.27.3/LICENSE) | MIT |
| [github.com/prometheus/client_golang/internal/github.com/golang/gddo/httputil](https://github.com/prometheus/client_golang/blob/v1.24.1/internal/github.com/golang/gddo/LICENSE) | BSD-3-Clause |
| [github.com/prometheus/client_golang/prometheus](https://github.com/prometheus/client_golang/blob/v1.24.1/LICENSE) | Apache-2.0 |
| [github.com/prometheus/client_model/go](https://github.com/prometheus/client_model/blob/v0.6.2/LICENSE) | Apache-2.0 |
| [github.com/prometheus/common](https://github.com/prometheus/common/blob/v0.70.1/LICENSE) | Apache-2.0 |
| [github.com/prometheus/otlptranslator](https://github.com/prometheus/otlptranslator/blob/v1.0.0/LICENSE) | Apache-2.0 |
| [github.com/prometheus/procfs](https://github.com/prometheus/procfs/blob/v0.21.1/LICENSE) | Apache-2.0 |
| [github.com/sethvargo/go-retry](https://github.com/sethvargo/go-retry/blob/v0.4.0/LICENSE) | Apache-2.0 |
| [github.com/teambition/rrule-go](https://github.com/teambition/rrule-go/blob/v1.8.2/LICENSE) | MIT |
| [go.opentelemetry.io/auto/sdk](https://github.com/open-telemetry/opentelemetry-go-instrumentation/blob/sdk/v1.2.1/sdk/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel](https://github.com/open-telemetry/opentelemetry-go/blob/v1.45.0/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace](https://github.com/open-telemetry/opentelemetry-go/blob/exporters/otlp/otlptrace/v1.45.0/exporters/otlp/otlptrace/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp](https://github.com/open-telemetry/opentelemetry-go/blob/exporters/otlp/otlptrace/otlptracehttp/v1.45.0/exporters/otlp/otlptrace/otlptracehttp/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/exporters/prometheus](https://github.com/open-telemetry/opentelemetry-go/blob/exporters/prometheus/v0.67.0/exporters/prometheus/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/metric](https://github.com/open-telemetry/opentelemetry-go/blob/metric/v1.45.0/metric/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/sdk](https://github.com/open-telemetry/opentelemetry-go/blob/sdk/v1.45.0/sdk/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/sdk/metric](https://github.com/open-telemetry/opentelemetry-go/blob/sdk/metric/v1.45.0/sdk/metric/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/otel/trace](https://github.com/open-telemetry/opentelemetry-go/blob/trace/v1.45.0/trace/LICENSE) | Apache-2.0 |
| [go.opentelemetry.io/proto/otlp](https://github.com/open-telemetry/opentelemetry-proto-go/blob/otlp/v1.11.0/otlp/LICENSE) | Apache-2.0 |
| [go.uber.org/multierr](https://github.com/uber-go/multierr/blob/v1.11.0/LICENSE.txt) | MIT |
| [golang.org/x/net](https://cs.opensource.google/go/x/net/+/v0.57.0:LICENSE) | BSD-3-Clause |
| [golang.org/x/sync](https://cs.opensource.google/go/x/sync/+/v0.22.0:LICENSE) | BSD-3-Clause |
| [golang.org/x/sys/unix](https://cs.opensource.google/go/x/sys/+/v0.47.0:LICENSE) | BSD-3-Clause |
| [golang.org/x/text](https://cs.opensource.google/go/x/text/+/v0.40.0:LICENSE) | BSD-3-Clause |
| [google.golang.org/genproto/googleapis/api/httpbody](https://github.com/googleapis/go-genproto/blob/6ac0973c030d/googleapis/api/LICENSE) | Apache-2.0 |
| [google.golang.org/genproto/googleapis/rpc/status](https://github.com/googleapis/go-genproto/blob/6ac0973c030d/googleapis/rpc/LICENSE) | Apache-2.0 |
| [google.golang.org/grpc](https://github.com/grpc/grpc-go/blob/v1.83.0/LICENSE) | Apache-2.0 |
| [google.golang.org/protobuf](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.11/LICENSE) | BSD-3-Clause |

