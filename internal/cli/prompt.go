package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// readLine печатает приглашение и читает строку до перевода строки.
func (a *app) readLine(prompt string) (string, error) {
	a.print("%s", prompt)

	line, err := a.bufReader().ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("чтение ввода: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// readPassword читает пароль без эха при работе с терминалом.
//
// Если ввод перенаправлен (файл или канал), пароль читается обычной строкой:
// term.ReadPassword требует настоящего терминала.
func (a *app) readPassword(prompt string) (string, error) {
	a.print("%s", prompt)

	if file, ok := a.in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		raw, err := term.ReadPassword(int(file.Fd()))
		a.print("\n")
		if err != nil {
			return "", fmt.Errorf("чтение пароля: %w", err)
		}
		return string(raw), nil
	}

	line, err := a.bufReader().ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("чтение пароля: %w", err)
	}
	return strings.TrimSpace(line), nil
}
