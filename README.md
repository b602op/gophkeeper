# GophKeeper

Менеджер паролей: клиент-серверная система для хранения логинов/паролей,
текста, бинарных данных и карт. У каждой записи может быть текстовая
метаинформация (сайт, личность, банк и т.п.).

## Что умеет

- Регистрация и вход по JWT.
- Хранение секретов на сервере (PostgreSQL).
- Синхронизация между устройствами одного пользователя.
- CLI для Windows, Linux и macOS с выводом версии и даты сборки.
- Шифрование на клиенте (AES-256-GCM, ключ из мастер-пароля).
  Сервер видит только шифротекст.

## Требования

- Go 1.26+
- PostgreSQL 16 (или Docker)
- goose для миграций: `go install github.com/pressly/goose/v3/cmd/goose@latest`

## Архитектура

```
handlers (HTTP/CLI) -> service -> repository -> domain
```

- `internal/domain` — модели и ошибки.
- `internal/config` — конфигурация: флаги > env > файл > дефолты
- `internal/repository` — PostgreSQL через pgx + миграции goose
- `internal/service` — бизнес-логика, общая для HTTP и CLI
- `internal/handler` — HTTP-хендлеры
- `internal/middleware` — JWT, логирование, recovery
- `internal/logger` — `log/slog`
- `internal/crypto`, `storage`, `api`, `session`, `cli` — клиентская часть

Подробная — в [docs/architecture.md](docs/architecture.md).

## Быстрый старт

```bash
# 1. Клонировать репозиторий
git clone https://github.com/b602op/gophkeeper
cd gophkeeper

# 2. Поднять PostgreSQL
docker compose up -d

# 3. Применить миграции
goose -dir internal/repository/migrations \
  postgres "postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable" up

# 4. Собрать сервер и клиент
make build

# 5. Запустить сервер
./bin/gophkeeper-server -a :8080 \
  -d "postgres://gophkeeper:gophkeeper@localhost:5432/gophkeeper?sslmode=disable" \
  -jwt-secret "change-me-please-1234"

# 6. Зарегистрироваться
./bin/gophkeeper-client register --login alice --password secret123

# 7. Войти (запросит мастер-пароль для шифрования)
./bin/gophkeeper-client login --login alice --password secret123

# 8. Добавить секрет
./bin/gophkeeper-client add "GitHub" --type credentials \
  --username alice --password gh_pass

# 9. Синхронизировать с сервером
./bin/gophkeeper-client sync
```

Если порт `5432` занят, задайте свободный порт:

```bash
GOPHKEEPER_PG_PORT=5433 docker compose up -d
```


## Конфигурация сервера

Приоритет источников: **флаги > переменные окружения > JSON-файл > значения по
умолчанию**. Пример всех опций — в `config.example.json`, список переменных — в
`.env.example`.

| Флаг | Переменная окружения | Поле JSON | Назначение |
| --- | --- | --- | --- |
| `-a` | `GOPHKEEPER_RUN_ADDRESS` | `run_address` | адрес прослушивания |
| `-d` | `GOPHKEEPER_DATABASE_DSN` | `database_dsn` | DSN PostgreSQL |
| `-jwt-secret` | `GOPHKEEPER_JWT_SECRET` | `jwt_secret` | секрет подписи JWT (обязателен) |
| `-token-ttl` | `GOPHKEEPER_TOKEN_TTL` | `token_ttl` | время жизни токена |
| `-log-level` | `GOPHKEEPER_LOG_LEVEL` | `log_level` | уровень логирования |
| `-https` | `GOPHKEEPER_ENABLE_HTTPS` | `enable_https` | включить HTTPS |
| `-tls-cert` | `GOPHKEEPER_TLS_CERT_FILE` | `tls_cert_file` | путь к сертификату |
| `-tls-key` | `GOPHKEEPER_TLS_KEY_FILE` | `tls_key_file` | путь к ключу |
| `-bcrypt-cost` | `GOPHKEEPER_BCRYPT_COST` | `bcrypt_cost` | стоимость bcrypt |
| `-migrations` | `GOPHKEEPER_MIGRATIONS_DIR` | `migrations_dir` | каталог миграций |
| `-c` | `CONFIG_FILE` | — | путь к JSON-файлу конфигурации |

Секрет JWT обязателен: без него сервер завершается с понятной ошибкой.

