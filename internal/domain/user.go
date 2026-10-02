package domain

import "time"

// User описывает зарегистрированного пользователя системы.
//
// PasswordHash содержит bcrypt-хеш пароля; открытый пароль нигде не хранится.
type User struct {
	ID           string
	Login        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
