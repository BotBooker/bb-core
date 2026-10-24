package main

import (
	"errors"
	"flag"
	"testing"
)

// fakeServer — мок-реализация runner для тестов.
type fakeServer struct {
	runFunc func() error
}

// Run возвращает результат runFunc или nil, если он не задан.
func (f *fakeServer) Run() error {
	if f.runFunc != nil {
		return f.runFunc()
	}

	return nil
}

// withServer временно подменяет фабрику сервера моком и возвращает
// исходное значение при завершении теста.
func withServer(t *testing.T, s runner) {
	t.Helper()

	original := newServerAPI
	newServerAPI = func() runner { return s }
	t.Cleanup(func() { newServerAPI = original })
}

// withArgs временно подменяет аргументы командной строки, которые разбирает
// Main. Без этого Main() в тестах разбирал бы флаги `go test`
// (-test.timeout, -test.paniconexit0) и завершался бы с кодом 2.
func withArgs(t *testing.T, argv ...string) {
	t.Helper()

	original := args
	args = argv
	t.Cleanup(func() { args = original })
}

func TestNewServerAPI_DefaultFactory(t *testing.T) {
	s := newServerAPI()
	if s == nil {
		t.Fatal("default newServerAPI() returned nil, want non-nil server")
	}
}

// TestParseArgs проверяет разбор аргументов: режим запуска задаётся только
// флагом --readycheck, всё остальное — ошибка разбора.
func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		want     bool
		wantErr  bool
		wantHelp bool
	}{
		{name: "no flags", argv: nil, want: false},
		{name: "readycheck with double dash", argv: []string{"--readycheck"}, want: true},
		{name: "readycheck with single dash", argv: []string{"-readycheck"}, want: true},
		{name: "help", argv: []string{"-h"}, wantErr: true, wantHelp: true},
		{name: "unknown flag", argv: []string{"--ready"}, wantErr: true},
		{name: "positional argument", argv: []string{"readycheck"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.argv)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%v) error = %v, wantErr %v", tt.argv, err, tt.wantErr)
			}

			if tt.wantHelp && !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("parseArgs(%v) error = %v, want flag.ErrHelp", tt.argv, err)
			}

			if got != tt.want {
				t.Errorf("parseArgs(%v) = %v, want %v", tt.argv, got, tt.want)
			}
		})
	}
}

func TestRun_Success(t *testing.T) {
	called := false
	withServer(t, &fakeServer{runFunc: func() error {
		called = true

		return nil
	}})

	if err := run(); err != nil {
		t.Fatalf("run() returned unexpected error: %v", err)
	}

	if !called {
		t.Error("run() did not call server.Run()")
	}
}

func TestRun_Error(t *testing.T) {
	wantErr := errors.New("server failed")
	withServer(t, &fakeServer{runFunc: func() error { return wantErr }})

	err := run()
	if err == nil {
		t.Fatal("run() expected error, got nil")
	}

	if !errors.Is(err, wantErr) {
		t.Errorf("run() error = %v, want %v", err, wantErr)
	}
}

func TestMain_Success(t *testing.T) {
	withArgs(t)
	withServer(t, &fakeServer{})

	if code := Main(); code != 0 {
		t.Errorf("Main() = %d, want 0", code)
	}
}

func TestMain_Error(t *testing.T) {
	withArgs(t)
	withServer(t, &fakeServer{runFunc: func() error { return errors.New("server failed") }})

	if code := Main(); code != 1 {
		t.Errorf("Main() = %d, want 1", code)
	}
}

// TestMain_UnknownFlag проверяет, что опечатка во флаге не приводит к запуску
// сервера: процесс завершается кодом 2, как принято у пакета flag.
func TestMain_UnknownFlag(t *testing.T) {
	withArgs(t, "--ready")
	withServer(t, &fakeServer{runFunc: func() error {
		t.Error("Main() started the server on unknown flag")

		return nil
	}})

	if code := Main(); code != exitCodeUsage {
		t.Errorf("Main() = %d, want %d", code, exitCodeUsage)
	}
}

// TestMain_Help проверяет, что -h — успешный режим: справка печатается, сервер
// не стартует.
func TestMain_Help(t *testing.T) {
	withArgs(t, "-h")
	withServer(t, &fakeServer{runFunc: func() error {
		t.Error("Main() started the server on -h")

		return nil
	}})

	if code := Main(); code != 0 {
		t.Errorf("Main() = %d, want 0", code)
	}
}

// withOSExit временно подменяет osExit, чтобы вызвать main() без завершения
// процесса тестирования, и возвращает указатель на записанный код выхода.
// Если osExit не был вызван, значение остаётся равным -1.
func withOSExit(t *testing.T) *int {
	t.Helper()

	original := osExit
	code := -1
	osExit = func(c int) { code = c }
	t.Cleanup(func() { osExit = original })

	return &code
}

func TestMain_ExitsOnError(t *testing.T) {
	code := withOSExit(t)
	withArgs(t)
	withServer(t, &fakeServer{runFunc: func() error { return errors.New("server failed") }})

	main()

	if *code != 1 {
		t.Errorf("main() exit code = %d, want 1", *code)
	}
}

func TestMain_NoExitOnSuccess(t *testing.T) {
	code := withOSExit(t)
	withArgs(t)
	withServer(t, &fakeServer{})

	main()

	if *code != -1 {
		t.Errorf("main() called osExit(%d) on success, want no exit", *code)
	}
}
