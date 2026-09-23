.PHONY: lint test build clean fmt vet help docs docs-serve docs-deps parity parity-report parity-baseline oracle

# Default target
.DEFAULT_GOAL := help

# Variables
GOLANGCI_LINT_TIMEOUT := 5m
# Matches the Test job in ci.yml, including -coverpkg, so the number `make test`
# prints is the number Codecov reports rather than a rosier per-package one.
TEST_FLAGS := -v -covermode=atomic -coverpkg=./... -coverprofile=coverage.out

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

clean: ## Clean build artifacts (Go cache, rendered site, docs venv)
	@echo "Cleaning..."
	go clean ./...
	rm -rf site $(DOCS_VENV)

GOVULNCHECK_VERSION ?= v1.7.0

vulncheck: ## Scan for vulnerabilities reachable from this code (incl. stdlib)
	@echo "Running govulncheck..."
	@# Pinning GOTOOLCHAIN to the version go.mod declares is the entire point,
	@# and it is the same version CI resolves via go-version-file. Left alone,
	@# `go run ...@latest` silently upgrades the toolchain to satisfy
	@# govulncheck's own go directive and then scans THAT stdlib — reporting
	@# clean while the version actually shipped stays vulnerable. (GOTOOLCHAIN=local
	@# is NOT the fix: it pins to whichever go is on PATH, which is a third
	@# unrelated version.) Scan what is pinned, not what is convenient.
	@# The tool is pinned too, for the same reason CI pins it: a govulncheck
	@# release whose go directive is newer than go.mod's cannot run under the
	@# pinned toolchain at all. Raise it when go.mod's go directive moves.
	GOTOOLCHAIN=go$(shell awk '/^go /{print $$2; exit}' go.mod) \
		go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

fuzz: ## Fuzz the untrusted-input paths (override with FUZZTIME=30m)
	@echo "Fuzzing each target for $(or $(FUZZTIME),60s)..."
	.github/scripts/fuzz-smoke.sh ./step/ FuzzParseBytes $(or $(FUZZTIME),60s)
	.github/scripts/fuzz-smoke.sh . FuzzAssemble $(or $(FUZZTIME),60s)
	.github/scripts/fuzz-smoke.sh ./geometry/ FuzzUnionArea2D $(or $(FUZZTIME),60s)

fuzz-deep: ## Fuzz hard (default 30m per target; override with FUZZTIME=2h)
	@echo "Deep-fuzzing each target for $(or $(FUZZTIME),30m)..."
	.github/scripts/fuzz-smoke.sh ./step/ FuzzParseBytes $(or $(FUZZTIME),30m)
	.github/scripts/fuzz-smoke.sh . FuzzAssemble $(or $(FUZZTIME),30m)
	.github/scripts/fuzz-smoke.sh ./geometry/ FuzzUnionArea2D $(or $(FUZZTIME),30m)

# The docs toolchain is Python, so it lives in a venv rather than in the
# developer's global site-packages — and in the SAME pinned versions the Docs
# workflow installs, so a local build that passes is one CI will reproduce.
DOCS_VENV := .venv-docs
DOCS_BIN  := $(DOCS_VENV)/bin

$(DOCS_BIN)/mkdocs: requirements-docs.txt
	@echo "Installing the docs toolchain into $(DOCS_VENV)..."
	python3 -m venv $(DOCS_VENV)
	$(DOCS_BIN)/pip install --quiet --upgrade pip
	$(DOCS_BIN)/pip install --quiet -r requirements-docs.txt
	@touch $(DOCS_BIN)/mkdocs

docs-deps: $(DOCS_BIN)/mkdocs ## Install the pinned docs toolchain into .venv-docs

docs: $(DOCS_BIN)/mkdocs ## Build the docs site (strict — a broken link fails)
	@echo "Building docs..."
	$(DOCS_BIN)/mkdocs build --strict

docs-serve: $(DOCS_BIN)/mkdocs ## Serve the docs at http://127.0.0.1:8000 with live reload
	$(DOCS_BIN)/mkdocs serve

parity: ## Run the parity and coverage gates against the public IFC corpus
	@echo "Running parity gates..."
	cd parity && CGO_ENABLED=0 go test ./...
	cd parity && CGO_ENABLED=0 go run ./cmd/coverage -check

parity-report: ## Regenerate docs/coverage.md from the corpus
	cd parity && CGO_ENABLED=0 go run ./cmd/coverage

parity-baseline: ## Rewrite the committed coverage baseline (after closing a gap)
	cd parity && CGO_ENABLED=0 go test ./... -run TestGate2 -update-baseline

oracle: ## Diff freshly generated ifcopenshell AABB oracles against the committed ones (maintainer only; needs Docker)
	@# This target NEVER writes parity/testdata/oracle/, which is what
	@# parity/oracle/README.md has always prescribed. Those files are the
	@# measurement Gate 1 and the nine-decimal shortfalls in
	@# parity/knownviolations.go are defined against: an unconditional copy from
	@# a half-working image would leave both gates passing against weaker boxes,
	@# the allowlist quietly meaningless and the published page regenerated to
	@# different-but-plausible numbers, with nothing to show it happened. So
	@# generate into a temporary directory, diff, and leave promoting the result
	@# to a deliberate human act (README: "Promoting a verified regeneration").
	@#
	@# One shell, `set -eu`, and an explicit check after every step, because the
	@# old loop took its exit status from a trailing `rm` and so reported
	@# success after a docker run that had failed or written half a file.
	@set -eu; \
	tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp" || true' EXIT INT TERM; \
	echo "Generating oracles into $$tmp (nothing under parity/testdata/oracle/ is written)..."; \
	moved=0; \
	for m in ifcopenhouse duplex_a fzk_haus; do \
		gzip -dc parity/testdata/$$m.ifc.gz > "$$tmp/$$m.ifc" \
			|| { echo "oracle: could not decompress parity/testdata/$$m.ifc.gz"; exit 1; }; \
		docker run --rm --user "$$(id -u):$$(id -g)" \
			-v "$$tmp":/data -v "$(CURDIR)/parity/oracle":/src:ro \
			aecgeeks/ifcopenshell:latest \
			python3 /src/dump_oracle.py "/data/$$m.ifc" "/data/$$m.json" \
			|| { echo "oracle: ifcopenshell failed on $$m — see the Status section of parity/oracle/README.md"; exit 1; }; \
		[ -s "$$tmp/$$m.json" ] \
			|| { echo "oracle: ifcopenshell wrote no output for $$m"; exit 1; }; \
		echo "=== $$m"; \
		if diff -u "parity/testdata/oracle/$$m.json" "$$tmp/$$m.json"; then \
			echo "    identical to the committed oracle"; \
		else \
			moved=1; \
		fi; \
	done; \
	echo ""; \
	if [ "$$moved" -eq 0 ]; then \
		echo "oracle: every model reproduces the committed oracle exactly."; \
	else \
		echo "oracle: the generated oracles DIFFER from the committed ones, and nothing was overwritten."; \
		echo "oracle: a shifted oracle moves Gate 1's meaning and invalidates the"; \
		echo "oracle: knownViolations shortfalls. Read parity/oracle/README.md before"; \
		echo "oracle: promoting anything."; \
		exit 1; \
	fi

ci: lint test vulncheck parity ## Run the blocking CI checks (lint + test + vulncheck + parity)

all: fmt vet lint test build ## Run all checks and build
