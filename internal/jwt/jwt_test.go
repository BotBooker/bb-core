package jwt

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

const (
	testSecret = "test-secret-with-at-least-32-bytes"
	testIssuer = "bb-core-test"
)

// testSigner возвращает подписывальщик с фиксированным секретом и издателем.
func testSigner(t *testing.T) *Signer {
	t.Helper()

	signer, err := NewSigner(testSecret, testIssuer)
	if err != nil {
		t.Fatalf("NewSigner() unexpected error = %v", err)
	}

	return signer
}

// testToken возвращает запись токена с временем, усечённым до секунд
// (JWT хранит время в Unix-секундах).
func testToken(t *testing.T) *model.Token {
	t.Helper()

	issued := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

	user, err := model.NewUser(model.CreateUserInput{
		Email:     "ivan@example.com",
		FirstName: "Иван",
		LastName:  "Иванов",
	}, issued)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.Role = model.RoleOperator

	token, err := model.NewToken(user, time.Hour, issued)
	if err != nil {
		t.Fatalf("model.NewToken() unexpected error = %v", err)
	}

	return token
}

// forge собирает токен из произвольных заголовка и нагрузки, подписывая его
// секретом signer: так проверяются ветки разбора, до которых честной подписью
// не добраться (чужой алгоритм, пустой sub).
func forge(t *testing.T, signer *Signer, rawHeader, rawClaims string) string {
	t.Helper()

	signingInput := encode([]byte(rawHeader)) + "." + encode([]byte(rawClaims))

	return signingInput + "." + encode(signer.sign(signingInput))
}

func TestSigner_SignAndParse(t *testing.T) {
	signer := testSigner(t)
	token := testToken(t)

	raw, err := signer.Sign(token)
	if err != nil {
		t.Fatalf("Sign() unexpected error = %v", err)
	}

	parts := strings.Split(raw, ".")
	if len(parts) != tokenParts {
		t.Fatalf("Sign() = %q, want %d части, разделённые точками", raw, tokenParts)
	}

	t.Run("заголовок", func(t *testing.T) {
		rawHeader, err := decode(parts[0])
		if err != nil {
			t.Fatalf("decode(header) unexpected error = %v", err)
		}

		var parsedHeader header
		if err := json.Unmarshal(rawHeader, &parsedHeader); err != nil {
			t.Fatalf("unmarshal(header) unexpected error = %v", err)
		}

		if parsedHeader.Algorithm != algorithm || parsedHeader.Type != tokenType {
			t.Errorf("header = %+v, want alg=%q typ=%q", parsedHeader, algorithm, tokenType)
		}
	})

	t.Run("нагрузка", func(t *testing.T) {
		rawClaims, err := decode(parts[1])
		if err != nil {
			t.Fatalf("decode(claims) unexpected error = %v", err)
		}

		var parsedClaims claims
		if err := json.Unmarshal(rawClaims, &parsedClaims); err != nil {
			t.Fatalf("unmarshal(claims) unexpected error = %v", err)
		}

		if parsedClaims.Subject != token.UserID {
			t.Errorf("sub = %q, want %q", parsedClaims.Subject, token.UserID)
		}
		if parsedClaims.Role != string(token.Role) {
			t.Errorf("role = %q, want %q", parsedClaims.Role, token.Role)
		}
		if parsedClaims.TokenID != token.ID {
			t.Errorf("jti = %q, want %q", parsedClaims.TokenID, token.ID)
		}
		if parsedClaims.Issuer != testIssuer {
			t.Errorf("iss = %q, want %q", parsedClaims.Issuer, testIssuer)
		}
		if parsedClaims.IssuedAt != token.IssuedAt.Unix() {
			t.Errorf("iat = %d, want %d", parsedClaims.IssuedAt, token.IssuedAt.Unix())
		}
		if parsedClaims.ExpiresAt != token.ExpiresAt.Unix() {
			t.Errorf("exp = %d, want %d", parsedClaims.ExpiresAt, token.ExpiresAt.Unix())
		}
	})

	t.Run("разбор возвращает ту же запись", func(t *testing.T) {
		parsed, err := signer.Parse(raw, token.IssuedAt.Add(time.Minute))
		if err != nil {
			t.Fatalf("Parse() unexpected error = %v", err)
		}

		if parsed.ID != token.ID || parsed.UserID != token.UserID || parsed.Role != token.Role {
			t.Errorf("Parse() = %+v, want id=%q user=%q role=%q", parsed, token.ID, token.UserID, token.Role)
		}
		if !parsed.IssuedAt.Equal(token.IssuedAt) || !parsed.ExpiresAt.Equal(token.ExpiresAt) {
			t.Errorf("Parse() время = %v..%v, want %v..%v",
				parsed.IssuedAt, parsed.ExpiresAt, token.IssuedAt, token.ExpiresAt)
		}
		// Отзыв хранится в БД, а не в токене: разбор его не выдумывает.
		if parsed.Revoked() {
			t.Error("Parse() вернул отозванный токен, хотя отзыв в JWT не передаётся")
		}
	})
}

