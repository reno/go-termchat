.DEFAULT_GOAL := help
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

BUF_VERSION        := 1.72.0
GOLANGCI_VERSION   := 2.12.2

GOBIN              := $(shell go env GOPATH)/bin
BUF                := $(GOBIN)/buf
GOLANGCI           := $(GOBIN)/golangci-lint

WEB                := web
PNPM               := corepack pnpm
export COREPACK_ENABLE_DOWNLOAD_PROMPT := 0

HAS_PROTO   := $(shell find proto -name '*.proto' -print -quit 2>/dev/null)
HAS_GO      := $(shell find cmd internal gen -name '*.go' -print -quit 2>/dev/null)
HAS_SERVER  := $(wildcard cmd/server)
HAS_WEB_SRC := $(shell find $(WEB)/src -name '*.ts*' -print -quit 2>/dev/null)
HAS_WEB_APP := $(wildcard $(WEB)/index.html)
HAS_E2E     := $(shell find e2e -name '*.ts' -print -quit 2>/dev/null)

.PHONY: help setup gen lint lint-proto lint-go lint-web fmt test test-go test-web e2e dev build build-go build-web check clean

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

setup: ## Install tool deps and web dependencies
	go install github.com/bufbuild/buf/cmd/buf@v$(BUF_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_VERSION)
	corepack enable pnpm
	cd $(WEB) && $(PNPM) install
	@echo "setup: ok"
	@echo "  hooks: opt in with 'git config core.hooksPath .githooks' (runs make check pre-commit)"

gen: ## buf generate -> gen/go + web/src/gen (run after ANY proto edit)
ifeq ($(HAS_PROTO),)
	@echo "gen: no proto/ yet — skipped"
else
	$(BUF) generate
endif

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
ifeq ($(HAS_WEB_SRC),)
	@echo "lint-web: no $(WEB)/src yet — skipped"
else
	cd $(WEB) && $(PNPM) run format
	cd $(WEB) && $(PNPM) run typecheck
endif
ifneq ($(HAS_WEB_APP),)
	cd $(WEB) && $(PNPM) run lint
endif

fmt: ## Apply formatters in place
ifneq ($(HAS_GO),)
	$(GOLANGCI) fmt
endif
ifneq ($(HAS_WEB_SRC),)
	cd $(WEB) && $(PNPM) exec prettier --write .
endif
	@echo "fmt: ok"

test: test-go test-web ## go test -race + web unit tests

test-go:
ifeq ($(HAS_GO),)
	@echo "test-go: no Go packages yet — skipped"
else
	go test ./... -race
endif

test-web:
ifeq ($(HAS_WEB_APP),)
	@echo "test-web: no Vite app yet — skipped"
else
	cd $(WEB) && $(PNPM) run test
endif

e2e: ## Playwright
ifeq ($(HAS_E2E),)
	@echo "e2e: no e2e/ yet — skipped"
else
	cd $(WEB) && $(PNPM) exec playwright test
endif

dev: ## Go server on :8080 + Vite on :5173 (proxying /chat.v1 -> :8080)
ifeq ($(HAS_SERVER)$(HAS_WEB_APP),)
	@echo "dev: nothing to run yet — skipped"
else
	@trap 'kill 0' EXIT INT TERM; \
	go run ./cmd/server & \
	( cd $(WEB) && $(PNPM) run dev ) & \
	wait
endif

build: build-go build-web ## server binary into bin/ + web production build into web/dist

build-go:
ifneq ($(HAS_GO),)
	go build ./...
endif
ifeq ($(HAS_SERVER),)
	@echo "build-go: no cmd/server yet — binary skipped"
else
	go build -o bin/server ./cmd/server
endif

build-web:
ifeq ($(HAS_WEB_APP),)
	@echo "build-web: no Vite app yet — skipped"
else
	cd $(WEB) && $(PNPM) run build
endif

check: lint test build ## THE GATE. Green before any commit.
	@echo "check: ok"

clean: ## Remove build output
	rm -rf bin $(WEB)/dist $(WEB)/dev-dist
