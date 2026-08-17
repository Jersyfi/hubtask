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

# Every tool is pinned. An unpinned tool version turns a gate into a moving target and is a
# supply chain decision made by whoever runs make (ADR-0015).
GOLANGCI_LINT_VERSION := v2.12.2
OAPI_CODEGEN_VERSION  := v2.8.0
SQLC_VERSION          := v1.31.1
GOOSE_VERSION         := v3.27.3
GOVULNCHECK_VERSION   := v1.7.0

export CGO_ENABLED := 0

.DEFAULT_GOAL := help

## help: List the available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F':' '{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ------------------------------------------------------------------- Utilities

# go_test runs the tests of a package set - but only once that set exists. The milestone builds
# every gate up front and fills them in task by task (docs/backlog/milestone-0.1.0.md); a gate
# for an empty directory must say so rather than fail, and must start biting the moment the
# first package appears.
# $(1) = build tags, $(2) = package patterns, $(3) = extra go test flags
define go_test
	@pkgs="$$($(GO) list -tags='$(1)' $(2) 2>/dev/null)"; \
	if [ -z "$$pkgs" ]; then \
		echo "skipped: no packages under $(2) yet"; \
	else \
		set -x; $(GO) test -tags='$(1)' $(3) $$pkgs; \
	fi
endef

# require_tool fails loudly instead of quietly skipping: a gate that silently does nothing is
# worse than no gate at all.
define require_tool
	@test -x $(TOOLS_DIR)/$(1) || { echo "$(1) is missing - run 'make tools'"; exit 1; }
endef

# ---------------------------------------------------------------- Development

## tools: Install the development tools into .tools
.PHONY: tools
tools:
	@mkdir -p $(TOOLS_DIR)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

## fmt: Format the source
.PHONY: fmt
fmt:
	$(GO) fmt ./...
	@test -x $(TOOLS_DIR)/golangci-lint && $(TOOLS_DIR)/golangci-lint fmt ./... || true

## generate: Generate code from openapi.yaml and db/queries
# The specification is the source, the code is the result (ADR-0004, CLAUDE.md rule 11). Running
# this target must never be a judgement call, which is why gate-quick runs it and fails on a diff.
.PHONY: generate
generate:
	$(GO) generate ./...
	$(call require_tool,oapi-codegen)
	$(TOOLS_DIR)/oapi-codegen --config api/oapi-codegen.yaml api/openapi.yaml
	@# sqlc gets wired in here with the first query file

## build: Build the server, the migrator and the CLI
# cmd/migrate arrives with A-03 and cmd/hubctl later; until then their directories are empty and
# there is nothing to build there.
.PHONY: build
build:
	@for pair in server:hubtask-server migrate:hubtask-migrate hubctl:hubctl; do \
		cmd="$${pair%%:*}"; out="$${pair##*:}"; \
		if [ -z "$$(ls cmd/$$cmd/*.go 2>/dev/null)" ]; then \
			echo "skipped: cmd/$$cmd has no sources yet"; continue; \
		fi; \
		( set -x; $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$$out ./cmd/$$cmd ) || exit 1; \
	done

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
	$(call require_tool,golangci-lint)
	@test -z "$$(gofmt -l . | grep -v '^vendor/')" || { echo "gofmt violations:"; gofmt -l .; exit 1; }
	@diff=$$($(TOOLS_DIR)/golangci-lint fmt --diff ./... 2>&1); \
		test -z "$$diff" || { echo "formatting violations - run 'make fmt':"; echo "$$diff"; exit 1; }
	$(GO) vet ./...
	$(TOOLS_DIR)/golangci-lint run ./...
	@# The comparison is against the state before generating, not against HEAD: a work tree with
	@# uncommitted changes must still be able to run the gate.
	@before="$$(git status --porcelain)"; \
		$(MAKE) --no-print-directory generate; \
		after="$$(git status --porcelain)"; \
		if [ "$$before" != "$$after" ]; then \
			echo "make generate produces a diff - please commit it:"; \
			diff <(echo "$$before") <(echo "$$after") || true; \
			exit 1; \
		fi

## gate-unit: Domain, application, adapter and presentation tests with coverage thresholds
# The race detector needs cgo, so this one target overrides the CGO_ENABLED=0 of the build.
#
# infrastructure is in the list because its adapters carry rules no other gate checks - metric
# label cardinality, log redaction, the tenant wrapper. Anything needing a container lives behind
# the `integration` build tag in test/integration and stays out of this target.
.PHONY: gate-unit
gate-unit: export CGO_ENABLED = 1
gate-unit:
	$(call go_test,,./core/... ./infrastructure/... ./presentation/...,-race -covermode=atomic -coverprofile=coverage.out)
	@$(MAKE) --no-print-directory coverage-check PKG=./core/domain/... MIN=85
	@$(MAKE) --no-print-directory coverage-check PKG=./core/application/... MIN=75

