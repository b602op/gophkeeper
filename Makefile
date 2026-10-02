# Makefile для GophKeeper.
# Все цели рассчитаны на запуск из корня репозитория.

MODULE      := github.com/b602op/gophkeeper
BIN_DIR     := bin
SERVER_BIN  := $(BIN_DIR)/gophkeeper-server
CLIENT_BIN  := $(BIN_DIR)/gophkeeper-client
COVER_FILE  := coverage.out
COVER_HTML  := coverage.html

# Метаданные сборки прокидываются через ldflags в internal/buildinfo.
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)

LDFLAGS := -X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
           -X $(MODULE)/internal/buildinfo.BuildDate=$(BUILD_DATE) \
           -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT)

# Параметры миграций.
MIGRATIONS_DIR ?= internal/repository/migrations
GOOSE_DSN      ?= postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable

# Каталог для самоподписанных сертификатов.
CERT_DIR := certs

.PHONY: help
help: ## Показать список целей
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Собрать сервер и клиент с метаданными сборки
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(SERVER_BIN) ./cmd/server
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(CLIENT_BIN) ./cmd/client

.PHONY: run-server
run-server: ## Запустить сервер
	go run -ldflags "$(LDFLAGS)" ./cmd/server

.PHONY: run-client
run-client: ## Запустить клиент
	go run -ldflags "$(LDFLAGS)" ./cmd/client

.PHONY: test
test: ## Запустить все тесты
	go test ./... -count=1

.PHONY: cover
cover: ## Посчитать покрытие и собрать HTML-отчёт
	go test ./... -count=1 -coverprofile=$(COVER_FILE)
	go tool cover -func=$(COVER_FILE)
	go tool cover -html=$(COVER_FILE) -o $(COVER_HTML)

.PHONY: lint
lint: ## Запустить go vet и статический анализатор staticlint
	go vet ./...
	go run ./cmd/staticlint ./...

.PHONY: migrate-up
migrate-up: ## Применить миграции
	goose -dir $(MIGRATIONS_DIR) postgres "$(GOOSE_DSN)" up

.PHONY: migrate-down
migrate-down: ## Откатить последнюю миграцию
	goose -dir $(MIGRATIONS_DIR) postgres "$(GOOSE_DSN)" down

.PHONY: certs
certs: ## Сгенерировать самоподписанные TLS-сертификаты
	@mkdir -p $(CERT_DIR)
	openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
		-keyout $(CERT_DIR)/server.key -out $(CERT_DIR)/server.crt \
		-subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

.PHONY: check
check: build test lint ## Выполнить все проверки одной командой
	@echo "Все проверки пройдены"
