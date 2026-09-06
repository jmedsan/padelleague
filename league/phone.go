package league

import (
	"fmt"
	"strings"
	"unicode"
)

// NormalizePhone validates and normalizes a phone number to E.164 format.
// Accepts optional country prefix (+XX or 00XX); if absent and the number
// is 9 digits starting with 6/7/8/9, assumes Spain (+34).
func NormalizePhone(raw string) (string, error) {
	cleaned := stripPhoneFormatting(raw)
	if cleaned == "" {
		return "", fmt.Errorf("teléfono vacío")
	}

	if strings.HasPrefix(cleaned, "00") {
		cleaned = "+" + cleaned[2:]
	}

	hasPrefix := strings.HasPrefix(cleaned, "+")
	digits := cleaned
	if hasPrefix {
		digits = cleaned[1:]
	}

	if !allDigits(digits) {
		return "", fmt.Errorf("el teléfono solo puede contener números")
	}

	if !hasPrefix {
		if len(digits) == 9 && isSpanishMobile(digits[0]) {
			return "+34" + digits, nil
		}
		return "", fmt.Errorf("añade el prefijo del país, p. ej. +34")
	}

	if len(digits) < 9 || len(digits) > 15 {
		return "", fmt.Errorf("el teléfono debe tener entre 9 y 15 dígitos")
	}

	return "+" + digits, nil
}

// FormatPhone formats an E.164 number for display. Spanish numbers get
// grouped as +34 612 34 56 78; others just get the + prefix.
func FormatPhone(e164 string) string {
	if !strings.HasPrefix(e164, "+34") || len(e164) != 12 {
		return e164
	}
	d := e164[3:]
	return fmt.Sprintf("+34 %s %s %s %s", d[0:3], d[3:5], d[5:7], d[7:9])
}

// MaskPhone masks a phone for logging: +34 6** *** **78.
func MaskPhone(e164 string) string {
	if len(e164) < 6 {
		return "***"
	}
	return e164[:4] + strings.Repeat("*", len(e164)-6) + e164[len(e164)-2:]
}

// WhatsAppURL returns a wa.me link for the given E.164 number.
func WhatsAppURL(e164 string) string {
	digits := strings.TrimPrefix(e164, "+")
	return "https://wa.me/" + digits
}

func stripPhoneFormatting(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '+' || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func isSpanishMobile(b byte) bool {
	return b == '6' || b == '7' || b == '8' || b == '9'
}
