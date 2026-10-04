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

# Цвета для вывода
GREEN  := \033[0;32m
YELLOW := \033[0;33m
CYAN   := \033[0;36m
RESET  := \033[0m

.PHONY: help
help: ## Показать список целей
	@echo "Доступные цели:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-16s$(RESET) %s\n", $$1, $$2}'

# =============================================================================
# Сборка
# =============================================================================

.PHONY: build
build: ## Собрать сервер и клиент с метаданными сборки
	@echo "=== Build ==="
	@mkdir -p $(BIN_DIR)
	@go build -trimpath -ldflags "$(LDFLAGS)" -o $(SERVER_BIN) ./cmd/server
	@go build -trimpath -ldflags "$(LDFLAGS)" -o $(CLIENT_BIN) ./cmd/client
	@echo "$(GREEN) Build OK$(RESET)"
	@echo ""

# =============================================================================
# Запуск
# =============================================================================

.PHONY: run-server
run-server: ## Запустить сервер
	@echo "=== Run server ==="
	@go run -ldflags "$(LDFLAGS)" ./cmd/server

.PHONY: run-client
run-client: ## Запустить клиент (аргументы: make run-client ARGS="version")
	@echo "=== Run client $(ARGS) ==="
	@go run -ldflags "$(LDFLAGS)" ./cmd/client $(ARGS)

# =============================================================================
# Тесты
# =============================================================================

.PHONY: test
test: ## Запустить все тесты
	@echo "=== Test ==="
	@go test ./... -count=1
	@echo "$(GREEN) Test OK$(RESET)"
	@echo ""

.PHONY: cover
cover: ## Посчитать покрытие и собрать HTML-отчёт
	@echo "=== Coverage ==="
	@go test ./... -count=1 -coverprofile=$(COVER_FILE)
	@go tool cover -func=$(COVER_FILE) | grep total
	@go tool cover -html=$(COVER_FILE) -o $(COVER_HTML)
	@echo "$(GREEN) Coverage OK$(RESET) → $(COVER_HTML)"
	@echo ""

# =============================================================================
# Линтинг
# =============================================================================

.PHONY: lint
lint: ## Запустить go vet и статический анализатор staticlint
	@echo "=== Lint ==="
	@go vet ./...
	@go run ./cmd/staticlint ./...
	@echo "$(GREEN) Lint OK$(RESET)"
	@echo ""

# =============================================================================
# Миграции
# =============================================================================

.PHONY: migrate-up
migrate-up: ## Применить миграции
	@echo "=== Migrate up ==="
	@goose -dir $(MIGRATIONS_DIR) postgres "$(GOOSE_DSN)" up
	@echo "$(GREEN) Migrations applied$(RESET)"

.PHONY: migrate-down
migrate-down: ## Откатить последнюю миграцию
	@echo "=== Migrate down ==="
	@goose -dir $(MIGRATIONS_DIR) postgres "$(GOOSE_DSN)" down
	@echo "$(GREEN) Migration rolled back$(RESET)"

.PHONY: migrate-status
migrate-status: ## Показать статус миграций
	@goose -dir $(MIGRATIONS_DIR) postgres "$(GOOSE_DSN)" status

# =============================================================================
# Сертификаты
# =============================================================================

.PHONY: certs
certs: ## Сгенерировать самоподписанные TLS-сертификаты
	@echo "=== Generate certs ==="
	@mkdir -p $(CERT_DIR)
	@openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
		-keyout $(CERT_DIR)/server.key -out $(CERT_DIR)/server.crt \
		-subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
	@echo "$(GREEN) Certificates generated in $(CERT_DIR)/$(RESET)"

# =============================================================================
# Полная проверка (build + test + lint + cover)
# =============================================================================

.PHONY: check
check: build test lint cover ## Выполнить все проверки одной командой
	@echo ""
	@echo "======================================"
	@echo "  $(GREEN) ВСЕ ПРОВЕРКИ ПРОЙДЕНЫ$(RESET)"
	@echo "======================================"

# =============================================================================
# Очистка
# =============================================================================

.PHONY: clean
clean: ## Удалить артефакты сборки и отчёты
	@echo "=== Clean ==="
	@rm -rf $(BIN_DIR) $(COVER_FILE) $(COVER_HTML)
	@echo "$(GREEN) Clean OK$(RESET)"