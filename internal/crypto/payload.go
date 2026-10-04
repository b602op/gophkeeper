package crypto

import "fmt"

// PayloadSaltLen — длина соли KDF, которая предваряет шифротекст в поле Data
// секрета.
//
// Соль хранится вместе с шифротекстом (а не только локально), чтобы тот же
// мастер-ключ можно было вывести на другом устройстве при синхронизации.
const PayloadSaltLen = saltLen

// PackPayload объединяет соль и строку шифротекста в единый блок данных.
//
// Формат: salt(16 байт) || "v1:<base64-nonce>:<base64-ciphertext>".
func PackPayload(salt []byte, encoded string) ([]byte, error) {
	if len(salt) != saltLen {
		return nil, fmt.Errorf("%w: длина соли %d байт вместо %d", ErrInvalidFormat, len(salt), saltLen)
	}
	payload := make([]byte, 0, saltLen+len(encoded))
	payload = append(payload, salt...)
	payload = append(payload, encoded...)
	return payload, nil
}

// UnpackPayload разделяет блок данных на соль KDF и строку шифротекста.
func UnpackPayload(payload []byte) (salt []byte, encoded string, err error) {
	if len(payload) <= saltLen {
		return nil, "", fmt.Errorf("%w: данные короче соли", ErrInvalidFormat)
	}
	salt = make([]byte, saltLen)
	copy(salt, payload[:saltLen])
	return salt, string(payload[saltLen:]), nil
}
