# Build commands previously lived only as prose spread across README,
# QUICK_START and AGENTS.md, which is how they drift out of sync.

SHELL := /bin/bash
export GOTOOLCHAIN ?= auto

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X sys-sentient/internal/version.Version=$(VERSION) \
	-X sys-sentient/internal/version.Commit=$(COMMIT) \
	-X sys-sentient/internal/version.BuildDate=$(DATE)

GO_FILES := $(shell git ls-files '*.go' 2>/dev/null)

.DEFAULT_GOAL := help
.PHONY: help build web daemon run test test-go test-web lint fmt fmt-check vet vuln audit typecheck docker clean verify

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: web daemon ## Build the dashboard and the daemon

web: ## Build the dashboard into web/dist
	cd web && npm ci && npm run build

daemon: ## Build the daemon binary with version stamps
	go build -trimpath -ldflags "$(LDFLAGS)" -o sys-daemon ./cmd/daemon
	@./sys-daemon --version

run: ## Run the daemon (requires web/dist)
	./sys-daemon

fmt: ## Format Go sources
	gofmt -w $(GO_FILES)

fmt-check: ## Fail if any Go source is unformatted
	@test -z "$$(gofmt -l $(GO_FILES))" || { echo "unformatted:"; gofmt -l $(GO_FILES); exit 1; }

vet: ## go vet
	go vet ./...

# Pinned to match .github/workflows/ci.yml. Running a different version
# locally than CI runs is how "works on my machine" lint failures happen.
GOLANGCI_VERSION ?= v2.13.2

lint: ## golangci-lint (installs on demand, pinned to the CI version)
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run ./...

vuln: ## Scan Go dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

audit: ## Scan npm dependencies
	cd web && npm audit --audit-level=moderate

typecheck: ## TypeScript type check
	cd web && npm run typecheck

test-go: ## Go tests with the race detector
	go test ./... -race

test-web: ## Frontend tests
	cd web && npm test

test: test-go test-web ## All tests

verify: fmt-check vet test-go audit typecheck test-web ## Everything CI runs
	@echo "all checks passed"

docker: ## Build the container image
	docker build --pull \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t sys-sentient:$(VERSION) .

clean: ## Remove build artefacts
	rm -f sys-daemon
	rm -rf web/dist
