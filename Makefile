# Every CI gate is a make target: the pipeline invokes nothing but make.
# This keeps each gate reproducible locally (see ADR-0022, docs/architecture/ci-cd.md).

SHELL       := /bin/bash
MODULE      := $(shell head -1 go.mod | cut -d' ' -f2)
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(BUILD_DATE)
GO          ?= go
TOOLS_DIR   := .tools
IMAGE       ?= ghcr.io/Jersyfi/hubtask

export CGO_ENABLED := 0

.DEFAULT_GOAL := help

## help: List the available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F':' '{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------- Development

## tools: Install the development tools into .tools
.PHONY: tools
tools:
	@mkdir -p $(TOOLS_DIR)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/pressly/goose/v3/cmd/goose@v3.22.1
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@latest

## fmt: Format the source
.PHONY: fmt
fmt:
	$(GO) fmt ./...

## generate: Generate code from openapi.yaml and db/queries
.PHONY: generate
generate:
	$(GO) generate ./...
	@# oapi-codegen and sqlc get wired in here from 0.1.0 onwards

## build: Build the server, the migrator and the CLI
.PHONY: build
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/hubtask-server ./cmd/server
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/hubtask-migrate ./cmd/migrate
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/hubctl ./cmd/hubctl

## run: Start the server locally
.PHONY: run
run:
	$(GO) run ./cmd/server

# ---------------------------------------------------------------------- Gates

## verify: Run every PR gate locally (mirrors ci.yml)
.PHONY: verify
verify: gate-quick gate-unit gate-architecture gate-security
	@echo "All locally runnable gates are green."

## gate-quick: Format, lint, generation without a diff
.PHONY: gate-quick
gate-quick:
	@test -z "$$(gofmt -l . | grep -v '^vendor/')" || { echo "gofmt violations:"; gofmt -l .; exit 1; }
	$(GO) vet ./...
	@if [ -x $(TOOLS_DIR)/golangci-lint ]; then $(TOOLS_DIR)/golangci-lint run ./...; else echo "Note: run 'make tools' for golangci-lint"; fi
	@$(MAKE) --no-print-directory generate
	@git diff --exit-code || { echo "make generate produces a diff - please commit it."; exit 1; }

## gate-unit: Domain and application tests with coverage thresholds
.PHONY: gate-unit
gate-unit:
	$(GO) test -race -covermode=atomic -coverprofile=coverage.out ./core/... ./presentation/...
	@$(MAKE) --no-print-directory coverage-check PKG=./core/domain/... MIN=85
	@$(MAKE) --no-print-directory coverage-check PKG=./core/application/... MIN=75

.PHONY: coverage-check
coverage-check:
	@$(GO) test -covermode=atomic -coverprofile=/tmp/cov.out $(PKG) >/dev/null 2>&1 || true
	@if [ -f /tmp/cov.out ]; then \
		pct=$$($(GO) tool cover -func=/tmp/cov.out 2>/dev/null | tail -1 | awk '{print $$3}' | tr -d '%'); \
		pct=$${pct:-0}; \
		awk -v p="$$pct" -v m="$(MIN)" 'BEGIN{ if (p+0 < m+0) { printf("Coverage %s%% below the %s%% threshold for $(PKG)\n", p, m); exit 1 } else printf("Coverage %s%% >= %s%% for $(PKG)\n", p, m) }'; \
	fi

## gate-integration: Tests against a real PostgreSQL (Testcontainers)
.PHONY: gate-integration
gate-integration:
	$(GO) test -tags=integration ./test/integration/...

## gate-contract: Responses against openapi.yaml, events against JSON schemas
.PHONY: gate-contract
gate-contract:
	$(GO) test -tags=contract ./test/contract/...

## gate-architecture: Layer rules, bare-goroutine ban, use case parity, audit registry
.PHONY: gate-architecture
gate-architecture:
	$(GO) test ./test/architecture/...

## gate-security: SG-1..SG-12
.PHONY: gate-security
gate-security:
	@if [ -x $(TOOLS_DIR)/govulncheck ]; then $(TOOLS_DIR)/govulncheck ./...; else echo "Note: run 'make tools' for govulncheck"; fi
	$(GO) test ./test/security/... 2>/dev/null || echo "Note: security tests follow from 0.1.0 onwards"

## gate-data: Migrations, retention, backup round trip, sync
.PHONY: gate-data
gate-data:
	$(GO) test -tags=integration ./test/retention/... ./test/backup/... ./test/sync/... ./test/audit/...

## gate-resilience: RT-1..RT-12
.PHONY: gate-resilience
gate-resilience:
	$(GO) test -tags=resilience ./test/resilience/...

## gate-docs: Check cross references and the ADR index
.PHONY: gate-docs
gate-docs:
	$(GO) run ./tools/checkdocs

# ------------------------------------------------------------- Database / ops

## db-up: Start the development database
.PHONY: db-up
db-up:
	docker compose -f deploy/docker/compose.dev.yaml up -d postgres

## migrate: Apply the migrations
.PHONY: migrate
migrate:
	$(TOOLS_DIR)/goose -dir db/migrations postgres "$$HUBTASK_DB_DSN" up

## docker-build: Build the container image
.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE):$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) \
		-f deploy/docker/Dockerfile .

## clean: Remove build artefacts
.PHONY: clean
clean:
	rm -rf bin dist coverage.out

## gate-fuzz: Fuzzing for the query DSL, CEL input, webhook signatures (nightly)
.PHONY: gate-fuzz
gate-fuzz:
	$(GO) test -run=NONE -fuzz=Fuzz -fuzztime=5m ./core/... ./infrastructure/... || true
	@echo "Note: fuzz targets appear alongside the building blocks they cover (SG-8)."

## gate-load: Load test against the target figures (nightly)
.PHONY: gate-load
gate-load:
	$(GO) test -tags=load -timeout=60m ./test/load/... 2>/dev/null || \
		echo "No load tests yet - they arrive with milestone 0.6.0 (RT-6)."

## release-tag: Create and push a signed release tag
.PHONY: release-tag
release-tag:
	@test -n "$(VERSION_TO_TAG)" || { echo "Usage: make release-tag VERSION_TO_TAG=1.2.3"; exit 1; }
	@git diff --quiet || { echo "Working tree is not clean"; exit 1; }
	@$(MAKE) verify
	git tag -s "v$(VERSION_TO_TAG)" -m "Release v$(VERSION_TO_TAG)"
	git push origin "v$(VERSION_TO_TAG)"
	@echo "The release workflow now waits for the 'production' environment approval."
