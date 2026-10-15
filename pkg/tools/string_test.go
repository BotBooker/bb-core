package tools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// truncationSuffix добавляется к значению, если оно длиннее 500 байт.
const truncationSuffix = "... [truncated]"

func TestSanitizeInputString(t *testing.T) {
	tests := []struct {
		name  string
		value string
		keys  []string
		want  string
	}{
		// Значение без ключа или с несекретным ключом отдаётся как есть.
		{
			name:  "empty value without key",
			value: "",
			want:  "",
		},
		{
			name:  "plain value without key",
			value: "hello world",
			want:  "hello world",
		},
		{
			name:  "empty key is ignored",
			value: "hello world",
			keys:  []string{""},
			want:  "hello world",
		},
		{
			name:  "non sensitive key",
			value: "john",
			keys:  []string{"username"},
			want:  "john",
		},
		{
			name:  "at sign is kept for non email key",
			value: "user@host",
			keys:  []string{"login"},
			want:  "user@host",
		},
		{
			name:  "only first key is used",
			value: "john",
			keys:  []string{"username", "password"},
			want:  "john",
		},

		// Маскирование секретов по имени ключа (регистронезависимо).
		{
			name:  "password key",
			value: "s3cr3t",
			keys:  []string{"password"},
			want:  "***",
		},
		{
			name:  "password inside composite key",
			value: "s3cr3t",
			keys:  []string{"user_password_hash"},
			want:  "***",
		},
		{
			name:  "uppercase key",
			value: "s3cr3t",
			keys:  []string{"PASSWORD"},
			want:  "***",
		},
		{
			name:  "mixed case secret key",
			value: "s3cr3t",
			keys:  []string{"Client_Secret"},
			want:  "***",
		},
		{
			name:  "token key",
			value: "eyJhbGciOi",
			keys:  []string{"api_token"},
			want:  "***",
		},
		{
			name:  "cvv key",
			value: "123",
			keys:  []string{"card_cvv"},
			want:  "***",
		},
		{
			name:  "auth key",
			value: "Bearer some-token",
			keys:  []string{"authorization"},
			want:  "***",
		},
		{
			name:  "secret check wins over sanitization",
			value: "ok\nERROR fake log record",
			keys:  []string{"client_secret"},
			want:  "***",
		},
		{
			name:  "secret check wins over email masking",
			value: "john@example.com",
			keys:  []string{"email_token"},
			want:  "***",
		},

		// Маскирование email: остаётся первый символ локальной части и весь домен.
		{
			name:  "email is masked",
			value: "john.doe@example.com",
			keys:  []string{"email"},
			want:  "j***@example.com",
		},
		{
			name:  "uppercase email key",
			value: "john@example.com",
			keys:  []string{"Email"},
			want:  "j***@example.com",
		},
		{
			name:  "email key inside composite key",
			value: "user@host.io",
			keys:  []string{"user_email"},
			want:  "u***@host.io",
		},
		{
			name:  "email key without at sign falls through",
			value: "not-an-email",
			keys:  []string{"email"},
			want:  "not-an-email",
		},
		{
			name:  "email with empty local part is fully masked",
			value: "@example.com",
			keys:  []string{"email"},
			want:  "***",
		},
		{
			name:  "email with multiple at signs is fully masked",
			value: "a@b@c.com",
			keys:  []string{"email"},
			want:  "***",
		},

		// Защита от log injection: переносы строк заменяются на пробелы.
		{
			name:  "newline is replaced",
			value: "line1\nline2",
			want:  "line1 line2",
		},
		{
			name:  "carriage return is replaced",
			value: "line1\rline2",
			want:  "line1 line2",
		},
		{
			name:  "crlf becomes two spaces",
			value: "line1\r\nline2",
			want:  "line1  line2",
		},
		{
			name:  "fake log record is flattened",
			value: "value\nINFO 2026-10-12 fake record",
			want:  "value INFO 2026-10-12 fake record",
		},
		{
			name:  "only newlines become spaces",
			value: "\n\n\n",
			want:  "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInputString(tt.value, tt.keys...)
			if got != tt.want {
				t.Errorf("SanitizeInputString(%q, %v) = %q, want %q", tt.value, tt.keys, got, tt.want)
			}
		})
	}
}

func TestSanitizeInputStringTruncation(t *testing.T) {
	ascii499 := strings.Repeat("a", 499)
	ascii500 := strings.Repeat("a", 500)

	tests := []struct {
		name  string
		value string
		keys  []string
		want  string
	}{
		{
			name:  "value below limit is untouched",
			value: ascii499,
			want:  ascii499,
		},
		{
			name:  "value at limit is untouched",
			value: ascii500,
			want:  ascii500,
		},
		{
			name:  "value above limit is truncated",
			value: ascii500 + "b",
			want:  ascii500 + truncationSuffix,
		},
		{
			name:  "value far above limit keeps only first 500 bytes",
			value: strings.Repeat("x", 1000),
			want:  strings.Repeat("x", 500) + truncationSuffix,
		},
		{
			name:  "sanitizing happens before truncation",
			value: "\n" + strings.Repeat("b", 600),
			want:  " " + strings.Repeat("b", 499) + truncationSuffix,
		},
		{
			name:  "secret key wins over truncation",
			value: strings.Repeat("a", 1000),
			keys:  []string{"token"},
			want:  "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInputString(tt.value, tt.keys...)
			if got != tt.want {
				t.Errorf("SanitizeInputString(len=%d, %v) = %q, want %q", len(tt.value), tt.keys, got, tt.want)
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("SanitizeInputString(len=%d) = %q contains a newline", len(tt.value), got)
			}
		})
	}
}

func TestSanitizeInputStringTruncationKeepsValidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			// 300 * 2 = 600 байт, 500 делится на 2 нацело — границы руны не рвутся.
			name:  "2-byte runes cut on rune boundary",
			value: strings.Repeat("я", 300),
			want:  strings.Repeat("я", 250) + truncationSuffix,
		},
		{
			// 200 * 3 = 600 байт, 500 не делится на 3 — граница сдвигается на 2 байта назад.
			name:  "3-byte runes rolled back to rune boundary",
			value: strings.Repeat("€", 200),
			want:  strings.Repeat("€", 166) + truncationSuffix,
		},
		{
			// 150 * 4 = 600 байт, 500 делится на 4 нацело.
			name:  "4-byte runes cut on rune boundary",
			value: strings.Repeat("😀", 150),
			want:  strings.Repeat("😀", 125) + truncationSuffix,
		},
		{
			// 1 + 200 * 3 = 601 байт, граница сдвигается на 1 байт назад (до 499).
			name:  "mixed ascii and multibyte rolled back",
			value: "a" + strings.Repeat("€", 200),
			want:  "a" + strings.Repeat("€", 166) + truncationSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInputString(tt.value)

			if got != tt.want {
				t.Errorf("SanitizeInputString(len=%d bytes) = %q, want %q", len(tt.value), got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("SanitizeInputString(len=%d bytes) returned invalid UTF-8: %q", len(tt.value), got)
			}
			if !strings.HasSuffix(got, truncationSuffix) {
				t.Errorf("SanitizeInputString(len=%d bytes) = %q, want suffix %q", len(tt.value), got, truncationSuffix)
			}

			body := strings.TrimSuffix(got, truncationSuffix)
			if len(body) > 500 {
				t.Errorf("truncated body is %d bytes, want <= 500", len(body))
			}
		})
	}
}
