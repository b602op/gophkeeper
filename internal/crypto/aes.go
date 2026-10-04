package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Параметры формата шифротекста.
const (
	// cipherVersion — текущая версия формата. Префикс позволяет менять схему
	// шифрования без потери совместимости со старыми записями.
	cipherVersion = "v1"
	// nonceLen — длина случайного nonce для AES-GCM.
	nonceLen = 12
	// separator разделяет части формата v1:<nonce>:<ciphertext>.
	separator = ":"
)

// Ошибки пакета. Проверять их следует через errors.Is.
var (
	// ErrInvalidFormat возвращается, если строка шифротекста не соответствует
	// ожидаемому формату.
	ErrInvalidFormat = errors.New("невалидный формат шифротекста")
	// ErrDecrypt возвращается, если расшифровка не удалась: повреждённые данные
	// или неверный ключ.
	ErrDecrypt = errors.New("не удалось расшифровать данные")
)

// Encrypt шифрует plaintext ключом AES-256-GCM.
//
// Возвращает строку формата v1:<base64-nonce>:<base64-ciphertext>. Nonce
// генерируется случайно при каждом вызове, поэтому одинаковые открытые данные
// дают разный шифротекст.
func Encrypt(key, plaintext []byte) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("генерация nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return strings.Join([]string{
		cipherVersion,
		base64.StdEncoding.EncodeToString(nonce),
		base64.StdEncoding.EncodeToString(ciphertext),
	}, separator), nil
}

// Decrypt расшифровывает строку формата v1:<base64-nonce>:<base64-ciphertext>.
//
// Неверный ключ, повреждённый шифротекст и неизвестная версия формата дают
// ошибку, оборачивающую ErrInvalidFormat или ErrDecrypt.
func Decrypt(key []byte, encoded string) ([]byte, error) {
	nonce, ciphertext, err := parseEncoded(encoded)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecrypt, err)
	}
	return plaintext, nil
}

// newGCM создаёт AES-GCM с указанным ключом.
func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("создание AES-шифра: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("создание GCM: %w", err)
	}
	return gcm, nil
}

// parseEncoded разбирает строку формата и возвращает nonce и шифротекст.
func parseEncoded(encoded string) (nonce, ciphertext []byte, err error) {
	parts := strings.SplitN(encoded, separator, 3)
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("%w: ожидается %s:<nonce>:<ciphertext>", ErrInvalidFormat, cipherVersion)
	}
	if parts[0] != cipherVersion {
		return nil, nil, fmt.Errorf("%w: неизвестная версия %q", ErrInvalidFormat, parts[0])
	}

	nonce, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: декодирование nonce: %v", ErrInvalidFormat, err)
	}
	if len(nonce) != nonceLen {
		return nil, nil, fmt.Errorf("%w: длина nonce %d байт вместо %d", ErrInvalidFormat, len(nonce), nonceLen)
	}

	ciphertext, err = base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: декодирование шифротекста: %v", ErrInvalidFormat, err)
	}
	if len(ciphertext) == 0 {
		return nil, nil, fmt.Errorf("%w: пустой шифротекст", ErrInvalidFormat)
	}
	return nonce, ciphertext, nil
}
