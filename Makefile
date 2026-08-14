# termchat — every command humans and agents run lives here (AGENTS.md §4).
.DEFAULT_GOAL := help
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

# --- pinned toolchain (AGENTS.md §3) -----------------------------------------
BUF_VERSION        := 1.72.0
GOLANGCI_VERSION   := 2.12.2

GOBIN              := $(shell go env GOPATH)/bin
BUF                := $(GOBIN)/buf
GOLANGCI           := $(GOBIN)/golangci-lint

WEB                := web
PNPM               := corepack pnpm
export COREPACK_ENABLE_DOWNLOAD_PROMPT := 0

# Subsystems land milestone by milestone; a target that has nothing to do yet says so
# and exits 0 rather than failing the gate (TASKS.md 0.3).
HAS_PROTO  := $(shell find proto -name '*.proto' -print -quit 2>/dev/null)
HAS_GO     := $(shell find cmd internal -name '*.go' -print -quit 2>/dev/null)
HAS_WEB    := $(shell find $(WEB)/src -name '*.ts*' -print -quit 2>/dev/null)
HAS_E2E    := $(shell find e2e -name '*.ts' -print -quit 2>/dev/null)

.PHONY: help setup gen lint lint-proto lint-go lint-web fmt test test-go test-web e2e dev build build-go build-web check clean

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

# --- setup -------------------------------------------------------------------

setup: ## Install tool deps and web dependencies
	go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_VERSION)
	corepack enable pnpm
	cd $(WEB) && $(PNPM) install
	@echo "setup: ok"
	@echo "  hooks: opt in with 'git config core.hooksPath .githooks' (runs make check pre-commit)"

# --- codegen -----------------------------------------------------------------

gen: ## buf generate -> gen/go + web/src/gen (run after ANY proto edit)
ifeq ($(HAS_PROTO),)
	@echo "gen: no proto/ yet — skipped"
else
	$(BUF) generate
endif

# --- lint --------------------------------------------------------------------

lint: lint-proto lint-go lint-web ## buf lint + golangci-lint + eslint + tsc --noEmit

lint-proto:
ifeq ($(HAS_PROTO),)
	@echo "lint-proto: no proto/ yet — skipped"
else
	$(BUF) lint
endif

lint-go:
ifeq ($(HAS_GO),)
	@echo "lint-go: no Go packages yet — skipped"
else
	go vet ./...
	$(GOLANGCI) run
endif

lint-web:
ifeq ($(HAS_WEB),)
	@echo "lint-web: no $(WEB)/src yet — skipped"
else
	cd $(WEB) && $(PNPM) run lint
	cd $(WEB) && $(PNPM) run format
	cd $(WEB) && $(PNPM) run typecheck
endif

fmt: ## Apply formatters in place
ifneq ($(HAS_GO),)
	$(GOLANGCI) fmt
endif
ifneq ($(HAS_WEB),)
	cd $(WEB) && $(PNPM) exec prettier --write .
endif
	@echo "fmt: ok"

# --- test --------------------------------------------------------------------

test: test-go test-web ## go test -race + web unit tests

test-go:
ifeq ($(HAS_GO),)
	@echo "test-go: no Go packages yet — skipped"
else
	go test ./... -race
endif

test-web:
ifeq ($(HAS_WEB),)
	@echo "test-web: no $(WEB)/src yet — skipped"
else
	cd $(WEB) && $(PNPM) run test
endif

e2e: ## Playwright
ifeq ($(HAS_E2E),)
	@echo "e2e: no e2e/ yet — skipped"
else
	cd $(WEB) && $(PNPM) exec playwright test
endif

# --- run ---------------------------------------------------------------------

dev: ## Go server on :8080 + Vite on :5173 (proxying /chat.v1 -> :8080)
ifeq ($(HAS_GO)$(HAS_WEB),)
	@echo "dev: nothing to run yet — skipped"
else
	@trap 'kill 0' EXIT INT TERM; \
	go run ./cmd/server & \
	( cd $(WEB) && $(PNPM) run dev ) & \
	wait
endif

# --- build -------------------------------------------------------------------

build: build-go build-web ## server binary into bin/ + web production build into web/dist

build-go:
ifeq ($(HAS_GO),)
	@echo "build-go: no Go packages yet — skipped"
else
	go build -o bin/server ./cmd/server
endif

build-web:
ifeq ($(HAS_WEB),)
	@echo "build-web: no $(WEB)/src yet — skipped"
else
	cd $(WEB) && $(PNPM) run build
endif

# --- the gate ----------------------------------------------------------------

check: lint test build ## THE GATE. Green before any commit.
	@echo "check: ok"

clean: ## Remove build output
	rm -rf bin $(WEB)/dist $(WEB)/dev-dist
