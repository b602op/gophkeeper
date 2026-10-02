-- +goose Up
-- Таблица пользователей. Пароль хранится только в виде bcrypt-хеша.
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    login         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS users;
