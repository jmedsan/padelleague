package league

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePhone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"spanish mobile no prefix", "612345678", "+34612345678", false},
		{"spanish mobile with +34", "+34612345678", "+34612345678", false},
		{"spanish mobile with 0034", "0034612345678", "+34612345678", false},
		{"spanish with spaces", "612 345 678", "+34612345678", false},
		{"spanish with dots", "612.345.678", "+34612345678", false},
		{"spanish with dashes", "612-345-678", "+34612345678", false},
		{"spanish with parens", "(612) 345 678", "+34612345678", false},
		{"spanish 7xx", "712345678", "+34712345678", false},
		{"spanish 8xx", "812345678", "+34812345678", false},
		{"spanish 9xx", "912345678", "+34912345678", false},
		{"international +44", "+447911123456", "+447911123456", false},
		{"international +1", "+12025551234", "+12025551234", false},
		{"international 0044", "00447911123456", "+447911123456", false},
		{"too short", "+341234", "", true},
		{"too long", "+3461234567890123456", "", true},
		{"letters", "+34abc612345", "", true},
		{"empty", "", "", true},
		{"no prefix non-spanish", "123456789", "", true},
		{"spanish landline no prefix (5xx)", "512345678", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePhone(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFormatPhone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "+34 612 34 56 78", FormatPhone("+34612345678"))
	assert.Equal(t, "+447911123456", FormatPhone("+447911123456"))
}

func TestMaskPhone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "+346******78", MaskPhone("+34612345678"))
	assert.Equal(t, "***", MaskPhone("+34"))
}

func TestWhatsAppURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://wa.me/34612345678", WhatsAppURL("+34612345678"))
}
