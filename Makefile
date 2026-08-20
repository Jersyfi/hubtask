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
# The container engine. Podman is a supported runtime and takes the same arguments; the matrix job
# sets both this and HUBTASK_COMPOSE (docs/architecture/support-matrix.md).
DOCKER      ?= docker
TOOLS_DIR   := .tools
IMAGE       ?= ghcr.io/jersyfi/hubtask

# Every tool is pinned. An unpinned tool version turns a gate into a moving target and is a
# supply chain decision made by whoever runs make (ADR-0015).
GOLANGCI_LINT_VERSION := v2.12.2
OAPI_CODEGEN_VERSION  := v2.8.0
SQLC_VERSION          := v1.31.1
GOOSE_VERSION         := v3.27.3
GOVULNCHECK_VERSION   := v1.7.0
# helm is installed the same way as every other tool - through the module proxy, with the Go
# checksum database vouching for it. A tarball from a release page would be a second trust
# anchor for no reason (ADR-0015).
HELM_VERSION          := v3.16.4
GO_LICENSES_VERSION   := v1.6.0
# promtool cannot be installed with `go install`: the Prometheus module carries replace
# directives, which the tool refuses. So it comes as the project's own release archive, pinned by
# version *and* by checksum - a download without one is a supply chain decision made by whoever
# happens to be on the network (ADR-0015).
PROMTOOL_VERSION      := 3.6.0
PROMTOOL_SHA256_darwin_arm64 := ad132f6b1651a2bdaa8464fb122898747ac406defc03e33a118afddd23138b65
PROMTOOL_SHA256_linux_amd64  := 2002ef4a55a64161affccd2786c7081d4e3b3a8d08786a98b3bb110971414916
PROMTOOL_SHA256_linux_arm64  := f7e66b9d47e86988fe8e7cb5a5b326cab6c56f5a74ba7133b899ef1daedaf633
# The chart declares kubeVersion >= 1.28. helm renders against a much older version unless it is
# told otherwise, so the gate says which cluster it is rendering for.
KUBE_VERSION          := 1.30.0

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
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)
	GOBIN=$(PWD)/$(TOOLS_DIR) $(GO) install github.com/google/go-licenses@$(GO_LICENSES_VERSION)
	@$(MAKE) --no-print-directory tools-promtool

# promtool, verified against the checksum above before anything is unpacked. Separated from the
# `go install` block only because its acquisition differs, not its status: it is as pinned as the
# rest.
.PHONY: tools-promtool
tools-promtool:
	@mkdir -p $(TOOLS_DIR)
	@if [ -x "$(TOOLS_DIR)/promtool" ]; then exit 0; fi; \
	os="$$(uname -s | tr '[:upper:]' '[:lower:]')"; \
	arch="$$(uname -m)"; \
	case "$$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac; \
	expected="$$($(MAKE) --no-print-directory -s print-promtool-sha OS=$$os ARCH=$$arch)"; \
	if [ -z "$$expected" ]; then \
		echo "no pinned promtool checksum for $$os/$$arch - add one to the Makefile"; exit 1; \
	fi; \
	work="$$(mktemp -d)"; \
	archive="$$work/promtool.tar.gz"; \
	url="https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).$$os-$$arch.tar.gz"; \
	echo "downloading promtool $(PROMTOOL_VERSION) for $$os/$$arch"; \
	curl -fsSL "$$url" -o "$$archive"; \
	actual="$$(shasum -a 256 "$$archive" | cut -d' ' -f1)"; \
	if [ "$$actual" != "$$expected" ]; then \
		echo "promtool checksum mismatch: expected $$expected, got $$actual"; exit 1; \
	fi; \
	tar -xzf "$$archive" -C "$$work"; \
	cp "$$work/prometheus-$(PROMTOOL_VERSION).$$os-$$arch/promtool" $(TOOLS_DIR)/promtool; \
	chmod +x $(TOOLS_DIR)/promtool; \
	rm -rf "$$work"

# print-promtool-sha exists so the recipe above can look a checksum up by platform; make cannot
# index a variable by a shell value any other way.
.PHONY: print-promtool-sha
print-promtool-sha:
	@echo "$(PROMTOOL_SHA256_$(OS)_$(ARCH))"

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
	$(call require_tool,sqlc)
	$(TOOLS_DIR)/sqlc -f db/sqlc.yaml generate

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
verify: gate-quick gate-unit gate-architecture gate-security gate-chart gate-licenses gate-docs gate-observability
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

## gate-unit: Domain, application, adapter, presentation and CLI tests with coverage thresholds
# The race detector needs cgo, so this one target overrides the CGO_ENABLED=0 of the build.
#
# infrastructure is in the list because its adapters carry rules no other gate checks - metric
# label cardinality, log redaction, the tenant wrapper. Anything needing a container lives behind
# the `integration` build tag in test/integration and stays out of this target.
#
# cmd joined the list with hubctl (B-13). A composition root has little to test, but a CLI is not
# one: its flags, its exit codes and the sentences it prints are the contract a script depends on,
# and the end-to-end session only reaches them once the whole stack is up.
.PHONY: gate-unit
gate-unit: export CGO_ENABLED = 1
gate-unit:
	$(call go_test,,./cmd/... ./core/... ./infrastructure/... ./presentation/...,-race -covermode=atomic -coverprofile=coverage.out)
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

