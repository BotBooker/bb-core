package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
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
	// ListAll возвращает активных пользователей с пагинацией: для глобальных
	// администраторов.
	ListAll(ctx context.Context, limit, offset int) ([]*model.User, error)
	// ListVisible возвращает пользователей, видимых staff тенанта: сотрудников
	// тенанта и клиентов, у которых есть хотя бы одна бронь в этом тенанте.
	ListVisible(ctx context.Context, tenantID string, limit, offset int) ([]*model.User, error)
	// GetVisible возвращает пользователя, видимого staff тенанта, или
	// database.ErrNotFound.
	GetVisible(ctx context.Context, tenantID, id string) (*model.User, error)
}

// userColumns — колонки users в том порядке, в котором их читает scanUser.
// Порядок должен совпадать: рассинхрон ловят тесты репозитория.
// Колонки роли здесь нет: роль хранится в членстве (tenant_members.role_id).
// is_superuser/is_global_operator — системный уровень доступа (см. model.User):
// они принадлежат профилю, а не членству, поэтому читаются вместе с ним.
const userColumns = "id, email, first_name, last_name, middle_name, phone_number," +
	" gender, birth_date, created_at, updated_at, last_login_at, is_active, is_delete," +
	" is_superuser, is_global_operator"

// aliasedUserColumns — те же колонки с алиасом u: для JOIN с tenant_members и
// bookings, где имя id/email иначе неоднозначно.
const aliasedUserColumns = "u.id, u.email, u.first_name, u.last_name, u.middle_name, u.phone_number," +
	" u.gender, u.birth_date, u.created_at, u.updated_at, u.last_login_at, u.is_active, u.is_delete," +
	" u.is_superuser, u.is_global_operator"

// activeAliasedUserFilter — условие «строка живая» для JOIN-запросов.
const activeAliasedUserFilter = "u.is_active = true AND u.is_delete = false"

// activeUserFilter — условие «строка живая»: удалённый пользователь
// (is_active = false, is_delete = true) не виден ни по id, ни по email.
const activeUserFilter = " AND is_active = true AND is_delete = false"

// userRepo — конкретная реализация, скрыта от внешних пакетов.
type userRepo struct {
	db UserDB
	codec
}

// NewUserRepo создаёт репозиторий пользователей.
//
// Диалект нужен для двух вещей: подстановки плейсхолдеров ($1 в PostgreSQL,
// ? в SQLite) и кодирования времени (TIMESTAMPTZ против строки RFC3339).
func NewUserRepo(ctx context.Context, db UserDB, dialect string) UserRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewUserRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "user repository created", "dialect", dialect)

	return &userRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет активного пользователя.
