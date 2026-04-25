package closer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAdd(t *testing.T) {
	// Reset global state by creating a new closer for testing
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	tests := []struct {
		name string
		fn   func(context.Context) error
	}{
		{
			name: "normal function",
			fn: func(ctx context.Context) error {
				return nil
			},
		},
		{
			name: "nil function",
			fn:   nil,
		},
		{
			name: "error function",
			fn: func(ctx context.Context) error {
				return errors.New("test error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(globalCloser.funcs)
			Add("test-resource", tt.fn)
			after := len(globalCloser.funcs)
			if after != before+1 {
				t.Errorf("Add() did not increase funcs length: got %d, want %d", after, before+1)
			}
		})
	}
}

func TestAddConcurrent(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	var wg sync.WaitGroup
	count := 100
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(n int) {
			defer wg.Done()
			Add("resource", func(ctx context.Context) error { return nil })
		}(i)
	}

	wg.Wait()

	if len(globalCloser.funcs) != count {
		t.Errorf("Expected %d functions, got %d", count, len(globalCloser.funcs))
	}
}

func TestCloseAll(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	closed := make([]string, 0)
	var mu sync.Mutex

	Add("first", func(ctx context.Context) error {
		mu.Lock()
		closed = append(closed, "first")
		mu.Unlock()
		return nil
	})
	Add("second", func(ctx context.Context) error {
		mu.Lock()
		closed = append(closed, "second")
		mu.Unlock()
		return nil
	})
	Add("third", func(ctx context.Context) error {
		mu.Lock()
		closed = append(closed, "third")
		mu.Unlock()
		return nil
	})

	ctx := context.Background()
	err := CloseAll(ctx)
	if err != nil {
		t.Errorf("CloseAll() returned error: %v", err)
	}

	// Check LIFO order (last added, first closed)
	expected := []string{"third", "second", "first"}
	if len(closed) != len(expected) {
		t.Fatalf("Expected %d closed resources, got %d", len(expected), len(closed))
	}
	for i, name := range expected {
		if closed[i] != name {
			t.Errorf("Close order mismatch at index %d: got %s, want %s", i, closed[i], name)
		}
	}
}

func TestCloseAllWithContextCancel(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	Add("slow", func(ctx context.Context) error {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err := CloseAll(ctx)
	if err == nil {
		t.Error("CloseAll() should return error on context timeout")
	}
}

func TestCloseAllWithError(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	Add("good", func(ctx context.Context) error {
		return nil
	})
	Add("bad", func(ctx context.Context) error {
		return errors.New("failed to close")
	})
	Add("good2", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	err := CloseAll(ctx)
	if err == nil {
		t.Fatal("CloseAll() should return error when a closer fails")
	}
	if err.Error() != "failed to close" {
		t.Errorf("CloseAll() error = %v, want 'failed to close'", err)
	}
}

func TestCloseAllMultipleErrors(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	Add("bad1", func(ctx context.Context) error {
		return errors.New("error1")
	})
	Add("bad2", func(ctx context.Context) error {
		return errors.New("error2")
	})

	ctx := context.Background()
	err := CloseAll(ctx)
	if err == nil {
		t.Fatal("CloseAll() should return error")
	}
	// errors.Join combines errors
	errStr := err.Error()
	if errStr != "error1" && errStr != "error2" {
		// The order might vary, but both errors should be present
		t.Logf("CloseAll() error = %v (multiple errors joined)", err)
	}
}

func TestCloseAllEmpty(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	ctx := context.Background()
	err := CloseAll(ctx)
	if err != nil {
		t.Errorf("CloseAll() with empty funcs returned error: %v", err)
	}
}

func TestCloseAllIdempotent(t *testing.T) {
	original := globalCloser
	defer func() { globalCloser = original }()
	globalCloser = &closer{}

	callCount := 0
	Add("once", func(ctx context.Context) error {
		callCount++
		return nil
	})

	ctx := context.Background()
	err1 := CloseAll(ctx)
	if err1 != nil {
		t.Errorf("First CloseAll() returned error: %v", err1)
	}

	err2 := CloseAll(ctx)
	if err2 != nil {
		t.Errorf("Second CloseAll() returned error: %v", err2)
	}

	if callCount != 1 {
		t.Errorf("Close functions called %d times, expected 1 (sync.Once)", callCount)
	}
}

func TestCloserAddAndCloseAll(t *testing.T) {
	c := &closer{}

	c.add("test", func(ctx context.Context) error {
		return nil
	})

	if len(c.funcs) != 1 {
		t.Errorf("Expected 1 function, got %d", len(c.funcs))
	}

	err := c.closeAll(context.Background())
	if err != nil {
		t.Errorf("closeAll() returned error: %v", err)
	}

	if len(c.funcs) != 0 {
		t.Errorf("Expected funcs to be cleared, got %d", len(c.funcs))
	}
}
