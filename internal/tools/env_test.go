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