## API

Все защищённые маршруты требуют заголовок `Authorization: Bearer <JWT>`.
Ошибки возвращаются в едином формате `{"error": "..."}`.

| Метод | Путь | Описание |
| --- | --- | --- |
| `GET` | `/health` | проверка живости сервера |
| `POST` | `/api/v1/register` | регистрация пользователя |
| `POST` | `/api/v1/login` | аутентификация, выдача JWT |
| `GET` | `/api/v1/secrets` | список неудалённых секретов |
| `POST` | `/api/v1/secrets` | создать секрет |
| `GET` | `/api/v1/secrets/{id}` | получить секрет |
| `PUT` | `/api/v1/secrets/{id}` | обновить секрет |
| `DELETE` | `/api/v1/secrets/{id}` | удалить секрет (soft delete) |
| `GET` | `/api/v1/sync?since=<RFC3339>` | синхронизация изменений |

Полное описание запросов, ответов, кодов и примеров — в
[docs/api.md](docs/api.md).

### Секреты

Тело запроса создания и обновления:

```json
{
  "type": "credentials",
  "name": "Почта",
  "metadata": "личное",
  "data": "<base64 от шифротекста>",
  "version": 1
}
```

- `type` — `credentials`, `text`, `binary` или `card`.
- `data` — зашифрованные на клиенте данные (сервер видит только шифротекст).
- `metadata` — произвольная текстовая метаинформация (до 64 КиБ).
- `data` — до 1 МиБ, `name` — до 255 символов.

Обновление использует оптимистичную блокировку по `version`: если запись уже
изменена другим клиентом, сервер отвечает `409 Conflict` с ошибкой
`ErrSecretVersionMismatch`.

### Синхронизация

`GET /api/v1/sync?since=<RFC3339>` возвращает все записи, изменённые строго
после указанного момента, **включая удалённые** (`deleted_at != nil`) — так
клиенты узнают об удалениях. Без параметра `since` возвращаются все неудалённые
секреты. Пример: `?since=2026-01-15T10:30:00Z`.

### Коды ошибок

| Ошибка | HTTP |
| --- | --- |
| `ErrValidation`, `ErrInvalidSecretType`, `ErrInvalidSecretData` | 400 |
| `ErrInvalidCredentials`, `ErrInvalidToken` | 401 |
| `ErrForbidden` | 403 (зарезервирован для ролей и операций над коллекцией) |
| `ErrUserNotFound`, `ErrSecretNotFound` | 404 (в том числе чужой секрет по ID) |
| `ErrUserAlreadyExists`, `ErrSecretAlreadyExists`, `ErrSecretVersionMismatch` | 409 |
| внутренняя ошибка | 500 |

Секреты адресуются по ID, и факт их существования скрыт: обращение к чужому
секрету неотличимо от обращения к несуществующему и даёт `404`, а не `403`
(защита от перебора идентификаторов). Код `403` остаётся для случаев, когда
ресурс в принципе виден пользователю, но действие ему запрещено (например,
проверки ролей или операции над коллекцией).

## CLI

Клиент шифрует данные мастер-паролем (Argon2id + AES-256-GCM) до отправки на
сервер. Пароль сервера и мастер-пароль можно передавать флагами (для скриптов)
или вводить интерактивно (пароли не отображаются).

| Команда | Назначение | Пример |
| --- | --- | --- |
| `register` | регистрация и вход | `gophkeeper register --login alice --password secret123` |
| `login` | вход и ввод мастер-пароля | `gophkeeper login --login alice --password secret123` |
| `logout` | выход, очистка токена и ключа | `gophkeeper logout` |
| `add <name>` | добавить секрет | `gophkeeper add "GitHub" --type credentials --username alice --password gh_pass` |
| `get <id>` | получить и расшифровать | `gophkeeper get <id>` |
| `list` | список записей | `gophkeeper list` |
| `search <query>` | поиск по названию и метаданным | `gophkeeper search GitHub` |
| `update <id>` | обновить запись | `gophkeeper update <id> --password new_pass` |
| `delete <id>` | удалить запись (soft delete) | `gophkeeper delete <id>` |
| `sync` | синхронизация с сервером | `gophkeeper sync` |
| `version` | версия и дата сборки | `gophkeeper version` |

