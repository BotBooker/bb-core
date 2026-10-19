BIN_FILE_API := $(shell test -x ./build/api && echo 1 || echo 0)
COVERAGE_OUT := $(shell test -f coverage.txt && echo 1 || echo 0)
CURRENT_DATE := $(shell date +%FT%T)
GO ?= go
GO_IMPORT_PATH ?= $(shell go list ./...)
GO_LD_FLAGS := -X "main.version=$(shell git describe --tags 2>/dev/null || echo dev) built at $(CURRENT_DATE) on $(shell hostname) with $(shell go version)"
GO_PKG ?= ./cmd/api ./cmd/bootstrap ./cmd/healthcheck ./cmd/migrate
GO_TEST_ARGS ?= "-v"
GO_TEST_FOLDERS  ?= $(shell go list -f '{{ .Dir }}' ./... | grep -vE 'pkg/proto|pkg/api')
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
test: ## Run tests to verify code functionality
test: tools
	@echo "Running tests with coverage report...";
	@set -eu;\
	$(GO) mod tidy;\
	$(GO) test -json -shuffle=on -timeout=5m -count=1 $(GO_TEST_ARGS) $(GO_TEST_FOLDERS) \
		-coverprofile=coverage.txt -covermode=atomic 2>&1 | tee ./gotest-e2e.log | gotestfmt

.PHONY: coverage
coverage: ## Percentage of test coverage. If coverage <80%, output signal 1
ifeq ($(COVERAGE_OUT), 0)
coverage: test
else
coverage:
endif
	@PERCENT=$$($(GO) tool cover -func=coverage.txt | grep total | awk '{print $$3}'); \
	echo "coverage at: $${PERCENT}"; \
	echo $${PERCENT} | sed 's/%//' | xargs -I {} sh -c 'echo "{} < 80" | bc -l | grep -q 1 && exit 1 || exit 0'

.PHONY: tc
tc: ## Run the test and then check the coverage
tc: test coverage

.PHONY: fmt
fmt: ## Ensure consistent code formatting
	@$(GOFMT) -w $(GOFILES)

.PHONY: fmt-check
fmt-check: ## format (check only)
	@diff=$$($(GOFMT) -d $(GOFILES)); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make fmt' and commit the result:"; \
		echo "$${diff}"; \
		exit 1; \
	fi;

.PHONY: vet
vet: ## Examine packages and report suspicious constructs if any
	@$(GO) vet $(VETPACKAGES)

# HOWTO: brew install bearer/tap/bearer
.PHONY: lint
lint: ## Inspect source code for stylistic errors or potential bugs
lint: tools
	@golangci-lint run --fix
	@bearer s .

.PHONY: go-check
go-check: ## To check for dependency vulnerabilities in Go
go-check: tools
	@govulncheck $(GOVULNCHECK_OPTS) ./...

.PHONY: misspell
misspell: ## Correct commonly misspelled English words in source code
	misspell -w $(GOFILES)

.PHONY: misspell-check
misspell-check: ## misspell (check only)
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

.PHONY: build
build: ## Build for DEV
	@rm -f ./build/api
	$(GO) build -ldflags='$(GO_LD_FLAGS)' -o ./build $(GO_PKG)

.PHONY: build-release
build-release: ## Build for release
	@rm -f ./build/api
	CGO_ENABLED=0 $(GO) build -mod=readonly -tags netgo -trimpath -ldflags='-s -w -extldflags "-static" $(GO_LD_FLAGS)' -o ./build/ $(GO_PKG)

