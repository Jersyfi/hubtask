# syntax=docker/dockerfile:1.10
# One image, several roles (ADR-0014). The role is set at runtime through HUBTASK_ROLES.

FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
# Dependencies first - their own layer, so that code changes do not discard the cache.
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
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
FROM alpine:3.20 AS certs
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
