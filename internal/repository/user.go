package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// UserDB — интерфейс базы данных, который нужен UserRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы, а менеджер транзакций и закрытие пула — нет.
type UserDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// UserRepo — интерфейс репозитория пользователей.
type UserRepo interface {
	// Create вставляет пользователя; конфликт уникальности — database.ErrDuplicate.
	Create(ctx context.Context, user *model.User) error
	// GetByID возвращает активного пользователя по идентификатору
	// или database.ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.User, error)
	// GetByEmail возвращает активного пользователя по email
	// или database.ErrNotFound.
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	// Update перезаписывает изменяемые поля активного пользователя;
	// отсутствие активной строки — database.ErrNotFound.
	Update(ctx context.Context, user *model.User) error
	// Delete помечает пользователя удалённым (soft delete: is_active = false);
	// отсутствие активной строки — database.ErrNotFound.
	Delete(ctx context.Context, id string) error
}

// userColumns — колонки users в том порядке, в котором их читает scanUser.
// Порядок должен совпадать: рассинхрон ловят тесты репозитория.
const userColumns = "id, email, first_name, last_name, middle_name, phone_number," +
	" gender, birth_date, role, created_at, updated_at, last_login_at, is_active"

// userRepo — конкретная реализация, скрыта от внешних пакетов.
type userRepo struct {
	db UserDB
	codec
}

