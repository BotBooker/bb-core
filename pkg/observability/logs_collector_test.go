package observability

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"

	"github.com/botbooker/bb-core/pkg/logger"
)

// otelLogReceiver — минимальный OTLP-коллектор логов для тестов: принимает
// ExportLogsServiceRequest и считает полученные записи.
type otelLogReceiver struct {
	collectorlogspb.UnimplementedLogsServiceServer

	mu      sync.Mutex
	records int
}

func (r *otelLogReceiver) Export(
	_ context.Context,
	req *collectorlogspb.ExportLogsServiceRequest,
) (*collectorlogspb.ExportLogsServiceResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, resourceLogs := range req.GetResourceLogs() {
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			r.records += len(scopeLogs.GetLogRecords())
		}
	}

	return &collectorlogspb.ExportLogsServiceResponse{}, nil
}

func (r *otelLogReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.records
}

// startLogCollector поднимает gRPC-сервер OTLP-логов на свободном порту и
// возвращает приёмник и его адрес.
func startLogCollector(t *testing.T) (*otelLogReceiver, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось занять порт под коллектор: %v", err)
	}

	receiver := &otelLogReceiver{}
	server := grpc.NewServer()

	collectorlogspb.RegisterLogsServiceServer(server, receiver)

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return receiver, listener.Addr().String()
}

// TestSetupLogs_ExportsToCollector — сквозная проверка: логи приложения уходят в
// OTel Collector по OTLP/gRPC, когда задан адрес коллектора.
func TestSetupLogs_ExportsToCollector(t *testing.T) {
	receiver, addr := startLogCollector(t)

	t.Setenv(envLogsEndpoint, "")
	t.Setenv(envCommonEndpoint, "http://"+addr)

	previous := slog.Default()
	t.Cleanup(func() {
		logger.SetOTelHandler(nil)
		slog.SetDefault(previous)
	})

	// Как в приложении: логгер инициализируется до OTel, уровень — DEBUG, чтобы
	// тест не зависел от окружения (LOG_LEVEL в CI может быть выше).
	logger.InitAndSetDefault(slog.LevelDebug, logger.LogFormatText)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := setupLogs(ctx)
	if err != nil {
		t.Fatalf("setupLogs() вернул ошибку: %v", err)
	}

	slog.Info("exported to collector")

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() не смог выгрузить логи: %v", err)
	}

	if got := receiver.count(); got == 0 {
		t.Error("коллектор не получил ни одной записи логов")
	}
}
