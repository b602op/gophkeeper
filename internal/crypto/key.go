// Package crypto реализует клиентское шифрование секретов GophKeeper.
//
// Ключ шифрования выводится из мастер-пароля через Argon2id и никогда не
// покидает устройство пользователя. Сервер хранит только шифротекст, поэтому
// схема является zero-knowledge: даже при полной компрометации сервера
// злоумышленник не получит открытые данные без мастер-пароля.
package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Параметры вывода ключа Argon2id.
//
// Значения подобраны как компромисс между стойкостью к перебору и временем
// вывода на обычном устройстве: 64 МиБ памяти и 3 прохода.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// DeriveKey выводит 32-байтовый ключ шифрования из мастер-пароля и соли.
//
// Одинаковые пароль и соль всегда дают одинаковый ключ, поэтому соль должна
// храниться вместе с шифротекстом: без неё расшифровка невозможна.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// GenerateSalt создаёт криптостойкую случайную соль для KDF.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("генерация соли: %w", err)
	}
	return salt, nil
}
