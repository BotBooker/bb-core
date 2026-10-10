package main

import (
	"errors"
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

func TestNewServerAPI_DefaultFactory(t *testing.T) {
	s := newServerAPI()
	if s == nil {
		t.Fatal("default newServerAPI() returned nil, want non-nil server")
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
	withServer(t, &fakeServer{})

	if code := Main(); code != 0 {
		t.Errorf("Main() = %d, want 0", code)
	}
}

func TestMain_Error(t *testing.T) {
	withServer(t, &fakeServer{runFunc: func() error { return errors.New("server failed") }})

	if code := Main(); code != 1 {
		t.Errorf("Main() = %d, want 1", code)
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
	withServer(t, &fakeServer{runFunc: func() error { return errors.New("server failed") }})

	main()

	if *code != 1 {
		t.Errorf("main() exit code = %d, want 1", *code)
	}
}

func TestMain_NoExitOnSuccess(t *testing.T) {
	code := withOSExit(t)
	withServer(t, &fakeServer{})

	main()

	if *code != -1 {
		t.Errorf("main() called osExit(%d) on success, want no exit", *code)
	}
}
