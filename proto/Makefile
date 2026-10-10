.PHONY: buf-lint
buf-lint: ## Проверка .proto на соответствие стилю (buf)
	@buf lint

.PHONY: buf-deps
buf-deps: ## Скачивает protobuf-зависимости, объявленные в buf.yaml (пишет buf.lock)
	@buf dep update

.PHONY: buf-gen
buf-gen: ## Генерирует Go код из proto через buf
	@mkdir -p ../pkg/proto ../pkg/api/swagger
	@buf generate

.PHONY: easyp-lint
easyp-lint: ## Проверка .proto на соответствие стилю (easyp)
	@easyp --config easyp.yaml lint

.PHONY: easyp-deps
easyp-deps: ## Скачивает protobuf-зависимости, объявленные в easyp.yaml (пишет easyp.lock)
	@easyp --config easyp.yaml mod download

.PHONY: easyp-gen
easyp-gen: ## Генерирует Go код из proto через easyp
	@mkdir -p ../pkg/proto ../pkg/api/swagger
	@easyp --config easyp.yaml generate