## gate-licenses: Refuse a dependency whose licence would make relicensing impossible
.PHONY: gate-licenses
gate-licenses:
	$(call require_tool,go-licenses)
	@# forbidden is AGPL and friends, restricted is the GPL/LGPL family. Either one would make the
	@# conversion to Apache-2.0 and the commercial licence impossible (ADR-0013), which is why this
	@# is a gate and not a review note. The warnings about packages containing non-Go code are the
	@# tool saying it cannot follow a .s file - not a finding.
	@output="$$($(TOOLS_DIR)/go-licenses check ./... --disallowed_types=forbidden,restricted 2>&1)"; \
		status=$$?; \
		echo "$$output" | grep -vE "contains non-Go code|^/|^W[0-9]" || true; \
		if [ $$status -ne 0 ]; then \
			echo "a dependency carries a licence that would make relicensing impossible (ADR-0013)"; \
			exit 1; \
		fi; \
		echo "licences: no forbidden or restricted dependency"
	@# And the list that ships with the release is the list of what is actually linked. The
	@# comparison is against the state before generating, like in gate-quick, so a work tree with
	@# other uncommitted changes can still run the gate.
	@before="$$(git status --porcelain THIRD-PARTY-LICENSES.md)"; \
		$(MAKE) --no-print-directory licenses >/dev/null; \
		after="$$(git status --porcelain THIRD-PARTY-LICENSES.md)"; \
		if [ "$$before" != "$$after" ]; then \
			echo "THIRD-PARTY-LICENSES.md is out of date - run 'make licenses' and commit it"; \
			exit 1; \
		fi

## licenses: Regenerate THIRD-PARTY-LICENSES.md
.PHONY: licenses
licenses:
	$(call require_tool,go-licenses)
	@# For linux/amd64, not for whoever runs make. The list is of what ships, and what ships is a
	@# linux image (ADR-0014) - on macOS the report is one dependency shorter, because
	@# prometheus/procfs is only linked there. A generated file that differs by developer is a
	@# generated file nobody can check.
	GOOS=linux GOARCH=amd64 $(TOOLS_DIR)/go-licenses report ./... --template tools/licenses.md.tpl 2>/dev/null > THIRD-PARTY-LICENSES.md
	@echo "THIRD-PARTY-LICENSES.md written"

## gate-chart: helm lint and template, with every optional object switched on
.PHONY: gate-chart
gate-chart:
	$(call require_tool,helm)
	@# The secret is a name, not a value: the chart refuses to render without one, because a
	@# secret in values.yaml would end up in the release history (deployment.md §6).
	$(TOOLS_DIR)/helm lint k8s --set existingSecret=hubtask-secrets
	$(TOOLS_DIR)/helm template hubtask k8s --kube-version $(KUBE_VERSION) \
		--set existingSecret=hubtask-secrets > /dev/null
	@# Again with everything the defaults leave off. An optional object nobody renders is an
	@# object that breaks in the one installation that needs it.
	$(TOOLS_DIR)/helm template hubtask k8s --kube-version $(KUBE_VERSION) \
		--set existingSecret=hubtask-secrets \
		--set ingress.enabled=true --set ingress.host=hubtask.example.com \
		--set serviceMonitor.enabled=true \
		--set roles.api.autoscaling.enabled=true \
		--set smtp.existingSecretKey=smtp-password \
		--set storage.existingSecret=hubtask-storage --set storage.bucket=hubtask-media \
		--set networkPolicy.allowedEgressCIDRs={10.0.0.0/8} > /dev/null
	@echo "chart: lint and template green"

## gate-compose: Start the self-hosting reference stack from a real image and wait for /readyz
.PHONY: gate-compose
gate-compose: docker-build
	scripts/compose-smoke.sh $(VERSION)

## gate-e2e: The hubctl end-to-end session against the reference Compose stack
# The other half of gate-compose. That one asks whether the stack starts; this one asks whether a
# person can do anything with it - sign in, build a hierarchy, complete, delete, restore - through
# the client rather than through curl.
.PHONY: gate-e2e
gate-e2e: docker-build
	scripts/hubctl-e2e.sh $(VERSION)

## gate-observability: The shipped alert rules, checked by Prometheus itself
.PHONY: gate-observability
gate-observability:
	$(call require_tool,promtool)
	$(TOOLS_DIR)/promtool check rules deploy/observability/alerts/prometheus-rules.yaml
	@# The structural half - every alert has a runbook, every runbook an alert - is a Go test, so
	@# that it runs in `make verify` without needing a downloaded tool (test/observability).
	$(call go_test,,./test/observability/...,)

## gate-kind: Install the chart into a real cluster (expects a kind cluster to exist)
.PHONY: gate-kind
gate-kind: docker-build
	$(call require_tool,helm)
	scripts/kind-smoke.sh $(VERSION)

## gate-docs: Check cross references and the ADR index
.PHONY: gate-docs
gate-docs:
	$(GO) run ./tools/checkdocs

## gate-action-pins: Every action pin resolves, and its comment names the right tag (needs network)
.PHONY: gate-action-pins
gate-action-pins:
	@# Not in `make verify`: it asks github.com what a commit SHA is, and the local gates run
	@# offline. The nightly carries it, which is where a pin that has rotted gets noticed.
	scripts/check-action-pins.sh

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
	$(DOCKER) build -t $(IMAGE):$(VERSION) \
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