// NewUserRepo создаёт репозиторий пользователей.
//
// Диалект нужен для двух вещей: подстановки плейсхолдеров ($1 в PostgreSQL,
// ? в SQLite) и кодирования времени (TIMESTAMPTZ против строки RFC3339).
func NewUserRepo(db UserDB, dialect string) UserRepo {
	slog.Debug("user repository created", "dialect", dialect)

	return &userRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет активного пользователя.
func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	const query = `INSERT INTO users` +
		` (id, email, first_name, last_name, middle_name, phone_number,` +
		` gender, birth_date, role, created_at, updated_at, last_login_at, is_active)` +
		` VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, r.rebind(query),
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.MiddleName,
		user.PhoneNumber,
		string(user.Gender),
		r.dateArg(user.BirthDate),
		string(user.Role),
		r.timeArg(user.CreatedAt),
		r.timeArg(user.UpdatedAt),
		r.nullTimeArg(user.LastLoginAt),
		user.IsActive,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", database.MapError(err))
	}

	return nil
}

// GetByID возвращает активного пользователя по идентификатору (UUID).
// Удалённые (is_active = false) записи не видны: soft delete делает их
// недоступными так же, как обычное удаление.
func (r *userRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	const query = "SELECT " + userColumns + " FROM users WHERE id = ? AND is_active = true"

	return r.getBy(ctx, query, id)
}

// GetByEmail возвращает активного пользователя по email: вход и восстановление
// доступа.
//
// Условие is_active = true обязательно: после soft delete в таблице может
// остаться несколько строк с одним email (одна активная и любое число удалённых),
// и без фильтра QueryRow вернул бы произвольную из них.
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	const query = "SELECT " + userColumns +
		" FROM users WHERE email = ? AND is_active = true"

	return r.getBy(ctx, query, email)
}

// Update перезаписывает изменяемые поля активного пользователя.
// Отсутствие активной строки — database.ErrNotFound, а не тихий успех:
// удалённого пользователя менять нельзя.
func (r *userRepo) Update(ctx context.Context, user *model.User) error {
	const query = `UPDATE users SET email = ?, first_name = ?, last_name = ?, middle_name = ?,` +
		` phone_number = ?, gender = ?, birth_date = ?, role = ?, updated_at = ?, last_login_at = ?` +
		` WHERE id = ? AND is_active = true`

	result, err := r.db.ExecContext(ctx, r.rebind(query),
		user.Email,
		user.FirstName,
		user.LastName,
		user.MiddleName,
		user.PhoneNumber,
		string(user.Gender),
		r.dateArg(user.BirthDate),
		string(user.Role),
		r.timeArg(user.UpdatedAt),
		r.nullTimeArg(user.LastLoginAt),
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", database.MapError(err))
	}

	return requireAffected(result, "update user")
}

// Delete помечает пользователя удалённым: строка остаётся, is_active = false.
//
// Это soft delete, а не DELETE: так сохраняется история (токены, учётные данные)
// и освобождается email — частичный уникальный индекс users_email_idx держит
// уникальность только среди активных строк. Условие is_active = true делает
// повторное удаление ошибкой ErrNotFound, а не тихим успехом.
func (r *userRepo) Delete(ctx context.Context, id string) error {
	const query = "UPDATE users SET is_active = false WHERE id = ? AND is_active = true"

	// Значение приходит от вызывающего слоя, поэтому проверяется до запроса:
	// невалидный id отклоняется здесь, а не отдаётся драйверу как аргумент.
	if err := model.ValidateUUID(id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Запрос — константа с плейсхолдером, id передаётся параметром. Bearer в
	// форме ExecContext с одним аргументом принимает этот аргумент за тело
	// запроса, поэтому срабатывание правила go_gosec_sql_concat_sqli ложное.
	// bearer:disable go_gosec_sql_concat_sqli
	result, err := r.db.ExecContext(ctx, r.rebind(query), id)
	if err != nil {
		return fmt.Errorf("delete user: %w", database.MapError(err))
	}

	return requireAffected(result, "delete user")
}

// getBy выполняет SELECT одной строки и читает её в модель.
func (r *userRepo) getBy(ctx context.Context, query string, args ...any) (*model.User, error) {
	user, err := scanUser(r.db.QueryRowContext(ctx, r.rebind(query), args...))
	if err != nil {
		return nil, fmt.Errorf("select user: %w", database.MapError(err))
	}

	return user, nil
}

// scanUser читает строку users в доменную модель.
//
// Время и дату разбирают обёртки nullTime и nullDate: PostgreSQL отдаёт
// time.Time, SQLite — строку в формате, который задал репозиторий.
func scanUser(row *sql.Row) (*model.User, error) {
	var (
		user                 model.User
		gender, role         string
		birthDate            nullDate
		createdAt, updatedAt nullTime
		lastLogin            nullTime
	)

	// is_active читается в bool: PostgreSQL отдаёт BOOLEAN, SQLite — INTEGER
	// (0/1), и database/sql приводит оба вида через driver.Bool.
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.MiddleName,
		&user.PhoneNumber,
		&gender,
		&birthDate,
		&role,
		&createdAt,
		&updatedAt,
		&lastLogin,
		&user.IsActive,
	)
	if err != nil {
		return nil, err
	}

	user.Gender = model.Gender(gender)
	user.BirthDate = birthDate.date
	user.Role = model.Role(role)
	user.CreatedAt = createdAt.time()
	user.UpdatedAt = updatedAt.time()
	user.LastLoginAt = lastLogin.ptr()

	return &user, nil
}

// nullDate принимает дату в видах, которые отдают диалекты: time.Time из
// PostgreSQL (DATE) и строку YYYY-MM-DD из SQLite. В модели это model.Date.
type nullDate struct {
	date *model.Date
}

// Scan реализует sql.Scanner.
func (d *nullDate) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		d.date = nil
	case time.Time:
		date := model.NewDate(value)
		d.date = &date
	case string:
		return d.parse(value)
	case []byte:
		return d.parse(string(value))
	default:
		return fmt.Errorf("unsupported date value %T", src)
	}

	return nil
}

// parse разбирает дату формата YYYY-MM-DD — так SQLite хранит birth_date.
func (d *nullDate) parse(value string) error {
	date, err := model.ParseDate(value)
	if err != nil {
		return fmt.Errorf("parse date %q: %w", value, err)
	}

	d.date = &date

	return nil
}
