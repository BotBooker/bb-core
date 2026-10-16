package servers

import (
	"strings"
	"testing"
)

// TestParseBearer проверяет разбор значения заголовка Authorization.
//
// Регрессия из ревью: санитайзер здесь больше не применяется, поэтому длинный
// токен должен возвращаться без изменений, а слишком длинное значение —
// отвергаться целиком, а не обрезаться (обрезка превращала бы валидный токен в
// 401).
func TestParseBearer(t *testing.T) {
	longToken := strings.Repeat("a", 600)
	tooLongValue := bearerPrefix + strings.Repeat("a", maxAuthorizationLength)

	tests := []struct {
		name      string
		value     string
		wantToken string
		wantOK    bool
	}{
		{
			name:      "схема в нижнем регистре",
			value:     "bearer token123",
			wantToken: "token123",
			wantOK:    true,
		},
		{
			name:      "схема в верхнем регистре",
			value:     "BEARER token123",
			wantToken: "token123",
			wantOK:    true,
		},
		{
			name:      "пустое значение",
			value:     "",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "значение без схемы",
			value:     "token123",
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "схема без токена",
			value:     bearerPrefix,
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "пробелы вокруг токена сохраняются внутри",
			value:     "Bearer   token 123   ",
			wantToken: "token 123",
			wantOK:    true,
		},
		{
			name:      "токен 600 символов возвращается без изменений",
			value:     bearerPrefix + longToken,
			wantToken: longToken,
			wantOK:    true,
		},
		{
			name:      "значение длиннее лимита отвергается, а не обрезается",
			value:     tooLongValue,
			wantToken: "",
			wantOK:    false,
		},
		{
			name:      "значение 9 КиБ отвергается",
			value:     bearerPrefix + strings.Repeat("a", 9<<10),
			wantToken: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := parseBearer(tt.value)
			if ok != tt.wantOK {
				t.Errorf("parseBearer() ok = %t, ожидалось %t", ok, tt.wantOK)
			}
			if token != tt.wantToken {
				t.Errorf("parseBearer() token = %q, ожидалось %q", token, tt.wantToken)
			}
			if tt.wantOK && len(tt.value) > 0 && token == "" {
				t.Error("parseBearer() вернул пустой токен при валидном значении")
			}
		})
	}
}
