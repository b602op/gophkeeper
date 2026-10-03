// Package domain содержит бизнес-модели GophKeeper и доменные ошибки.
//
// Пакет не зависит от других пакетов проекта и не знает о транспортах или
// хранилищах. Все остальные слои могут зависеть от domain, но не наоборот.
package domain

import "errors"

// Доменные ошибки. Сравнивать их следует через errors.Is, чтобы обёрнутые
// ошибки сохраняли первопричину.
var (
	// ErrUserNotFound возвращается, когда пользователь отсутствует в хранилище.
	ErrUserNotFound = errors.New("пользователь не найден")
	// ErrUserAlreadyExists возвращается при попытке занять занятый логин.
	ErrUserAlreadyExists = errors.New("пользователь уже существует")
	// ErrInvalidCredentials возвращается при неверном логине или пароле.
	ErrInvalidCredentials = errors.New("неверный логин или пароль")
	// ErrInvalidToken возвращается, если токен отсутствует, истёк или невалиден.
	ErrInvalidToken = errors.New("недействительный токен")
	// ErrSecretNotFound возвращается, когда секрет не найден или недоступен.
	ErrSecretNotFound = errors.New("секрет не найден")
	// ErrSecretAlreadyExists возвращается при попытке создать существующий секрет.
	ErrSecretAlreadyExists = errors.New("секрет уже существует")
	// ErrSecretVersionMismatch возвращается при несовпадении версии секрета.
	ErrSecretVersionMismatch = errors.New("конфликт версий секрета")
	// ErrInvalidSecretType возвращается при неизвестном типе секрета.
	ErrInvalidSecretType = errors.New("неизвестный тип секрета")
	// ErrInvalidSecretData возвращается при некорректных данных секрета.
	ErrInvalidSecretData = errors.New("некорректные данные секрета")
	// ErrForbidden возвращается при попытке доступа к чужому ресурсу.
	ErrForbidden = errors.New("доступ запрещён")
	// ErrValidation возвращается, когда входные данные не прошли проверку.
	ErrValidation = errors.New("ошибка валидации")
)
