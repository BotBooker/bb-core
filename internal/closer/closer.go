package closer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// closeFn — одна функция закрытия с именем ресурса.
// Имя нужно для логирования: при shutdown видно, какой именно ресурс закрывается.
type closeFn struct {
	name string
	fn   func(context.Context) error
}

// closer управляет graceful shutdown приложения.
//
// Принцип работы — как defer: последний добавленный ресурс закрывается первым (LIFO).
// Это важно для зависимостей: если кэш добавлен после базы данных,
// то при shutdown кэш закроется первым, а база — последней.
// Так гарантируется, что ни один ресурс не обращается к уже закрытой зависимости.
//
// Потокобезопасен: Add() можно вызывать из разных горутин
// при ленивой инициализации зависимостей в DI-контейнере.
// CloseAll() безопасен для повторного вызова — выполнится только один раз (sync.Once).
//
// Структура приватная — снаружи пакета доступны только функции Add() и CloseAll(),
// работающие с глобальным экземпляром.
type closer struct {
	mu    sync.Mutex // защищает слайс funcs от конкурентной записи
	once  sync.Once  // гарантирует что CloseAll выполнится только один раз
	funcs []closeFn  // накопленные функции закрытия в порядке добавления
}

// globalCloser — глобальный экземпляр.
// Позволяет вызывать closer.Add() и closer.CloseAll() из любого места,
// не передавая экземпляр через конструкторы и DI-контейнер.
var globalCloser = &closer{}

// Add добавляет функцию закрытия в глобальный closer.
// Вызывается при создании каждого ресурса, например:
//
//	closer.Add("база данных", func(_ context.Context) error {
//	    return db.Close()
//	})
func Add(name string, fn func(context.Context) error) {
	globalCloser.add(name, fn)
}

// CloseAll вызывает все функции закрытия глобального closer-а в обратном порядке (LIFO).
// Принимает context с таймаутом — если ресурс не закрылся вовремя, context отменится.
//
// Важно: таймаут в ctx — общий на все ресурсы, а не на каждый по отдельности.
// Если БД закрывалась 20 секунд из 25 — на кэш останется только 5.
//
// Безопасен для повторного вызова — выполнится только один раз.
func CloseAll(ctx context.Context) error {
	return globalCloser.closeAll(ctx)
}

// add добавляет функцию закрытия с именем ресурса.
func (c *closer) add(name string, fn func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.funcs = append(c.funcs, closeFn{name: name, fn: fn})
}

// closeAll вызывает все зарегистрированные функции закрытия в обратном порядке (LIFO).
//
// Порядок закрытия — обратный порядку добавления:
//
//	closer.Add("база данных", dbClose)    // добавлен первым
//	closer.Add("кэш", cacheClose)         // добавлен вторым
//	closer.Add("HTTP-сервер", srvStop)    // добавлен третьим
//
//	При вызове CloseAll:
//	1. HTTP-сервер  (добавлен последним — закрывается первым)
//	2. кэш
//	3. база данных  (добавлена первой — закрывается последней)
//
// Если один ресурс не закрылся — остальные всё равно закроются.
func (c *closer) closeAll(ctx context.Context) error {
	var result error

	c.once.Do(func() {
		// Забираем все функции под мьютексом и обнуляем слайс,
		// чтобы не держать ссылки на ресурсы после закрытия.
		c.mu.Lock()
		funcs := c.funcs
		c.funcs = nil
		c.mu.Unlock()

		if len(funcs) == 0 {
			return
		}

		slog.Debug("начинаем плавное завершение", "count", len(funcs))

		var errs []error

		// Идём от конца к началу — LIFO, как defer.
		for i := len(funcs) - 1; i >= 0; i-- {
			f := funcs[i]

			start := time.Now()
			slog.Debug("закрываем ресурс", "name", f.name)

			if err := f.fn(ctx); err != nil {
				// Логируем ошибку, но продолжаем закрывать остальные ресурсы.
				slog.Error("ошибка при закрытии ресурса",
					"name", f.name,
					"error", err,
					"duration", time.Since(start),
				)

				errs = append(errs, err)
			} else {
				slog.Debug("ресурс закрыт", "name", f.name, "duration", time.Since(start))
			}
		}

		slog.Info("все ресурсы закрыты")

		result = errors.Join(errs...)
	})

	return result
}