Команда `add` универсальна: тип задаётся флагом `--type` (`credentials`, `card`,
`text`, `binary`).

### Флаги команд `add` и `update`

| Флаг | Назначение |
| --- | --- |
| `--type` | тип секрета: `credentials`, `card`, `text`, `binary` |
| `--metadata` | произвольная метаинформация |
| `--username`, `--password` | данные для `credentials` |
| `--card-number`, `--card-holder`, `--expiry`, `--cvv` | данные для `card` |
| `--text` | данные для `text` |
| `--file` | файл с бинарными данными для `binary` |

Если данные не заданы флагами, клиент запросит их интерактивно. При `update`
заданные флаги накладываются на существующие данные, поэтому можно изменить
только одно поле (например, пароль), не затирая остальные.

### Конфигурация клиента

- Конфиг: `~/.config/gophkeeper/config.json` (Linux/macOS),
  `%APPDATA%\gophkeeper\config.json` (Windows).
- Токен: `~/.local/share/gophkeeper/token` (Linux/macOS),
  `%LOCALAPPDATA%\gophkeeper	oken` (Windows), права `600`.
- Локальная база: `~/.local/share/gophkeeper/<user_id>/secrets.db`, права `600`.

Адрес сервера задаётся полем `server_address` (по умолчанию
`http://localhost:8080`):

```json
{
  "server_address": "https://gophkeeper.example.com"
}
```

### Разрешение конфликтов

При синхронизации и обновлении используется оптимистичная блокировка по
`version`. Если запись изменена другим клиентом, сервер отвечает `409`, и
пользователю предлагается выбор:

```
Конфликт: GitHub
  Локальная версия: 3 (2026-10-04T13:31:57+03:00)
  Серверная версия: 4 (2026-10-04T13:31:58+03:00)
Выберите: [l]ocal / [r]emote / [s]kip:
```

- `[l]ocal` — перезаписать серверную версию локальной;
- `[r]emote` — принять серверную версию;
- `[s]kip` — пропустить запись.

## Безопасность

- **Zero-knowledge.** Мастер-пароль и производный от него ключ никогда не
  передаются на сервер и не сохраняются на диск. Сервер хранит только
  шифротекст.
- **KDF Argon2id** (time=3, memory=64 МиБ, threads=4, keyLen=32) защищает от
  перебора мастер-пароля. Соль (16 байт) генерируется случайно и хранится рядом
  с шифротекстом, чтобы ключ можно было восстановить на другом устройстве.
- **AES-256-GCM.** Формат шифротекста `v1:<base64-nonce>:<base64-ciphertext>`;
  nonce случаен для каждой записи.
- **JWT (HS256)** для аутентификации; секрет подписи задаётся конфигурацией.
- **Оптимистичная блокировка** по `version` и **soft delete** защищают от
  потери параллельных правок и позволяют распространять удаления между
  клиентами.
- Пароли сервера хранятся в виде bcrypt-хешей; секреты, пароли и ключи не
  логируются.

## Разработка

```bash
make build        # собрать сервер и клиент с метаданными сборки
make run-server   # запустить сервер
make test         # тесты
make cover        # покрытие и HTML-отчёт
make lint         # go vet + staticlint
make check        # build + test + lint + cover
make migrate-up   # применить миграции
make migrate-down # откатить последнюю миграцию
make certs        # самоподписанные TLS-сертификаты
```

## Тестирование

```bash
go test ./... -count=1                       # все тесты
go test ./... -count=1 -coverprofile=coverage.out
go tool cover -func=coverage.out             # покрытие по функциям
```

Покрытие по пакетам (требование — не менее 70% в каждом):

| Пакет | Покрытие |
| --- | --- |
| `internal/buildinfo` | 100.0% |
| `internal/domain` | 100.0% |
| `internal/idgen` | 100.0% |
| `internal/logger` | 100.0% |
| `internal/service` | 97.4% |
| `internal/repository` | 95.0% |
| `internal/config` | 94.1% |
| `internal/middleware` | 94.1% |
| `internal/api` | 91.7% |
| `internal/session` | 91.8% |
| `internal/crypto` | 89.5% |
| `internal/handler/http` | 87.0% |
| `internal/analysis/exitcheck` | 85.2% |
| `internal/storage` | 84.6% |
| `internal/cli` | 80.8% |
| `internal/clientconfig` | 80.2% |
