BUF_VERSION ?= v1.74.0
EASYP_VERSION ?= v0.17.0
GOFUMPT_VERSION ?= v0.12.0
GOIMPORTS_VERSION ?= v0.51.0
GOLANGCI_LINT_VERSION ?= v2.15.0
GOOSE_VERSION ?= v3.29.0
GOTESTFMT_VERSION ?= v2.5.0
GOVULNCHECK_VERSION ?= v1.8.0
GRPCURL_VERSION ?= v1.9.4
MISSPELL_VERSION ?= v0.8.0
PROTOC_GEN_GO_GRPC_VERSION ?= v1.6.2
PROTOC_GEN_GO_VERSION ?= v1.36.12
PROTOC_GEN_GRPC_GATEWAY_VERSION ?= v2.32.0
PROTOC_GEN_OPENAPIV2_VERSION ?= v2.32.0
PROTOC_GEN_VALIDATE_VERSION ?= v1.3.3

# Install specific utilities only if they are missing
$(GOBIN)/buf:
	@buf --version 2>/dev/null | grep -q '$(BUF_VERSION)' || \
	$(GO) install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

$(GOBIN)/easyp:
	@easyp --version 2>/dev/null | grep -q '$(EASYP_VERSION)' || \
	$(GO) install github.com/easyp-tech/easyp/cmd/easyp@$(EASYP_VERSION)

$(GOBIN)/protoc-gen-go:
	@protoc-gen-go --version 2>/dev/null | grep -q '$(PROTOC_GEN_GO_VERSION)' || \
	$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)

$(GOBIN)/protoc-gen-go-grpc:
	@protoc-gen-go-grpc --version 2>/dev/null | grep -q '$(PROTOC_GEN_GO_GRPC_VERSION)' || \
	$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

$(GOBIN)/protoc-gen-validate:
	@protoc-gen-validate --version 2>/dev/null | grep -q '$(PROTOC_GEN_VALIDATE_VERSION)' || \
	$(GO) install github.com/envoyproxy/protoc-gen-validate@$(PROTOC_GEN_VALIDATE_VERSION)

$(GOBIN)/protoc-gen-validate-go:
	@protoc-gen-validate-go --version 2>/dev/null | grep -q '$(PROTOC_GEN_VALIDATE_VERSION)' || \
	$(GO) install github.com/envoyproxy/protoc-gen-validate/cmd/protoc-gen-validate-go@$(PROTOC_GEN_VALIDATE_VERSION)

$(GOBIN)/protoc-gen-grpc-gateway:
	@protoc-gen-grpc-gateway --version 2>/dev/null | grep -q '$(PROTOC_GEN_GRPC_GATEWAY_VERSION)' || \
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@$(PROTOC_GEN_GRPC_GATEWAY_VERSION)

$(GOBIN)/protoc-gen-openapiv2:
	@protoc-gen-openapiv2 --version 2>/dev/null | grep -q '$(PROTOC_GEN_OPENAPIV2_VERSION)' || \
	$(GO) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@$(PROTOC_GEN_OPENAPIV2_VERSION)

$(GOBIN)/grpcurl:
	$(GO) install github.com/fullstorydev/grpcurl/cmd/grpcurl@$(GRPCURL_VERSION)

$(GOBIN)/gofumpt:
	$(GO) install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOBIN)/goimports:
	$(GO) install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(GOBIN)/golangci-lint:
	@golangci-lint --version 2>/dev/null | grep -q '$(GOLANGCI_LINT_VERSION)' || \
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOBIN)/goose:
	@goose --version 2>/dev/null | grep -q '$(GOOSE_VERSION)' || \
	$(GO) install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)

$(GOBIN)/gotestfmt:
	$(GO) install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@$(GOTESTFMT_VERSION)

$(GOBIN)/govulncheck:
	@govulncheck --version 2>/dev/null | grep -q '$(GOVULNCHECK_VERSION)' || \
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GOBIN)/misspell:
	$(GO) install github.com/golangci/misspell/cmd/misspell@$(MISSPELL_VERSION)
