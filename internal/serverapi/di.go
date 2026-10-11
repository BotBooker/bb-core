package serverapi

import (
	"context"
	"fmt"

	"github.com/botbooker/bb-core/internal/cache"
	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/internal/events"
	"github.com/botbooker/bb-core/internal/repository"
	"github.com/botbooker/bb-core/internal/service"
)

// newDatabase подключается к базе данных.
//
// Переменная, а не прямой вызов: unit-тесты DI подменяют подключение,
// чтобы не ходить в настоящую базу данных.
var newDatabase = newDatabaseByDSN

// newPostgresDB и newSQLiteDB — конструкторы драйверов.
// Подменяются в тестах, чтобы проверить выбор драйвера по DSN.
var (
	newPostgresDB = postgres.New
	newSQLiteDB   = sqlite.New
)

// newDatabaseByDSN выбирает драйвер по строке подключения.
// Отдельного поля с диалектом нет: он определяется из DSN, поэтому
// нельзя случайно подключиться не тем драйвером.
// Само подключение живёт в пакетах-драйверах: у них нет общей точки входа,
// потому что драйверы зависят от database.DB, а не наоборот.
func newDatabaseByDSN(ctx context.Context) (database.DB, error) {
	cfg := config.DBConfig()

	dialect, err := database.DialectFromDSN(cfg.DSN)
	if err != nil {
		return nil, err
	}

	switch dialect {
	case database.DialectPostgres:
		return newPostgresDB(ctx, *cfg)
	case database.DialectSQLite:
		return newSQLiteDB(ctx, *cfg)
	default:
		// Недостижимо: DialectFromDSN возвращает только константы выше.
		return nil, fmt.Errorf("%w: %q", database.ErrUnsupportedDialect, dialect)
	}
}

// diContainer — контейнер зависимостей с ленивой инициализацией.
//
// Это тот же контейнер из видео про DI, но при создании каждого ресурса
// с методом Close() мы сразу регистрируем его в глобальном closer.
// closer.Add() вызывается прямо в геттере — одна строка, и ресурс
// автоматически закроется при graceful shutdown в правильном LIFO-порядке.
//
// БД подключается сразу в newDIContainer: без неё приложение работать не может,
// поэтому ошибка возвращается вызывающему коду, а не приводится к os.Exit.
// БД создаётся первой — значит закроется последней (LIFO): кэш закроется раньше БД.
type diContainer struct {
	// Инфраструктура
	db       database.DB
	cache    cache.Cache
	eventBus events.EventBus

	// Репозитории
	userRepo         repository.UserRepo
	sessionRepo      repository.SessionRepo
	notificationRepo repository.NotificationRepo

	// Сервисы
	authService         service.AuthService
	userService         service.UserService
	notificationService service.NotificationService

	// API
	handler Handler
}

// newDIContainer создаёт контейнер и подключается к базе данных.
// Ошибка подключения возвращается сразу: приложению без БД работать нечем.
func newDIContainer(ctx context.Context) (*diContainer, error) {
	db, err := newDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("init database: %w", err)
	}

	// При создании — сразу регистрируем Close() в глобальном closer.
	closer.Add("database", func(_ context.Context) error {
		return db.Close()
	})

	return &diContainer{db: db}, nil
}

// DB возвращает подключение к базе данных, созданное вместе с контейнером.
func (d *diContainer) DB() database.DB {
	return d.db
}

// EventBus возвращает шину событий.
func (d *diContainer) EventBus() events.EventBus {
	if d.eventBus == nil {
		d.eventBus = events.NewEventBus()
	}

	return d.eventBus
}

// Cache возвращает подключение к кэшу.
// При создании — сразу регистрирует Close() в глобальном closer.
// Кэш создаётся после БД — значит закроется раньше БД (LIFO).
func (d *diContainer) Cache() cache.Cache {
	if d.cache == nil {
		c := cache.New(config.AppConfig().RedisAddr)

		closer.Add("cache", func(_ context.Context) error {
			return c.Close()
		})

		d.cache = c
	}

	return d.cache
}

// UserRepo возвращает репозиторий пользователей.
func (d *diContainer) UserRepo() repository.UserRepo {
	if d.userRepo == nil {
		d.userRepo = repository.NewUserRepo(d.DB())
	}

	return d.userRepo
}

// SessionRepo возвращает репозиторий сессий.
func (d *diContainer) SessionRepo() repository.SessionRepo {
	if d.sessionRepo == nil {
		d.sessionRepo = repository.NewSessionRepo(d.DB(), d.Cache())
	}

	return d.sessionRepo
}

// NotificationRepo возвращает репозиторий уведомлений.
func (d *diContainer) NotificationRepo() repository.NotificationRepo {
	if d.notificationRepo == nil {
		d.notificationRepo = repository.NewNotificationRepo(d.DB())
	}

	return d.notificationRepo
}

// AuthService возвращает сервис авторизации.
func (d *diContainer) AuthService() service.AuthService {
	if d.authService == nil {
		d.authService = service.NewAuthService(d.UserRepo(), d.SessionRepo(), d.Cache(), d.EventBus())
	}

	return d.authService
}

// UserService возвращает сервис пользователей.
func (d *diContainer) UserService() service.UserService {
	if d.userService == nil {
		d.userService = service.NewUserService(d.UserRepo(), d.AuthService(), d.EventBus())
	}

	return d.userService
}

// NotificationService возвращает сервис уведомлений.
func (d *diContainer) NotificationService() service.NotificationService {
	if d.notificationService == nil {
		d.notificationService = service.NewNotificationService(d.NotificationRepo(), d.UserService(), d.EventBus())
	}

	return d.notificationService
}

// Handler возвращает HTTP-хендлер.
func (d *diContainer) Handler() Handler {
	if d.handler == nil {
		d.handler = NewHandler(
			d.UserService(),
			d.AuthService(),
			d.NotificationService(),
		)
	}

	return d.handler
}
