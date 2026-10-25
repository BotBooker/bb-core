package domain

// PasswordHasher — порт хеширования паролей.
//
// Домен хранит только строку хеша (model.Credentials.Hash) и проверяет его
// непустоту; вычисление и сравнение — инфраструктурная операция, её выполняет
// реализация (pkg/password). Так internal/model остаётся на stdlib, как
// требует spec §1.1, а параметры Argon2id можно менять, не трогая домен.
type PasswordHasher interface {
	// Hash считает хеш пароля и проверяет требования к паролю.
	Hash(password string) (string, error)
	// Verify сравнивает пароль с закодированным хешем.
	Verify(password, encoded string) error
	// NeedsRehash сообщает, что хеш посчитан устаревшим алгоритмом или с
	// устаревшими параметрами.
	NeedsRehash(encoded string) bool
}
