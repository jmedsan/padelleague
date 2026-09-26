package league

import (
	"fmt"
	"net/mail"
	"strings"
)

// NormalizeEmail validates and lowercases an email address.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("email vacío")
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", fmt.Errorf("email no válido")
	}
	return strings.ToLower(addr.Address), nil
}
