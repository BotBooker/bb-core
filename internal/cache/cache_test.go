package cache

import (
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		wantNotNil bool
	}{
		{
			name:       "valid address",
			addr:       "localhost:6379",
			wantNotNil: true,
		},
		{
			name:       "empty address",
			addr:       "",
			wantNotNil: true,
		},
		{
			name:       "IPv6 address",
			addr:       "[::1]:6379",
			wantNotNil: true,
		},
		{
			name:       "address with password",
			addr:       "redis://user:pass@localhost:6379/0",
			wantNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.addr)
			if tt.wantNotNil && got == nil {
				t.Error("New() returned nil, expected non-nil Cache")
			}
		})
	}
}

func TestRedisCache_Get(t *testing.T) {
	c := New("localhost:6379")

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "existing key",
			key:     "mykey",
			wantErr: false,
		},
		{
			name:    "non-existing key",
			key:     "nonexistent",
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: false,
		},
		{
			name:    "special characters in key",
			key:     "key:with:colons",
			wantErr: false,
		},
		{
			name:    "very long key",
			key:     "a_very_long_key_that_exceeds_normal_length_limits_for_redis_keys_to_test_boundary_conditions",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Mock implementation returns the key as value
			if got != tt.key && !tt.wantErr {
				t.Errorf("Get() = %v, want %v", got, tt.key)
			}
		})
	}
}

func TestRedisCache_Set(t *testing.T) {
	c := New("localhost:6379")

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{
			name:    "normal key-value pair",
			key:     "mykey",
			value:   "myvalue",
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			value:   "value",
			wantErr: false,
		},
		{
			name:    "empty value",
			key:     "key",
			value:   "",
			wantErr: false,
		},
		{
			name:    "both empty",
			key:     "",
			value:   "",
			wantErr: false,
		},
		{
			name:    "value with special characters",
			key:     "key",
			value:   "value with spaces and !@#$%^&*()",
			wantErr: false,
		},
		{
			name:    "very long value",
			key:     "key",
			value:   "a_very_long_value_that_exceeds_normal_length_limits_for_redis_values_to_test_boundary_conditions_and_ensure_the_implementation_handles_it_correctly",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Set(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedisCache_Close(t *testing.T) {
	c := New("localhost:6379")

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "close after creation",
			wantErr: false,
		},
		{
			name:    "close twice",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Close()
			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Second close
			if tt.name == "close twice" {
				err = c.Close()
				if (err != nil) != tt.wantErr {
					t.Errorf("Close() second call error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}