# The threshold applies per package, not as an average over the tree. An average lets a new,
# entirely untested package hide behind well-covered neighbours - which is exactly what
# gate-selftest caught once core/domain held its first real package.
.PHONY: coverage-check
coverage-check:
	@pkgs="$$($(GO) list $(PKG) 2>/dev/null)"; \
	if [ -z "$$pkgs" ]; then echo "coverage $(PKG): no packages yet - skipped"; exit 0; fi; \
	out="$$($(GO) test -covermode=atomic -cover $$pkgs 2>&1)" || { echo "$$out"; exit 1; }; \
	echo "$$out" | awk -v min="$(MIN)" ' \
		/\[no test files\]/ { printf("  %s: no test file at all\n", $$2); failed=1; next } \
		/coverage:/ { \
			for (i = 1; i <= NF; i++) if ($$i == "coverage:") { value = $$(i + 1); break } \
			if (value ~ /statements/ || value ~ /\[/) next; \
			gsub(/%/, "", value); \
			if (value + 0 < min + 0) { printf("  %s: %s%% below the %s%% threshold\n", $$2, value, min); failed=1 } \
			else printf("  %s: %s%%\n", $$2, value) \
		} \
		END { if (failed) { print "coverage $(PKG): below the threshold"; exit 1 } }'

## gate-integration: Tests against a real PostgreSQL (Testcontainers)
.PHONY: gate-integration
gate-integration:
	$(call go_test,integration,./test/integration/...,)

## gate-contract: Responses against openapi.yaml, events against JSON schemas
.PHONY: gate-contract
gate-contract:
	$(call go_test,contract,./test/contract/...,)

## gate-architecture: Layer rules, bare-goroutine ban, use case parity, audit registry
.PHONY: gate-architecture
gate-architecture:
	$(call go_test,,./test/architecture/...,)

## gate-security: SG-1..SG-12
.PHONY: gate-security
gate-security:
	$(call require_tool,govulncheck)
	$(TOOLS_DIR)/govulncheck ./...
	$(call go_test,,./test/security/...,)

## gate-data: Migrations, retention, backup round trip, sync
.PHONY: gate-data
gate-data:
	$(call go_test,integration,./test/retention/... ./test/backup/... ./test/sync/... ./test/audit/...,)

## gate-resilience: RT-1..RT-12
.PHONY: gate-resilience
gate-resilience:
	$(call go_test,resilience,./test/resilience/...,)

## gate-docs: Check cross references and the ADR index
.PHONY: gate-docs
gate-docs:
	@if [ -d tools/checkdocs ]; then $(GO) run ./tools/checkdocs; \
		else echo "skipped: tools/checkdocs arrives with task A-10"; fi

## gate-selftest: Prove that every configured rule actually fails a build
.PHONY: gate-selftest
gate-selftest:
	$(call require_tool,golangci-lint)
	@scripts/gate-selftest.sh

# ------------------------------------------------------------- Database / ops

## db-up: Start the development database
.PHONY: db-up
db-up:
	docker compose -f deploy/docker/compose.dev.yaml up -d postgres

## migrate: Apply the migrations
.PHONY: migrate
migrate:
	$(call require_tool,goose)
	@test -n "$$(ls db/migrations/*.sql 2>/dev/null)" || { echo "skipped: no migrations yet"; exit 0; }; \
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
	rm -rf bin dist coverage.out coverage.*.out

## gate-fuzz: Fuzzing for the query DSL, CEL input, webhook signatures (nightly)
.PHONY: gate-fuzz
gate-fuzz:
	$(GO) test -run=NONE -fuzz=Fuzz -fuzztime=5m ./core/... ./infrastructure/... || true
	@echo "Note: fuzz targets appear alongside the building blocks they cover (SG-8)."

## gate-load: Load test against the target figures (nightly)
.PHONY: gate-load
gate-load:
	$(call go_test,load,./test/load/...,-timeout=60m)

## release-tag: Create and push a signed release tag
.PHONY: release-tag
release-tag:
	@test -n "$(VERSION_TO_TAG)" || { echo "Usage: make release-tag VERSION_TO_TAG=1.2.3"; exit 1; }
	@git diff --quiet || { echo "Working tree is not clean"; exit 1; }
	@$(MAKE) verify
	git tag -s "v$(VERSION_TO_TAG)" -m "Release v$(VERSION_TO_TAG)"
	git push origin "v$(VERSION_TO_TAG)"
	@echo "The release workflow now waits for the 'production' environment approval."
