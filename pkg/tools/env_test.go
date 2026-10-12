package tools

import (
	"os"
	"reflect"
	"testing"
)

func TestGetEnvOrDefault(t *testing.T) {
	// Save original env
	original := os.Getenv("TEST_KEY")
	defer os.Setenv("TEST_KEY", original)

	tests := []struct {
		name         string
		key          string
		defaultValue string
		setup        func()
		want         string
	}{
		{
			name:         "env var exists",
			key:          "TEST_KEY",
			defaultValue: "default",
			setup: func() {
				os.Setenv("TEST_KEY", "value")
			},
			want: "value",
		},
		{
			name:         "env var does not exist",
			key:          "NONEXISTENT",
			defaultValue: "default",
			setup: func() {
				os.Unsetenv("NONEXISTENT")
			},
			want: "default",
		},
		{
			name:         "env var empty string",
			key:          "TEST_KEY",
			defaultValue: "default",
			setup: func() {
				os.Setenv("TEST_KEY", "")
			},
			want: "default",
		},
		{
			name:         "env var with spaces",
			key:          "TEST_KEY",
			defaultValue: "default",
			setup: func() {
				os.Setenv("TEST_KEY", "  value with spaces  ")
			},
			want: "  value with spaces  ",
		},
		{
			name:         "special characters in value",
			key:          "TEST_KEY",
			defaultValue: "default",
			setup: func() {
				os.Setenv("TEST_KEY", "value!@#$%^&*()")
			},
			want: "value!@#$%^&*()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got := GetEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvList(t *testing.T) {
	// Save original env
	original := os.Getenv("TEST_LIST")
	defer os.Setenv("TEST_LIST", original)

	tests := []struct {
		name         string
		key          string
		defaultValue []string
		setup        func()
		want         []string
	}{
		{
			name:         "env var with comma-separated values",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "a,b,c")
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:         "env var with comma-separated values with spaces",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "a, b, c")
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:         "env var does not exist",
			key:          "NONEXISTENT_LIST",
			defaultValue: []string{"default1", "default2"},
			setup: func() {
				os.Unsetenv("NONEXISTENT_LIST")
			},
			want: []string{"default1", "default2"},
		},
		{
			name:         "env var empty string",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "")
			},
			want: []string{"default"},
		},
		{
			name:         "env var with empty items",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "a,,b,,c")
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:         "env var with only spaces",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "  ,  ,  ")
			},
			want: []string{"default"},
		},
		{
			name:         "single value",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "single")
			},
			want: []string{"single"},
		},
		{
			name:         "empty default",
			key:          "TEST_LIST",
			defaultValue: []string{},
			setup: func() {
				os.Setenv("TEST_LIST", "a,b,c")
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:         "nil default",
			key:          "TEST_LIST",
			defaultValue: nil,
			setup: func() {
				os.Setenv("TEST_LIST", "a,b,c")
			},
			want: []string{"a", "b", "c"},
		},
		{
			name:         "special characters in values",
			key:          "TEST_LIST",
			defaultValue: []string{"default"},
			setup: func() {
				os.Setenv("TEST_LIST", "val!@#,test$%^,foo&*()")
			},
			want: []string{"val!@#", "test$%^", "foo&*()"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got := GetEnvList(tt.key, tt.defaultValue)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetEnvList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvOrDefaultToInt32(t *testing.T) {
	// Save original env
	original := os.Getenv("TEST_INT")
	defer os.Setenv("TEST_INT", original)

	tests := []struct {
		name         string
		key          string
		defaultValue int32
		setup        func()
		want         int32
	}{
		{
			name:         "env var with valid number",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "25")
			},
			want: 25,
		},
		{
			name:         "env var with negative number",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "-5")
			},
			want: -5,
		},
		{
			name:         "env var with zero",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "0")
			},
			want: 0,
		},
		{
			name:         "env var does not exist",
			key:          "NONEXISTENT_INT",
			defaultValue: 10,
			setup: func() {
				os.Unsetenv("NONEXISTENT_INT")
			},
			want: 10,
		},
		{
			name:         "env var empty string",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "")
			},
			want: 10,
		},
		{
			name:         "env var with non-numeric value",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "many")
			},
			want: 10,
		},
		{
			name:         "env var with float value",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "1.5")
			},
			want: 10,
		},
		{
			name:         "env var with int32 overflow",
			key:          "TEST_INT",
			defaultValue: 10,
			setup: func() {
				os.Setenv("TEST_INT", "9999999999999")
			},
			want: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			got := GetEnvOrDefaultToInt32(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetEnvOrDefaultToInt32() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvOrDefaultToBool(t *testing.T) {
	// Save original env
	original := os.Getenv("TEST_BOOL")
	defer os.Setenv("TEST_BOOL", original)

	tests := []struct {
		name         string
		key          string
		defaultValue bool
		setup        func()
		want         bool
	}{
		{
			name:         "env var true",
			key:          "TEST_BOOL",
			defaultValue: false,
			setup: func() {
				os.Setenv("TEST_BOOL", "true")
			},
			want: true,
		},
		{
			name:         "env var 1",
			key:          "TEST_BOOL",
			defaultValue: false,
			setup: func() {
				os.Setenv("TEST_BOOL", "1")
			},
			want: true,
		},
		{
			name:         "env var false overrides default true",
			key:          "TEST_BOOL",
			defaultValue: true,
			setup: func() {
				os.Setenv("TEST_BOOL", "false")
			},
			want: false,
		},
		{
			name:         "env var 0",
			key:          "TEST_BOOL",
			defaultValue: true,
			setup: func() {
				os.Setenv("TEST_BOOL", "0")
			},
			want: false,
		},
		{
			name:         "env var with upper case",
			key:          "TEST_BOOL",
			defaultValue: false,
			setup: func() {
				os.Setenv("TEST_BOOL", "TRUE")
			},
			want: true,
		},
		{
			name:         "env var does not exist",
			key:          "NONEXISTENT_BOOL",
			defaultValue: true,
			setup: func() {
				os.Unsetenv("NONEXISTENT_BOOL")
			},
			want: true,
		},
		{
			name:         "env var empty string",
			key:          "TEST_BOOL",
			defaultValue: true,
			setup: func() {
				os.Setenv("TEST_BOOL", "")
			},
			want: true,
		},
		{
			name:         "env var with non-boolean value",
			key:          "TEST_BOOL",
			defaultValue: true,
			setup: func() {
				os.Setenv("TEST_BOOL", "yes")
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			got := GetEnvOrDefaultToBool(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetEnvOrDefaultToBool(%q, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}
