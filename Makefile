BIN_FILE_API := $(shell test -x ./build/api && echo 1 || echo 0)
COVERAGE_OUT := $(shell test -f coverage.txt && echo 1 || echo 0)
GO ?= go
GO_IMPORT_PATH ?= $(shell go list ./...)
GO_TESTFOLDER  ?= $(shell go list -f '{{ .Dir }}' ./... | grep -vE 'pkg/proto|pkg/api')
GO_TESTTAGS ?= "-v"
GO_VERSION=$(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
GOBIN = $(shell go env GOPATH)/bin
GOFILES := $(shell find . -name "*.go")
GOFMT ?= gofmt "-s"
PACKAGES ?= $(shell $(GO) list ./...)
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /examples/)

# fix version tools
BUF_VERSION ?= v1.73.0
EASYP_VERSION ?= v0.17.0
GOFUMPT_VERSION ?= v0.12.0
GOIMPORTS_VERSION ?= v0.51.0
GOLANGCI_LINT_VERSION ?= v2.14.0
GOOSE_VERSION ?= v3.28.0
GOTESTFMT_VERSION ?= v2.5.0
GOVULNCHECK_VERSION ?= v1.8.0
GRPCURL_VERSION ?= v1.9.4
MISSPELL_VERSION ?= v0.8.0
PROTOC_GEN_GO_GRPC_VERSION ?= v1.6.2
PROTOC_GEN_GO_VERSION ?= v1.36.12
PROTOC_GEN_GRPC_GATEWAY_VERSION ?= v2.31.0
PROTOC_GEN_OPENAPIV2_VERSION ?= v2.31.0
PROTOC_GEN_VALIDATE_VERSION ?= v1.3.3

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
	$(GO) test -json -shuffle=on -timeout=5m -count=1 $(GO_TESTTAGS) $(GO_TESTFOLDER) \
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
	@govulncheck ./...

.PHONY: misspell
misspell: ## Correct commonly misspelled English words in source code.
	misspell -w $(GOFILES)

.PHONY: misspell-check
misspell-check: ## misspell (check only).
	misspell -error $(GOFILES)

TOOLS = buf easyp gofumpt goimports golangci-lint goose gotestfmt govulncheck grpcurl misspell protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2 protoc-gen-validate protoc-gen-validate-go

TOOLS_BIN = $(addprefix $(GOBIN)/, $(TOOLS))

.PHONY: tools
tools: $(TOOLS_BIN) ## Install Go tools
	@command -v goenv >/dev/null 2>&1 && goenv rehash >/dev/null 2>&1 || true

# Install specific utilities only if they are missing
$(GOBIN)/buf:
	$(GO) install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

$(GOBIN)/easyp:
	$(GO) install github.com/easyp-tech/easyp/cmd/easyp@$(EASYP_VERSION)

$(GOBIN)/protoc-gen-go:
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)

$(GOBIN)/protoc-gen-go-grpc:
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

$(GOBIN)/protoc-gen-validate:
	$(GO) install github.com/envoyproxy/protoc-gen-validate@$(PROTOC_GEN_VALIDATE_VERSION)

$(GOBIN)/protoc-gen-validate-go:
	$(GO) install github.com/envoyproxy/protoc-gen-validate/cmd/protoc-gen-validate-go@$(PROTOC_GEN_VALIDATE_VERSION)

$(GOBIN)/protoc-gen-grpc-gateway:
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@$(PROTOC_GEN_GRPC_GATEWAY_VERSION)

$(GOBIN)/protoc-gen-openapiv2:
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@$(PROTOC_GEN_OPENAPIV2_VERSION)

$(GOBIN)/grpcurl:
	$(GO) install github.com/fullstorydev/grpcurl/cmd/grpcurl@$(GRPCURL_VERSION)

$(GOBIN)/gofumpt:
	$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOBIN)/goimports:
	$(GO) install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(GOBIN)/golangci-lint:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOBIN)/goose:
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

$(GOBIN)/gotestfmt:
	$(GO) install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@$(GOTESTFMT_VERSION)

$(GOBIN)/govulncheck:
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GOBIN)/misspell:
	$(GO) install github.com/golangci/misspell/cmd/misspell@$(MISSPELL_VERSION)

.PHONY: deps
deps: ## Install dependencies
	$(GO) mod verify
	$(GO) mod tidy

.PHONY: build-debug
build-debug: ## Build for DEV
	@rm -f ./build/api
	$(GO) build -o ./build/api ./cmd/api

.PHONY: build
build: ## Build for release
	@rm -f ./build/api
	CGO_ENABLED=0 $(GO) build -mod=readonly -tags netgo -trimpath -ldflags='-s -w -extldflags "-static"' -o ./build/api ./cmd/api

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
