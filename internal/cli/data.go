package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/b602op/gophkeeper/internal/domain"
)

// secretData хранит флаги команд add и update.
type secretData struct {
	secretType string
	metadata   string
	username   string
	password   string
	cardNumber string
	cardHolder string
	cardExpiry string
	cardCVV    string
	text       string
	file       string
}

// register объявляет флаги команды с указанным значением типа по умолчанию.
func (d *secretData) register(cmd *cobra.Command, defaultType string) {
	cmd.Flags().StringVar(&d.secretType, "type", defaultType, "тип секрета: credentials, card, text, binary")
	cmd.Flags().StringVar(&d.metadata, "metadata", "", "произвольная метаинформация")
	cmd.Flags().StringVar(&d.username, "username", "", "логин (для credentials)")
	cmd.Flags().StringVar(&d.password, "password", "", "пароль (для credentials)")
	cmd.Flags().StringVar(&d.cardNumber, "card-number", "", "номер карты (для card)")
	cmd.Flags().StringVar(&d.cardHolder, "card-holder", "", "держатель карты (для card)")
	cmd.Flags().StringVar(&d.cardExpiry, "card-expiry", "", "срок действия карты (для card)")
	cmd.Flags().StringVar(&d.cardCVV, "card-cvv", "", "CVV карты (для card)")
	cmd.Flags().StringVar(&d.text, "text", "", "текст (для text)")
	cmd.Flags().StringVar(&d.file, "file", "", "файл с бинарными данными (для binary)")
}

// payload-структуры описывают открытые данные секрета до шифрования.
type credentialsPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type cardPayload struct {
	Number string `json:"number"`
	Holder string `json:"holder"`
	Expiry string `json:"expiry"`
	CVV    string `json:"cvv"`
}

type textPayload struct {
	Text string `json:"text"`
}

// buildPayload собирает открытые данные секрета из флагов или интерактивного
// ввода и возвращает тип записи вместе с сериализованным представлением.
func (a *app) buildPayload(d *secretData) (domain.SecretType, []byte, error) {
	secretType := domain.SecretType(strings.ToLower(strings.TrimSpace(d.secretType)))
	if !secretType.IsValid() {
		return "", nil, fmt.Errorf("%w: %q", domain.ErrInvalidSecretType, d.secretType)
	}

	switch secretType {
	case domain.SecretTypeCredentials:
		payload, err := a.buildCredentials(d)
		return secretType, payload, err
	case domain.SecretTypeCard:
		payload, err := a.buildCard(d)
		return secretType, payload, err
	case domain.SecretTypeText:
		payload, err := a.buildText(d)
		return secretType, payload, err
	case domain.SecretTypeBinary:
		payload, err := a.buildBinary(d)
		return secretType, payload, err
	default:
		return "", nil, fmt.Errorf("%w: %q", domain.ErrInvalidSecretType, d.secretType)
	}
}

func (a *app) buildCredentials(d *secretData) ([]byte, error) {
	username, password := d.username, d.password
	var err error

	if username == "" {
		if username, err = a.readLine("Логин: "); err != nil {
			return nil, err
		}
	}
	if password == "" {
		if password, err = a.readPassword("Пароль: "); err != nil {
			return nil, err
		}
	}
	if username == "" && password == "" {
		return nil, errors.New("укажите логин или пароль")
	}
	return json.Marshal(credentialsPayload{Username: username, Password: password})
}

func (a *app) buildCard(d *secretData) ([]byte, error) {
	card := cardPayload{
		Number: d.cardNumber,
		Holder: d.cardHolder,
		Expiry: d.cardExpiry,
		CVV:    d.cardCVV,
	}
	var err error

	if card.Number == "" {
		if card.Number, err = a.readLine("Номер карты: "); err != nil {
			return nil, err
		}
	}
	if card.Holder == "" {
		if card.Holder, err = a.readLine("Держатель: "); err != nil {
			return nil, err
		}
	}
	if card.Expiry == "" {
		if card.Expiry, err = a.readLine("Срок действия: "); err != nil {
			return nil, err
		}
	}
	if card.CVV == "" {
		if card.CVV, err = a.readPassword("CVV: "); err != nil {
			return nil, err
		}
	}
	if card.Number == "" {
		return nil, errors.New("номер карты не может быть пустым")
	}
	return json.Marshal(card)
}

func (a *app) buildText(d *secretData) ([]byte, error) {
	text := d.text
	if text == "" {
		var err error
		if text, err = a.readLine("Текст: "); err != nil {
			return nil, err
		}
	}
	if text == "" {
		return nil, errors.New("текст не может быть пустым")
	}
	return json.Marshal(textPayload{Text: text})
}

func (a *app) buildBinary(d *secretData) ([]byte, error) {
	path := d.file
	if path == "" {
		var err error
		if path, err = a.readLine("Путь к файлу: "); err != nil {
			return nil, err
		}
	}
	if path == "" {
		return nil, errors.New("путь к файлу не задан")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение файла %q: %w", path, err)
	}
	if len(data) == 0 {
		return nil, errors.New("файл пуст")
	}
	return data, nil
}

// formatSecret преобразует расшифрованные данные в человекочитаемый вид.
func formatSecret(secret *domain.Secret, plaintext []byte) string {
	switch secret.Type {
	case domain.SecretTypeCredentials:
		var payload credentialsPayload
		if err := json.Unmarshal(plaintext, &payload); err == nil {
			return fmt.Sprintf("Логин: %s\nПароль: %s", payload.Username, payload.Password)
		}
	case domain.SecretTypeCard:
		var payload cardPayload
		if err := json.Unmarshal(plaintext, &payload); err == nil {
			return fmt.Sprintf("Номер: %s\nДержатель: %s\nСрок: %s\nCVV: %s",
				payload.Number, payload.Holder, payload.Expiry, payload.CVV)
		}
	case domain.SecretTypeText:
		var payload textPayload
		if err := json.Unmarshal(plaintext, &payload); err == nil {
			return payload.Text
		}
	case domain.SecretTypeBinary:
		return base64.StdEncoding.EncodeToString(plaintext)
	}
	return string(plaintext)
}
