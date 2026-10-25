// Package password реализует хеширование и проверку паролей.
//
// Пароли хеширует Argon2id — современный алгоритм, победитель Password Hashing
// Competition. Он намеренно медленный и «жадный» до памяти: подбор пароля
// требует заметной памяти на каждую попытку, поэтому перебор на GPU/ASIC
// становится невыгодным. Устаревшие алгоритмы (MD5, SHA1/SHA256 без растяжения
// ключа) не используются вовсе.
//
// Алгоритм и его параметры хранятся в самой строке хеша, поэтому их изменение
// не ломает уже сохранённые пароли: старые хеши продолжают проверяться, а при
// следующем успешном входе пересчитываются новыми параметрами (см. Hasher.NeedsRehash).
//
// Пакет — инфраструктурная реализация порта domain.PasswordHasher: домен
// (internal/model) хранит только строку хеша и на stdlib проверяет его
// непустоту, а вычисление живёт здесь (spec §1.1).
package password

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
)

const (
	// passwordSaltLen — длина соли в байтах: своя для каждого пароля, поэтому
	// одинаковые пароли дают разные хеши.
	passwordSaltLen = 16
	// passwordKeyLen — длина ключа (хеша) в байтах.
	passwordKeyLen = 32
	// hashAlgo — имя алгоритма хеширования пароля в закодированном хеше.
	hashAlgo = "argon2id"
	// argon2Version — версия Argon2. Хранится в хеше вместе с параметрами,
	// поэтому обновление библиотеки не ломает уже сохранённые пароли.
	argon2Version = 19
	// argon2Memory — сколько KiB памяти использует один расчёт (19 MiB —
	// рекомендация OWASP для Argon2id).
	argon2Memory = 19 * 1024
	// argon2Time — число проходов (iterations).
	argon2Time = 2
	// argon2Threads — число потоков, которыми считается хеш.
	argon2Threads = 1
	// legacyHashAlgo — алгоритм прежних версий сервиса (PBKDF2-HMAC-SHA256).
	// Поддерживается только для проверки уже сохранённых хешей: новые пароли
	// всегда считаются Argon2id.
	legacyHashAlgo = "pbkdf2_sha256"
	// maxPasswordLen — технический предел входного пароля: защита от того,
	// чтобы один запрос не заставил хешировать мегабайты. Политика длины
	// пароля — правило домена (model.ValidatePassword).
	maxPasswordLen = 1024
)

// Ошибки хеширования и проверки пароля.
var (
	// ErrMismatch — пароль не совпадает с сохранённым хешем.
	ErrMismatch = errors.New("password does not match")
	// ErrInvalidHash — хранимый хеш повреждён или посчитан неизвестным
	// алгоритмом: проверить по нему пароль нельзя.
	ErrInvalidHash = errors.New("stored password hash is invalid")
	// ErrEmptyPassword — хешировать нечего: пустой пароль.
	ErrEmptyPassword = errors.New("password is empty")
	// ErrPasswordTooLong — пароль длиннее технического предела.
	ErrPasswordTooLong = errors.New("password is too long")
)

// Hasher — реализация domain.PasswordHasher на Argon2id.
//
// Параметры алгоритма заданы константами пакета: менять их можно без миграции
// данных, а уже сохранённые хеши пересчитываются при следующем входе
// (NeedsRehash).
type Hasher struct{}

// NewHasher создаёт хешер с параметрами OWASP по умолчанию.
func NewHasher() *Hasher { return &Hasher{} }

// Hash считает хеш пароля Argon2id в формате
//
//	argon2id$v=19$m=19456,t=2,p=1$<соль>$<ключ>
//
// Соль случайная для каждого пароля, поэтому одинаковые пароли дают разные
// хеши. Требования к длине пароля проверяет домен (model.ValidatePassword) —
// здесь остаётся только техническая защита от слишком длинного входа.
func (h *Hasher) Hash(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}

	if len(password) > maxPasswordLen {
		return "", fmt.Errorf("%w: %d bytes is more than %d", ErrPasswordTooLong, len(password), maxPasswordLen)
	}

	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	key := argon2IDKey(password, salt, argon2Time, argon2Memory, argon2Threads)

	return encodeArgon2Hash(salt, key), nil
}

// Verify сравнивает пароль с закодированным хешем и возвращает nil, если
// пароль подходит.
//
// Понимаются оба формата: текущий Argon2id и прежний PBKDF2-HMAC-SHA256
// (legacyHashAlgo) — иначе после перехода на новый алгоритм уже сохранённые
// пароли перестали бы проверяться. Возвращает ErrMismatch, если пароль не
// подходит, и ErrInvalidHash, если хеш нельзя разобрать. Сравнение ключей — за
// постоянное время (crypto/subtle): по времени ответа нельзя узнать, сколько
// байт хеша совпало.
func (h *Hasher) Verify(password, encoded string) error {
	switch {
	case strings.HasPrefix(encoded, hashAlgo+"$"):
		return verifyArgon2(password, encoded)
	case strings.HasPrefix(encoded, legacyHashAlgo+"$"):
		return verifyPBKDF2(password, encoded)
	default:
		return fmt.Errorf("%w: unknown algorithm in %q", ErrInvalidHash, prefix(encoded))
	}
}

// NeedsRehash сообщает, что хеш посчитан устаревшим алгоритмом или с
// параметрами, отличными от текущих, и его стоит пересчитать.
//
// Вызывается после успешной проверки пароля: открытый пароль в этот момент
// известен, поэтому хеш можно обновить, не заставляя пользователя менять
// пароль (см. AuthService — «прозрачное» обновление хеша при входе).
func (h *Hasher) NeedsRehash(encoded string) bool {
	if !strings.HasPrefix(encoded, hashAlgo+"$") {
		return true
	}

	params, _, key, err := decodeArgon2Hash(encoded)
	if err != nil {
		return true
	}

	return params != currentArgon2Params() || uint32(len(key)) != passwordKeyLen
}
