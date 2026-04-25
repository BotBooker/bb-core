package serverapi

import (
	"context"
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/api"
	"github.com/botbooker/bb-core/internal/cache"
	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/events"
	"github.com/botbooker/bb-core/internal/repository"
	"github.com/botbooker/bb-core/internal/service"
)

// diContainer — контейнер зависимостей с ленивой инициализацией.
//
// Это тот же контейнер из видео про DI, но при создании каждого ресурса
// с методом Close() мы сразу регистрируем его в глобальном closer.
// closer.Add() вызывается прямо в геттере — одна строка, и ресурс
// автоматически закроется при graceful shutdown в правильном LIFO-порядке.
//
// Почему порядок закрытия детерминирован при ленивой инициализации:
// initDeps() вызывается в одной горутине. Ленивая инициализация — это
// depth-first обход графа зависимостей. Порядок определяется графом:
//
//	Handler() → UserService() → UserRepo() → DB()              ← 1-й closer.Add
//	                           → AuthService() → SessionRepo() → Cache()  ← 2-й closer.Add
//
// DB всегда создаётся раньше Cache (UserRepo запрашивает DB до того,
// как SessionRepo запросит Cache). Граф не меняется → порядок не меняется.
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
	handler api.Handler
}

// newDIContainer создаёт новый пустой контейнер.
func newDIContainer() *diContainer {
	return &diContainer{}
}

// DB возвращает подключение к базе данных.
// При создании — сразу регистрирует Close() в глобальном closer.
// БД создаётся одной из первых — значит закроется одной из последних (LIFO).
func (d *diContainer) DB() database.DB {
	if d.db == nil {
		db, err := database.New(config.AppConfig().DSN)
		if err != nil {
			slog.Error("не удалось подключиться к БД", "err", err)
			os.Exit(1)
		}

		closer.Add("база данных", func(_ context.Context) error {
			return db.Close()
		})

		d.db = db
	}

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

		closer.Add("кэш", func(_ context.Context) error {
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
func (d *diContainer) Handler() api.Handler {
	if d.handler == nil {
		d.handler = api.NewHandler(
			d.UserService(),
			d.AuthService(),
			d.NotificationService(),
		)
	}

	return d.handler
}
