package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvOrDefault_WithEnvSet(t *testing.T) {
	os.Setenv("TEST_ENV_KEY", "test_value")
	defer os.Unsetenv("TEST_ENV_KEY")

	result := GetEnvOrDefault("TEST_ENV_KEY", "default")

	assert.Equal(t, "test_value", result)
}

func TestGetEnvOrDefault_WithEnvUnset(t *testing.T) {
	os.Unsetenv("TEST_ENV_KEY_MISSING")

	result := GetEnvOrDefault("TEST_ENV_KEY_MISSING", "default")

	assert.Equal(t, "default", result)
}

func TestGetEnvOrDefault_EmptyEnvValue(t *testing.T) {
	// Устанавливаем пустое значение
	os.Setenv("TEST_ENV_EMPTY", "")
	defer os.Unsetenv("TEST_ENV_EMPTY")

	result := GetEnvOrDefault("TEST_ENV_EMPTY", "default")

	assert.Equal(t, "default", result)
}

func TestGetEnvOrDefault_DifferentTypes(t *testing.T) {
	testCases := []struct {
		name         string
		key          string
		value        string
		defaultValue string
		expected     string
	}{
		{
			name:         "numeric value",
			key:          "TEST_PORT",
			value:        "8080",
			defaultValue: "3000",
			expected:     "8080",
		},
		{
			name:         "host value",
			key:          "TEST_HOST",
			value:        "0.0.0.0",
			defaultValue: "localhost",
			expected:     "0.0.0.0",
		},
		{
			name:         "empty value uses default",
			key:          "TEST_EMPTY",
			value:        "",
			defaultValue: "fallback",
			expected:     "fallback",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value != "" {
				os.Setenv(tc.key, tc.value)
				defer os.Unsetenv(tc.key)
			} else {
				os.Unsetenv(tc.key)
			}

			result := GetEnvOrDefault(tc.key, tc.defaultValue)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetEnvList_WithEnvSet(t *testing.T) {
	os.Setenv("TEST_LIST_KEY", "a,b,c")
	defer os.Unsetenv("TEST_LIST_KEY")

	result := GetEnvList("TEST_LIST_KEY", nil)

	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestGetEnvList_WithEnvUnset(t *testing.T) {
	os.Unsetenv("TEST_LIST_KEY_MISSING")
	defaultValue := []string{"default"}

	result := GetEnvList("TEST_LIST_KEY_MISSING", defaultValue)

	assert.Equal(t, defaultValue, result)
}

func TestGetEnvList_WithSpaces(t *testing.T) {
	os.Setenv("TEST_LIST_SPACES", " a , b , c ")
	defer os.Unsetenv("TEST_LIST_SPACES")

	result := GetEnvList("TEST_LIST_SPACES", nil)

	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestGetEnvList_WithEmptyValues(t *testing.T) {
	os.Setenv("TEST_LIST_EMPTY", ",,a,,b,,")
	defer os.Unsetenv("TEST_LIST_EMPTY")

	result := GetEnvList("TEST_LIST_EMPTY", nil)

	assert.Equal(t, []string{"a", "b"}, result)
}

func TestGetEnvList_WithSingleValue(t *testing.T) {
	os.Setenv("TEST_LIST_SINGLE", "single")
	defer os.Unsetenv("TEST_LIST_SINGLE")

	result := GetEnvList("TEST_LIST_SINGLE", nil)

	assert.Equal(t, []string{"single"}, result)
}

func TestGetEnvList_WithEmptyEnv(t *testing.T) {
	os.Setenv("TEST_LIST_EMPTY_ENV", "")
	defer os.Unsetenv("TEST_LIST_EMPTY_ENV")
	defaultValue := []string{"default1", "default2"}

	result := GetEnvList("TEST_LIST_EMPTY_ENV", defaultValue)

	assert.Equal(t, defaultValue, result)
}
