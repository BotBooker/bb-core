// Package jwt выпускает и проверяет токены доступа в формате JWT (HS256).
//
// Реализация — на стандартной библиотеке: подпись это HMAC-SHA256, полезная
// нагрузка это JSON, а сам формат — три блока base64url, разделённые точками.
// Собственной криптографии здесь нет, поэтому внешняя зависимость не нужна;
// поддерживается ровно один алгоритм, а поле alg в заголовке проверяется —
// иначе токен с alg=none или другим алгоритмом мог бы обойти проверку подписи.
//
// Срок действия проверяется здесь же и по переданным часам: парсер не зависит
// от time.Now, поэтому тесты детерминированы.
package jwt

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

const (
	// algorithm — единственный поддерживаемый алгоритм подписи.
	algorithm = "HS256"
	// tokenType — значение заголовка typ.
	tokenType = "JWT"
	// secretMinLen — длина генерируемого секрета в байтах (256 бит).
	secretMinLen = 32
	// tokenParts — сколько блоков в JWT: заголовок, нагрузка, подпись.
	tokenParts = 3
)

// Ошибки разбора токена.
var (
	// ErrMalformedToken — токен не похож на JWT: не тот формат или битый base64.
	ErrMalformedToken = errors.New("token is malformed")
	// ErrInvalidSignature — подпись не сходится (в том числе чужой секрет).
	ErrInvalidSignature = errors.New("token signature is invalid")
	// ErrUnsupportedAlgorithm — в заголовке не HS256.
	ErrUnsupportedAlgorithm = errors.New("token algorithm is not supported")
	// ErrInvalidClaims — нагрузка не соответствует ожиданиям (iss, sub, jti, role, время).
	ErrInvalidClaims = errors.New("token claims are invalid")
)

// header — заголовок JWT.
type header struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

// claims — полезная нагрузка JWT. В модели токена этим полям соответствуют
// TokenID (jti), UserID (sub), TenantID (tenant), Role (role), IssuedAt (iat)
// и ExpiresAt (exp).
type claims struct {
	Issuer    string `json:"iss,omitempty"`
	Subject   string `json:"sub"`
	Tenant    string `json:"tenant"`
	Role      string `json:"role"`
	TokenID   string `json:"jti"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// Signer выпускает и проверяет токены одним секретом.
type Signer struct {
	secret []byte
	issuer string
}

// NewSigner создаёт подписывальщик токенов.
//
// Пустой секрет — не ошибка: генерируется случайный (256 бит) и в лог пишется
// предупреждение. Так приложение работает в разработке без настройки, но
// токены не переживут перезапуск и не будут приниматься другими экземплярами
// приложения, поэтому в промышленной среде секрет задают явно (JWT_SECRET).
func NewSigner(secret, issuer string) (*Signer, error) {
	if secret == "" {
		generated := make([]byte, secretMinLen)
		if _, err := rand.Read(generated); err != nil {
			return nil, fmt.Errorf("generate jwt secret: %w", err)
		}

		slog.Warn("JWT_SECRET is not set: a random secret is generated, tokens will not survive a restart")

		secret = encode(generated)
	}

	return &Signer{secret: []byte(secret), issuer: issuer}, nil
}

// Sign выпускает токен по записи модели.
func (s *Signer) Sign(token *model.Token) (string, error) {
	if token == nil {
		return "", fmt.Errorf("%w: token is nil", ErrInvalidClaims)
	}

	if err := token.Validate(); err != nil {
		return "", err
	}

	rawHeader, err := json.Marshal(header{Algorithm: algorithm, Type: tokenType})
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}

	rawClaims, err := json.Marshal(claims{
		Issuer:    s.issuer,
		Subject:   token.UserID,
		Tenant:    token.TenantID,
		Role:      string(token.Role),
		TokenID:   token.ID,
		IssuedAt:  token.IssuedAt.Unix(),
		ExpiresAt: token.ExpiresAt.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	signingInput := encode(rawHeader) + "." + encode(rawClaims)

	return signingInput + "." + encode(s.sign(signingInput)), nil
}

// Parse проверяет подпись и срок действия и разбирает токен в запись модели.
//
// Проверяются: формат (три блока base64url), подпись, алгоритм из заголовка,
// издатель, обязательные поля и срок действия на момент now.
func (s *Signer) Parse(raw string, now time.Time) (*model.Token, error) {
	parts := strings.Split(raw, ".")

	if len(parts) != tokenParts {
		return nil, fmt.Errorf("%w: %d parts, want %d", ErrMalformedToken, len(parts), tokenParts)
	}

	signingInput := parts[0] + "." + parts[1]

	signature, err := decode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature: %w", ErrMalformedToken, err)
	}

	if !hmac.Equal(s.sign(signingInput), signature) {
		return nil, ErrInvalidSignature
	}

	rawHeader, err := decode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: header: %w", ErrMalformedToken, err)
	}

	var parsedHeader header
	if err := json.Unmarshal(rawHeader, &parsedHeader); err != nil {
		return nil, fmt.Errorf("%w: header: %w", ErrMalformedToken, err)
	}

	// Алгоритм проверяется после подписи: заголовок подписан, но токен,
	// подписанный другим алгоритмом, доверять нельзя.
	if parsedHeader.Algorithm != algorithm {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, parsedHeader.Algorithm)
	}

	rawClaims, err := decode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: claims: %w", ErrMalformedToken, err)
	}

	var parsedClaims claims
	if err := json.Unmarshal(rawClaims, &parsedClaims); err != nil {
		return nil, fmt.Errorf("%w: claims: %w", ErrMalformedToken, err)
	}

	if parsedClaims.Issuer != s.issuer {
		return nil, fmt.Errorf("%w: issuer %q", ErrInvalidClaims, parsedClaims.Issuer)
	}

	token := &model.Token{
		ID:        parsedClaims.TokenID,
		UserID:    parsedClaims.Subject,
		TenantID:  parsedClaims.Tenant,
		Role:      model.Role(parsedClaims.Role),
		IssuedAt:  time.Unix(parsedClaims.IssuedAt, 0).UTC(),
		ExpiresAt: time.Unix(parsedClaims.ExpiresAt, 0).UTC(),
	}

	if err := token.Validate(); err != nil {
		return nil, err
	}

	if err := token.Valid(now); err != nil {
		return nil, err
	}

	return token, nil
}

// sign считает подпись HMAC-SHA256 от строки подписи.
func (s *Signer) sign(signingInput string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))

	return mac.Sum(nil)
}

// encode кодирует блок токена: base64url без выравнивания (как требует RFC 7515).
func encode(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

// decode разбирает блок токена.
func decode(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}
