.PHONY: lint test build clean fmt vet help

# Default target
.DEFAULT_GOAL := help

# Variables
GOLANGCI_LINT_TIMEOUT := 5m
TEST_FLAGS := -v -cover

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	golangci-lint run --timeout=$(GOLANGCI_LINT_TIMEOUT)

test: ## Run tests with coverage
	@echo "Running tests..."
	go test $(TEST_FLAGS) ./...

build: ## Build the project
	@echo "Building..."
	go build ./...

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	go clean ./...

vulncheck: ## Scan for vulnerabilities reachable from this code (incl. stdlib)
	@echo "Running govulncheck..."
	@# Pinning GOTOOLCHAIN to the version go.mod declares is the entire point,
	@# and it is the same version CI resolves via go-version-file. Left alone,
	@# `go run ...@latest` silently upgrades the toolchain to satisfy
	@# govulncheck's own go directive and then scans THAT stdlib — reporting
	@# clean while the version actually shipped stays vulnerable. (GOTOOLCHAIN=local
	@# is NOT the fix: it pins to whichever go is on PATH, which is a third
	@# unrelated version.) Scan what is pinned, not what is convenient.
	GOTOOLCHAIN=go$(shell awk '/^go /{print $$2; exit}' go.mod) \
		go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fuzz: ## Fuzz the untrusted-input paths (override with FUZZTIME=30m)
	@echo "Fuzzing each target for $(or $(FUZZTIME),60s)..."
	go test ./step/ -run FuzzParseBytes -fuzz FuzzParseBytes -fuzztime=$(or $(FUZZTIME),60s)
	go test . -run FuzzAssemble -fuzz FuzzAssemble -fuzztime=$(or $(FUZZTIME),60s)

ci: lint test vulncheck ## Run the blocking CI checks (lint + test + vulncheck)

all: fmt vet lint test build ## Run all checks and build
