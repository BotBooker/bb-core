// Package logger предоставляет утилиты для инициализации и настройки логирования.
//
// Основные функции:
//   - Init: инициализирует slog логгер с заданными параметрами.
//   - InitAndSetDefault: инициализирует логгер и делает его глобальным.
//   - SetOTelHandler: подключает к глобальному логгеру обработчик OpenTelemetry,
//     сохраняя привычный вывод в stdout.
//   - MultiHandler: отправляет записи сразу нескольким обработчикам.
//   - TraceHandler: добавляет к записям trace_id активной трассы,
//     чтобы логи можно было связать с трассой OpenTelemetry.
package logger

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "plain"
)

var (
	// handlerMu защищает пересборку глобального логгера.
	handlerMu sync.Mutex
	// baseHandler — обработчик вывода (stdout), установленный InitAndSetDefault.
	baseHandler slog.Handler
	// otelHandler — обработчик OpenTelemetry, подключённый SetOTelHandler.
	otelHandler slog.Handler
	// logLevel — минимальный уровень, установленный InitAndSetDefault.
	logLevel slog.Level
)

// Init инициализирует slog логгер с обработчиком вывода (текстовым или JSON).
//
// Контекстные вызовы логгера (slog.InfoContext и подобные) получают в записи
// trace_id активной трассы — его добавляет TraceHandler (см. initHandler).
//
// Параметры:
//   - level: уровень логирования (например, slog.LevelInfo, slog.LevelDebug).
//   - format: формат вывода (LogFormatJSON или LogFormatText).
//
// Возвращает:
//   - *slog.Logger: настроенный экземпляр логгера.
//
// Пример использования:
//
//	log := logger.Init(slog.LevelInfo)
//	slog.SetDefault(log)
func Init(level slog.Level, format LogFormat) *slog.Logger {
	return slog.New(initHandler(os.Stdout, level, format))
}

// initHandler собирает обработчик вывода логов приложения: json или plain по
// формату, с добавлением trace_id активной трассы к каждой записи.
//
// writer вынесен параметром, чтобы тесты проверяли обработчик на буфере, а не
// на os.Stdout.
func initHandler(writer io.Writer, level slog.Level, format LogFormat) slog.Handler {
	if format == LogFormatJSON {
		return NewTraceHandler(slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: level,
		}))
	}

	return NewTraceHandler(slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
	}))
}

// InitAndSetDefault инициализирует логгер и устанавливает его как глобальный по умолчанию.
//
// Параметры:
//   - level: уровень логирования (например, slog.LevelInfo, slog.LevelDebug).
//
// Возвращает:
//   - *slog.Logger: настроенный экземпляр логгера.
//
// Пример использования:
//
//	log := logger.InitAndSetDefault(slog.LevelInfo)
func InitAndSetDefault(level slog.Level, format LogFormat) *slog.Logger {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	baseHandler = Init(level, format).Handler()
	logLevel = level

	log := slog.New(composeLocked())
	slog.SetDefault(log)

	return log
}

// composeLocked собирает обработчик глобального логгера из вывода в stdout и,
// если он подключён, OTel-обработчика с тем же ограничением уровня.
func composeLocked() slog.Handler {
	if otelHandler == nil {
		return baseHandler
	}

	return NewMultiHandler(baseHandler, withLevel(otelHandler, logLevel))
}

// SetOTelHandler подключает обработчик OpenTelemetry к глобальному логгеру,
// сохраняя привычный вывод приложения: каждая запись веером уходит и в
// обработчик, установленный InitAndSetDefault, и в OTel. Уровень тот же, что у
// вывода в stdout. Повторный вызов заменяет ранее подключённый OTel-обработчик,
// а не наслаивает его; передача nil отключает экспорт в OTel.
//
// Функцию вызывает код, инициализирующий OTel SDK, после того как логгер создан
// InitAndSetDefault.
//
// Пример использования:
//
//	logger.SetOTelHandler(otelslog.NewHandler("my-service"))
func SetOTelHandler(handler slog.Handler) {
	handlerMu.Lock()
	defer handlerMu.Unlock()

	if baseHandler == nil {
		// InitAndSetDefault ещё не вызывался (например, в тестах): пишем в stdout
		// обычным текстовым обработчиком через initHandler — он же добавляет
		// trace_id активной трассы. Брать slog.Default().Handler()
		// нельзя: это stdlib defaultHandler, который выводит через пакет log, а
		// slog.SetDefault переводит log.SetOutput на итоговый обработчик — вышла
		// бы бесконечная рекурсия и дедлок на mutex пакета log.
		baseHandler = initHandler(os.Stdout, slog.LevelInfo, LogFormatText)
	}

	otelHandler = handler

	slog.SetDefault(slog.New(composeLocked()))
}

// parseLogLevel parses a slog.Level from a string.
// Returns the default level if the input is invalid.
func ParseLogLevel(level string, defaultLevel slog.Level) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return defaultLevel
	}
}
