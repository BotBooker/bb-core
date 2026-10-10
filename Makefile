BIN_FILE_API := $(shell test -x ./build/api && echo 1 || echo 0)
COVERAGE_OUT := $(shell test -f coverage.txt && echo 1 || echo 0)
CURRENT_DATE := $(shell date +%FT%T)
GO ?= go
GO_IMPORT_PATH ?= $(shell go list ./...)
GO_LD_FLAGS := -X "main.version=$(shell git describe --tags 2>/dev/null || echo dev) built at $(CURRENT_DATE) on $(shell hostname) with $(shell go version)"
GO_TEST_FOLDERS  ?= $(shell go list -f '{{ .Dir }}' ./... | grep -vE 'pkg/proto|pkg/api')
GO_TEST_ARGS ?= "-v"
GO_VERSION=$(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
GOBIN = $(shell go env GOPATH)/bin
GOFILES := $(shell find . -name "*.go")
GOFMT ?= gofmt "-s"
GOVULNCHECK_OPTS ?= -show color
PACKAGES ?= $(shell $(GO) list ./...)
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /examples/)

-include .env
export

-include vars.mk

export PATH := $(GOBIN):$(PATH)

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@echo "\033[1;3;34mBotBooker core Go.\033[0m\n"
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_0-9\/-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: test
test: ## Run tests to verify code functionality.
test: tools
	@echo "Running tests with coverage report...";
	@set -eu;\
	$(GO) mod tidy;\
	$(GO) test -json -shuffle=on -timeout=5m -count=1 $(GO_TEST_ARGS) $(GO_TEST_FOLDERS) \
		-coverprofile=coverage.txt -covermode=atomic 2>&1 | tee ./gotest-e2e.log | gotestfmt

.PHONY: coverage
coverage: ## Percentage of test coverage. If coverage <80%, output signal 1.
ifeq ($(COVERAGE_OUT), 0)
coverage: test
else
coverage:
endif
	@PERCENT=$$($(GO) tool cover -func=coverage.txt | grep total | awk '{print $$3}'); \
	echo "coverage at: $${PERCENT}"; \
	echo $${PERCENT} | sed 's/%//' | xargs -I {} sh -c 'echo "{} < 80" | bc -l | grep -q 1 && exit 1 || exit 0'

.PHONY: fmt
fmt: ## Ensure consistent code formatting.
	@$(GOFMT) -w $(GOFILES)

.PHONY: fmt-check
fmt-check: ## format (check only).
	@diff=$$($(GOFMT) -d $(GOFILES)); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make fmt' and commit the result:"; \
		echo "$${diff}"; \
		exit 1; \
	fi;

.PHONY: vet
vet: ## Examine packages and report suspicious constructs if any.
	@$(GO) vet $(VETPACKAGES)

.PHONY: lint
lint: ## Inspect source code for stylistic errors or potential bugs.
lint: tools
	@golangci-lint run --fix

.PHONY: go-check
go-check: ## To check for dependency vulnerabilities in Go
go-check: tools
	@govulncheck $(GOVULNCHECK_OPTS) ./...

.PHONY: misspell
misspell: ## Correct commonly misspelled English words in source code.
	misspell -w $(GOFILES)

.PHONY: misspell-check
misspell-check: ## misspell (check only).
	misspell -error $(GOFILES)

TOOLS = buf easyp gofumpt goimports golangci-lint goose gotestfmt govulncheck grpcurl misspell protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2 protoc-gen-validate protoc-gen-validate-go

TOOLS_BIN = $(addprefix $(GOBIN)/, $(TOOLS))

.PHONY: tools
tools:
-include tools.mk
tools: $(TOOLS_BIN) ## Install Go tools
	@command -v goenv >/dev/null 2>&1 && goenv rehash >/dev/null 2>&1 || true

.PHONY: deps
deps: ## Install dependencies
	$(GO) mod verify
	$(GO) mod tidy

.PHONY: build-debug
build-debug: ## Build for DEV
	@rm -f ./build/api
	$(GO) build -ldflags='$(GO_LD_FLAGS)' -o ./build/api ./cmd/api

.PHONY: build
build: ## Build for release
	@rm -f ./build/api
	CGO_ENABLED=0 $(GO) build -mod=readonly -tags netgo -trimpath -ldflags='-s -w -extldflags "-static" $(GO_LD_FLAGS)' -o ./build/api ./cmd/api

.PHONY: clean
clean: ## Clean all cache
	@rm -f coverage.txt gotest-e2e.log
	@$(GO) clean -modcache

.PHONY: run
run: ## Run API Server
	LOG_LEVEL=INFO go run ./cmd/api

.PHONY: debug
debug: ## Run API Server (mode=debug)
ifeq ($(BIN_FILE_API), 0)
debug: build-debug
else
debug:
endif
	LOG_LEVEL=DEBUG ./build/api

.PHONY: proto/buf-lint
proto/buf-lint: ## Проверка .proto на соответствие стилю (buf)
	$(MAKE) -C proto buf-lint

.PHONY: proto/buf-deps
proto/buf-deps: ## Скачивает protobuf-зависимости, объявленные в buf.yaml (пишет buf.lock)
	$(MAKE) -C proto buf-deps

.PHONY: proto/buf-gen
proto/buf-gen: ## Генерирует Go код из proto через buf
	$(MAKE) -C proto buf-gen

.PHONY: proto/easyp-lint
proto/easyp-lint: ## Проверка .proto на соответствие стилю (easyp)
	$(MAKE) -C proto easyp-lint

.PHONY: proto/easyp-deps
proto/easyp-deps: ## Скачивает protobuf-зависимости, объявленные в easyp.yaml (пишет easyp.lock)
	$(MAKE) -C proto easyp-deps

.PHONY: proto/easyp-gen
proto/easyp-gen: ## Генерирует Go код из proto через easyp
	$(MAKE) -C proto easyp-gen
