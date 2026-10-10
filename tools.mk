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