func TestSigner_Sign_Errors(t *testing.T) {
	signer := testSigner(t)

	t.Run("нет токена", func(t *testing.T) {
		if _, err := signer.Sign(nil); !errors.Is(err, ErrInvalidClaims) {
			t.Errorf("Sign() error = %v, want %v", err, ErrInvalidClaims)
		}
	})

	t.Run("невалидная запись", func(t *testing.T) {
		token := testToken(t)
		token.Role = model.Role("wat")

		if _, err := signer.Sign(token); !errors.Is(err, model.ErrInvalidRole) {
			t.Errorf("Sign() error = %v, want %v", err, model.ErrInvalidRole)
		}
	})
}

func TestSigner_Parse_Errors(t *testing.T) {
	signer := testSigner(t)
	token := testToken(t)

	valid, err := signer.Sign(token)
	if err != nil {
		t.Fatalf("Sign() unexpected error = %v", err)
	}

	parts := strings.Split(valid, ".")

	foreign, err := NewSigner("another-secret-with-at-least-32-b", testIssuer)
	if err != nil {
		t.Fatalf("NewSigner() unexpected error = %v", err)
	}

	otherIssuer, err := NewSigner(testSecret, "another-issuer")
	if err != nil {
		t.Fatalf("NewSigner() unexpected error = %v", err)
	}

	tampered := parts[0] + "." + encode([]byte(`{"sub":"x"}`)) + "." + parts[2]

	tests := []struct {
		name   string
		raw    string
		now    time.Time
		signer *Signer
		want   error
	}{
		{
			name:   "пустая строка",
			raw:    "",
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "две части",
			raw:    parts[0] + "." + parts[1],
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "четыре части",
			raw:    valid + ".extra",
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "подпись не base64",
			raw:    parts[0] + "." + parts[1] + ".***",
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "заголовок не base64",
			raw:    "***." + parts[1] + "." + encode(signer.sign("***."+parts[1])),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "нагрузка не base64",
			raw:    parts[0] + ".***." + encode(signer.sign(parts[0]+".***")),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "заголовок не JSON",
			raw:    forge(t, signer, "{not json}", `{"iss":"bb-core-test"}`),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "нагрузка не JSON",
			raw:    forge(t, signer, `{"alg":"HS256","typ":"JWT"}`, "{not json}"),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrMalformedToken,
		},
		{
			name:   "подпись чужим секретом",
			raw:    valid,
			now:    token.IssuedAt,
			signer: foreign,
			want:   ErrInvalidSignature,
		},
		{
			name:   "нагрузка подменена",
			raw:    tampered,
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrInvalidSignature,
		},
		{
			name:   "другой издатель",
			raw:    valid,
			now:    token.IssuedAt,
			signer: otherIssuer,
			want:   ErrInvalidClaims,
		},
		{
			name:   "просрочен",
			raw:    valid,
			now:    token.ExpiresAt,
			signer: signer,
			want:   model.ErrTokenExpired,
		},
		{
			name: "чужой алгоритм",
			raw: forge(t, signer,
				`{"alg":"HS512","typ":"JWT"}`,
				`{"iss":"bb-core-test","sub":"`+token.UserID+`","role":"operator",`+
					`"jti":"`+token.ID+`","iat":1,"exp":2}`,
			),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrUnsupportedAlgorithm,
		},
		{
			name: "alg=none",
			raw: forge(t, signer,
				`{"alg":"none","typ":"JWT"}`,
				`{"iss":"bb-core-test","sub":"`+token.UserID+`","role":"operator",`+
					`"jti":"`+token.ID+`","iat":1,"exp":2}`,
			),
			now:    token.IssuedAt,
			signer: signer,
			want:   ErrUnsupportedAlgorithm,
		},
		{
			name: "нет владельца",
			raw: forge(t, signer,
				`{"alg":"HS256","typ":"JWT"}`,
				`{"iss":"bb-core-test","sub":"","role":"operator","jti":"`+token.ID+`",`+
					`"iat":1,"exp":2}`,
			),
			now:    token.IssuedAt,
			signer: signer,
			want:   model.ErrInvalidToken,
		},
		{
			name: "неизвестная роль",
			raw: forge(t, signer,
				`{"alg":"HS256","typ":"JWT"}`,
				`{"iss":"bb-core-test","sub":"`+token.UserID+`","role":"wat","jti":"`+token.ID+`",`+
					`"iat":1,"exp":2}`,
			),
			now:    token.IssuedAt,
			signer: signer,
			want:   model.ErrInvalidRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.signer.Parse(tt.raw, tt.now)
			if !errors.Is(err, tt.want) {
				t.Errorf("Parse() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewSigner_EmptySecret(t *testing.T) {
	first, err := NewSigner("", testIssuer)
	if err != nil {
		t.Fatalf("NewSigner() unexpected error = %v", err)
	}

	second, err := NewSigner("", testIssuer)
	if err != nil {
		t.Fatalf("NewSigner() unexpected error = %v", err)
	}

	token := testToken(t)

	raw, err := first.Sign(token)
	if err != nil {
		t.Fatalf("Sign() unexpected error = %v", err)
	}

	if _, err := first.Parse(raw, token.IssuedAt); err != nil {
		t.Errorf("Parse() своим секретом unexpected error = %v", err)
	}

	// Секрет случайный для каждого запуска, поэтому чужой экземпляр токен не примет.
	if _, err := second.Parse(raw, token.IssuedAt); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Parse() чужим секретом error = %v, want %v", err, ErrInvalidSignature)
	}
}