.PHONY: clean
clean: ## Clean all cache
	@rm -rf build/* coverage.txt gotest-e2e.log
	@$(GO) clean -modcache

.PHONY: run
run: ## Run API Server
	LOG_LEVEL=INFO go run ./cmd/api

.PHONY: debug
debug: ## Run API Server (mode=debug)
ifeq ($(BIN_FILE_API), 0)
debug: build
else
debug:
endif
	LOG_LEVEL=DEBUG ./build/api

DOCKER ?= docker
DOCKER_IMAGE ?= bb-core
DOCKER_TAG ?= $(shell git rev-parse --short HEAD || echo dev)-$(shell go version | cut -d ' ' -f3)
DOCKERFILE ?= Dockerfile
DOCKER_PORT_HTTP ?= 8080
DOCKER_PORT_GRPC ?= 50051
DOCKER_RUN_ARGS ?=

.PHONY: docker-build
docker-build: ## Test build of the Docker image (tag: dev)
	$(DOCKER) image history -q $(DOCKER_IMAGE):$(DOCKER_TAG) >/dev/null 2>&1 \
	|| $(DOCKER) build \
		--build-arg BUILD_DATE=$(CURRENT_DATE) \
		--build-arg VERSION=$(DOCKER_TAG) \
		--tag $(DOCKER_IMAGE):$(DOCKER_TAG) \
		--file $(DOCKERFILE) .

# SERVER_HOST/SERVER_GRPC_HOST переопределяются на 0.0.0.0: значения из .env
# (localhost) привязали бы сокеты к loopback контейнера и опубликованные порты
# были бы недоступны с хоста.
.PHONY: docker-run
docker-run: ## Run the Docker image (tag: dev); HTTP :8080, gRPC :50051
	$(DOCKER) run --rm \
		--name $(DOCKER_IMAGE)-$(DOCKER_TAG) \
		--publish $(DOCKER_PORT_HTTP):8080 \
		--publish $(DOCKER_PORT_GRPC):50051 \
		--env SERVER_HOST=0.0.0.0 \
		--env SERVER_GRPC_HOST=0.0.0.0 \
		--env DSN \
		--env JWT_SECRET \
		$(DOCKER_RUN_ARGS) \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

##@ Kind (тестовый Kubernetes)

# Тестовый стенд: однонодовый kind-кластер, PostgreSQL внутри кластера и
# bb-core:dev, развёрнутый Helm-чартом. Всё создаётся и удаляется Makefile:
#   make kind-test    — сборка образа, кластер, деплой и smoke-проверка
#   make kind-down    — удалить кластер вместе с содержимым
KIND ?= kind
KUBECTL ?= kubectl
HELM ?= helm
KIND_CLUSTER ?= bb-core-test
KIND_CONTEXT := kind-$(KIND_CLUSTER)
KIND_CONFIG ?= deploy/kind/kind-config.yaml
KIND_NAMESPACE ?= bb-core
KIND_RELEASE ?= bb-core
# Helm fullname схлопывается в имя релиза, т.к. релиз содержит имя чарта.
KIND_SERVICE ?= $(KIND_RELEASE)
KIND_DEPLOYMENT ?= $(KIND_RELEASE)
KIND_CHART ?= deploy/helm/bb-core
KIND_SECRET_NAME ?= bb-core-secrets
KIND_POSTGRES_MANIFEST ?= deploy/kind/postgres.yaml
KIND_POSTGRES_SECRET ?= bb-core-postgres
KIND_POSTGRES_PASSWORD ?= bb-core-test-password
KIND_DSN ?= postgres://bbcore:$(KIND_POSTGRES_PASSWORD)@postgres:5432/bbcore?sslmode=disable
KIND_TIMEOUT ?= 5m
KIND_HTTP_PORT ?= 8080
KIND_GRPC_PORT ?= 50051
# Отдельный порт для smoke-проверки: не конфликтует с локальным API на :8080.
KIND_SMOKE_PORT ?= 18080

.PHONY: kind-up
kind-up: ## Создать тестовый kind-кластер (если ещё нет)
	@$(KIND) get clusters 2>/dev/null | grep -qx "$(KIND_CLUSTER)" \
		&& echo "kind cluster $(KIND_CLUSTER) already exists" \
		|| $(KIND) create cluster --name $(KIND_CLUSTER) --config $(KIND_CONFIG) --wait $(KIND_TIMEOUT)

.PHONY: kind-down
kind-down: ## Удалить тестовый kind-кластер со всем содержимым
	$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: kind-load
kind-load: kind-up docker-build ## Собрать bb-core:dev и загрузить образ в kind
	$(KIND) load docker-image $(DOCKER_IMAGE):$(DOCKER_TAG) --name $(KIND_CLUSTER)

.PHONY: kind-namespace
kind-namespace: kind-up ## Создать namespace для тестового стенда (идемпотентно)
	@$(KUBECTL) --context $(KIND_CONTEXT) create namespace $(KIND_NAMESPACE) \
		--dry-run=client -o yaml | $(KUBECTL) --context $(KIND_CONTEXT) apply -f -

# Секреты создаются один раз: повторный make kind-deploy не ротирует
# JWT_SECRET и не инвалидирует уже выданные токены, а DSN продолжает
# указывать на PostgreSQL внутри кластера.
.PHONY: kind-secrets
kind-secrets: kind-namespace ## Создать Secret для PostgreSQL и bb-core (идемпотентно)
	@$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) get secret $(KIND_POSTGRES_SECRET) >/dev/null 2>&1 \
		|| $(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) create secret generic $(KIND_POSTGRES_SECRET) \
			--from-literal=POSTGRES_PASSWORD='$(KIND_POSTGRES_PASSWORD)'
	@$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) get secret $(KIND_SECRET_NAME) >/dev/null 2>&1 \
		|| $(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) create secret generic $(KIND_SECRET_NAME) \
			--from-literal=DSN='$(KIND_DSN)' \
			--from-literal=JWT_SECRET="$${JWT_SECRET:-$$(openssl rand -base64 32)}"

.PHONY: kind-postgres
kind-postgres: kind-secrets ## Развернуть PostgreSQL для тестового стенда
	$(KUBECTL) --context $(KIND_CONTEXT) apply -n $(KIND_NAMESPACE) -f $(KIND_POSTGRES_MANIFEST)
	$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) rollout status deployment/postgres --timeout=$(KIND_TIMEOUT)

.PHONY: kind-deploy
kind-deploy: kind-load kind-postgres ## Развернуть bb-core:dev в kind через Helm
	@# Тег dev при повторной сборке не меняется, поэтому Helm не видит diff в
	@# pod-шаблоне и оставляет старые pod'ы. Перезапуск ДО helm upgrade
	@# гарантирует, что они возьмут только что загруженный в kind образ и не
	@# пересекутся с первым применением миграций при установке с нуля.
	@if $(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) get deployment/$(KIND_DEPLOYMENT) >/dev/null 2>&1; then \
		$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) rollout restart deployment/$(KIND_DEPLOYMENT); \
	fi
	$(HELM) upgrade --install $(KIND_RELEASE) $(KIND_CHART) \
		--kube-context $(KIND_CONTEXT) \
		--namespace $(KIND_NAMESPACE) \
		--set image.repository=$(DOCKER_IMAGE) \
		--set image.tag=$(DOCKER_TAG) \
		--set image.pullPolicy=IfNotPresent \
		--set existingSecret=$(KIND_SECRET_NAME) \
		--set config.publicBaseURL=http://localhost:$(KIND_HTTP_PORT) \
		--set config.logLevel=DEBUG
	$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) rollout status deployment/$(KIND_DEPLOYMENT) --timeout=$(KIND_TIMEOUT)

.PHONY: kind-status
kind-status: ## Показать pods, services и статус Helm-релиза в kind
	$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) get pods,svc
	$(HELM) status $(KIND_RELEASE) --kube-context $(KIND_CONTEXT) --namespace $(KIND_NAMESPACE)

.PHONY: kind-logs
kind-logs: ## Последние 200 строк логов API в kind
	$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) logs \
		-l app.kubernetes.io/instance=$(KIND_RELEASE) --all-containers --tail=200

.PHONY: kind-port-forward
kind-port-forward: ## Пробросить HTTP :8080 и gRPC :50051 из kind на localhost (Ctrl+C — выход)
	$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) port-forward \
		service/$(KIND_SERVICE) $(KIND_HTTP_PORT):8080 $(KIND_GRPC_PORT):50051

.PHONY: kind-smoke
kind-smoke: ## Smoke-проверка API в kind: GET /health через port-forward
	@echo "Проверка /health через port-forward на 127.0.0.1:$(KIND_SMOKE_PORT)..."
	@$(KUBECTL) --context $(KIND_CONTEXT) -n $(KIND_NAMESPACE) port-forward \
		service/$(KIND_SERVICE) $(KIND_SMOKE_PORT):8080 >/dev/null 2>&1 & \
		PF_PID=$$!; \
		trap 'kill $$PF_PID 2>/dev/null' EXIT INT TERM; \
		for attempt in $$(seq 1 20); do \
			if curl -fsS "http://127.0.0.1:$(KIND_SMOKE_PORT)/health" >/dev/null 2>&1; then \
				echo "health check OK: http://127.0.0.1:$(KIND_SMOKE_PORT)/health"; \
				exit 0; \
			fi; \
			sleep 1; \
		done; \
		echo "health check failed: service/$(KIND_SERVICE) не ответил за 20 секунд" >&2; \
		exit 1

.PHONY: kind-test
kind-test: kind-deploy kind-smoke ## Полный тестовый прогон: образ dev, kind-кластер, Helm-деплой, smoke

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

.PHONY: migrate-up
migrate-up: ## Применить миграции вручную (DSN из окружения или .env)
	$(GO) run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Откатить последнюю применённую миграцию
	$(GO) run ./cmd/migrate down

.PHONY: bootstrap
bootstrap: ## Создать первого суперпользователя (SUPERUSER_EMAIL/SUPERUSER_PASSWORD из .env)
	$(GO) run ./cmd/bootstrap
