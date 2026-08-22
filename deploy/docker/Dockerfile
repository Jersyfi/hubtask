# syntax=docker/dockerfile:1.10
# One image, several roles (ADR-0014). The role is set at runtime through HUBTASK_ROLES.
#
# Three stages, and the order of the first two is the point: the frontend is built in its own
# stage so that the Go module download does not depend on it. A changed stylesheet then costs a
# frontend build and a link, never a re-download of the module cache - and BuildKit runs the two
# in parallel.
#
# The result is still ONE image. The web interface is inside the binary (ADR-0028), so
# self-hosting stays two containers: this one and PostgreSQL. A UI container plus a proxy would be
# four, and would make it possible to run an interface against an API of a different version.

FROM node:24-alpine AS ui
WORKDIR /ui
ENV CI=true
RUN corepack enable
# The manifests first, so that `pnpm install` is a layer of its own: editing a component must not
# re-resolve the dependency tree.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/webapp/package.json apps/webapp/
COPY apps/website/package.json apps/website/
COPY packages/api-client/package.json packages/api-client/
COPY packages/design-system/package.json packages/design-system/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY apps apps
COPY packages packages
# The contract, for the generated client. Nothing else of the Go tree is needed here, and copying
# less is what keeps this stage from rebuilding when the backend changes.
COPY api/openapi.yaml api/openapi.yaml
# In dependency order, because each one consumes the one before it.
RUN pnpm --filter @hubtask/api-client build \
 && pnpm --filter @hubtask/design-system build \
 && pnpm --filter @hubtask/webapp build

FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
# Dependencies first - their own layer, so that code changes do not discard the cache.
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# The built interface goes where //go:embed expects it. This replaces the committed placeholder,
# which exists so that `go build ./...` works without Node.js at all (ADR-0028); a release image
# always has the real thing, and the build fails here rather than shipping the placeholder if the
# stage above produced nothing.
COPY --from=ui /ui/apps/webapp/dist ./presentation/webui/dist
RUN test -d ./presentation/webui/dist/assets || \
    { echo "the ui stage produced no assets - refusing to build a binary with the placeholder"; exit 1; }
ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/hubtask ./cmd/server && \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
      -trimpath -ldflags "-s -w" -o /out/hubtask-migrate ./cmd/migrate

# Time zone data and root certificates from a package image, not hand-assembled.
FROM alpine:3.24 AS certs
RUN apk add --no-cache ca-certificates tzdata

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/hubtask /usr/local/bin/hubtask
COPY --from=build /out/hubtask-migrate /usr/local/bin/hubtask-migrate
COPY --from=build /src/locales /app/locales

USER nonroot:nonroot
WORKDIR /app
EXPOSE 8080 9090
ENV HUBTASK_ROLES=api,worker,scheduler,automation \
    HUBTASK_HTTP_ADDR=:8080 \
    HUBTASK_OPS_ADDR=:9090

# Liveness checks only the process - never dependencies (ADR-0016).
ENTRYPOINT ["/usr/local/bin/hubtask"]