//
// Роль не записывается: её хранит членство в тенанте (tenant_members), а
// профиль в users колонки с ролью не имеет. Системные флаги is_superuser и
// is_global_operator — часть профиля, поэтому пишутся вместе с ним.
func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Create")
	defer spanEnd()

	const query = `INSERT INTO users` +
		` (id, email, first_name, last_name, middle_name, phone_number,` +
		` gender, birth_date, created_at, updated_at, last_login_at, is_active, is_delete,` +
		` is_superuser, is_global_operator)` +
		` VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := r.db.ExecContext(traceCtx, r.rebind(query),
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.MiddleName,
		user.PhoneNumber,
		string(user.Gender),
		r.dateArg(user.BirthDate),
		r.timeArg(user.CreatedAt),
		r.timeArg(user.UpdatedAt),
		r.nullTimeArg(user.LastLoginAt),
		user.IsActive,
		user.IsDelete,
		user.IsSuperuser,
		user.IsGlobalOperator,
	); err != nil {
		return fmt.Errorf("insert user: %w", database.MapError(err))
	}

	return nil
}

// GetByID возвращает активного пользователя по идентификатору (UUID).
// Удалённые (is_active = false, is_delete = true) записи не видны: soft delete
// делает их недоступными так же, как обычное удаление.
func (r *userRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByID")
	defer spanEnd()

	const query = "SELECT " + userColumns + " FROM users WHERE id = ?" + activeUserFilter

	return r.getBy(traceCtx, query, id)
}

// GetByEmail возвращает активного пользователя по email: вход и восстановление
// доступа.
//
// Фильтр «строка живая» обязателен: после soft delete в таблице может
// остаться несколько строк с одним email (одна активная и любое число
// удалённых), и без фильтра QueryRow вернул бы произвольную из них.
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByEmail")
	defer spanEnd()

	const query = "SELECT " + userColumns + " FROM users WHERE email = ?" + activeUserFilter

	return r.getBy(traceCtx, query, email)
}

// Update перезаписывает изменяемые поля активного пользователя, включая
// системные флаги глобального доступа.
// Отсутствие активной строки — database.ErrNotFound, а не тихий успех:
// удалённого пользователя менять нельзя.
func (r *userRepo) Update(ctx context.Context, user *model.User) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Update")
	defer spanEnd()

	const query = `UPDATE users SET email = ?, first_name = ?, last_name = ?, middle_name = ?,` +
		` phone_number = ?, gender = ?, birth_date = ?, updated_at = ?, last_login_at = ?,` +
		` is_superuser = ?, is_global_operator = ?` +
		` WHERE id = ? AND is_active = true AND is_delete = false`

	result, err := r.db.ExecContext(traceCtx, r.rebind(query),
		user.Email,
		user.FirstName,
		user.LastName,
		user.MiddleName,
		user.PhoneNumber,
		string(user.Gender),
		r.dateArg(user.BirthDate),
		r.timeArg(user.UpdatedAt),
		r.nullTimeArg(user.LastLoginAt),
		user.IsSuperuser,
		user.IsGlobalOperator,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", database.MapError(err))
	}

	return requireAffected(result, "update user")
}

// Delete помечает пользователя удалённым: строка остаётся, is_active = false и
// is_delete = true.
//
// Это soft delete, а не DELETE: так сохраняется история (токены, учётные
// данные) и освобождается email — частичный уникальный индекс users_email_idx
// держит уникальность только среди активных строк. Оба признака
// (is_active/is_delete) выставляются вместе, как в ERDB для пользователей и
// фото; условие is_active = true делает повторное удаление ошибкой ErrNotFound,
// а не тихим успехом.
func (r *userRepo) Delete(ctx context.Context, id string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Delete")
	defer spanEnd()

	const query = "UPDATE users SET is_active = false, is_delete = true WHERE id = ? AND is_active = true"

	// Значение приходит от вызывающего слоя, поэтому проверяется до запроса:
	// невалидный id отклоняется здесь, а не отдаётся драйверу как аргумент.
	if err := model.ValidateUUID(id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// Запрос — константа с плейсхолдером, id передаётся параметром. Bearer в
	// форме ExecContext с одним аргументом принимает этот аргумент за тело
	// запроса, поэтому срабатывание правила go_gosec_sql_concat_sqli ложное.
	// bearer:disable go_gosec_sql_concat_sqli
	result, err := r.db.ExecContext(traceCtx, r.rebind(query), id)
	if err != nil {
		return fmt.Errorf("delete user: %w", database.MapError(err))
	}

	return requireAffected(result, "delete user")
}

// getBy выполняет SELECT одной строки и читает её в модель.
// ListAll возвращает активных пользователей с пагинацией.
//
// Глобальный список доступен только глобальным администраторам; сервис
// проверяет это до вызова, поэтому репозиторий остаётся tenant-agnostic.
func (r *userRepo) ListAll(ctx context.Context, limit, offset int) ([]*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListAll")
	defer spanEnd()

	query := "SELECT " + userColumns + " FROM users WHERE is_active = true AND is_delete = false" +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?"

	return r.list(traceCtx, query, limit, offset)
}

// ListVisible возвращает пользователей, видимых staff тенанта.
//
// Правило видимости (ADR 0001 §7): сотрудники видят друг друга в своём
// тенанте, а клиента — только если у него есть хотя бы одна бронь в этом
// тенанте, включая отменённые. EXISTS по bookings не зависит от статуса брони:
// история броней и есть основание видеть клиента.
func (r *userRepo) ListVisible(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListVisible")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select visible users: %w", err)
	}

	query := "SELECT " + aliasedUserColumns + " FROM users u" +
		" JOIN tenant_members m ON m.user_id = u.id" +
		" WHERE m.tenant_id = ? AND " + activeAliasedUserFilter +
		" AND (m.role_id <> ? OR EXISTS (" +
		"SELECT 1 FROM bookings b WHERE b.tenant_id = ? AND b.client_id = u.id))" +
		" ORDER BY u.created_at DESC LIMIT ? OFFSET ?"

	return r.list(traceCtx, query, tenantID, model.RoleIDClient, tenantID, limit, offset)
}

// GetVisible возвращает пользователя, видимого staff тенанта.
func (r *userRepo) GetVisible(ctx context.Context, tenantID, id string) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetVisible")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select visible user: %w", err)
	}

	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("select visible user: %w", err)
	}

	query := "SELECT " + aliasedUserColumns + " FROM users u" +
		" JOIN tenant_members m ON m.user_id = u.id" +
		" WHERE m.tenant_id = ? AND u.id = ? AND " + activeAliasedUserFilter +
		" AND (m.role_id <> ? OR EXISTS (" +
		"SELECT 1 FROM bookings b WHERE b.tenant_id = ? AND b.client_id = u.id))"

	user, err := scanUser(r.db.QueryRowContext(traceCtx, r.rebind(query),
		tenantID, id, model.RoleIDClient, tenantID))
	if err != nil {
		return nil, fmt.Errorf("select visible user: %w", database.MapError(err))
	}

	return user, nil
}

// list выполняет списочный запрос и читает строки в модели.
func (r *userRepo) list(ctx context.Context, query string, args ...any) ([]*model.User, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", database.MapError(err))
	}
	defer func() { _ = rows.Close() }()

	users := make([]*model.User, 0)

	for rows.Next() {
		user, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("select users: %w", scanErr)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select users: %w", database.MapError(err))
	}

	return users, nil
}

func (r *userRepo) getBy(ctx context.Context, query string, args ...any) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "getBy")
	defer spanEnd()

	user, err := scanUser(r.db.QueryRowContext(traceCtx, r.rebind(query), args...))
	if err != nil {
		return nil, fmt.Errorf("select user: %w", database.MapError(err))
	}

	return user, nil
}

// scanUser читает строку users в доменную модель.
//
// Время и дату разбирают обёртки nullTime и nullDate: PostgreSQL отдаёт
// time.Time, SQLite — строку в формате, который задал репозиторий.
func scanUser(row scanner) (*model.User, error) {
	var (
		user                 model.User
		gender               string
		birthDate            nullDate
		createdAt, updatedAt nullTime
		lastLogin            nullTime
	)

	// Флаги (is_active, is_delete, is_superuser, is_global_operator) читаются в
	// bool: PostgreSQL отдаёт BOOLEAN, SQLite — INTEGER (0/1), и database/sql
	// приводит оба вида через driver.Bool.
	if err := row.Scan(
		&user.ID,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.MiddleName,
		&user.PhoneNumber,
		&gender,
		&birthDate,
		&createdAt,
		&updatedAt,
		&lastLogin,
		&user.IsActive,
		&user.IsDelete,
		&user.IsSuperuser,
		&user.IsGlobalOperator,
	); err != nil {
		return nil, err
	}

	user.Gender = model.Gender(gender)
	user.BirthDate = birthDate.date
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
