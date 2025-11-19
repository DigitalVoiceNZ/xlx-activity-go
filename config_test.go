// Package main provides tests for configuration management.
package main

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected Config
	}{
		{
			name:    "default values",
			envVars: map[string]string{},
			expected: Config{
				SystemName: "299",
				Timezone:   "Pacific/Auckland",
				DBPath:     "./pb_data/data.db",
				SSEAddr:    ":8080",
				LogPath:    "/var/log/syslog",
			},
		},
		{
			name: "custom values from environment",
			envVars: map[string]string{
				"SYSTEM_NAME": "123",
				"TIMEZONE":    "UTC",
				"DB_PATH":     "/tmp/test.db",
				"SSE_ADDR":    ":9090",
				"LOG_PATH":    "/tmp/test.log",
			},
			expected: Config{
				SystemName: "123",
				Timezone:   "UTC",
				DBPath:     "/tmp/test.db",
				SSEAddr:    ":9090",
				LogPath:    "/tmp/test.log",
			},
		},
		{
			name: "partial override",
			envVars: map[string]string{
				"SYSTEM_NAME": "456",
				"SSE_ADDR":    ":3000",
			},
			expected: Config{
				SystemName: "456",
				Timezone:   "Pacific/Auckland",
				DBPath:     "./pb_data/data.db",
				SSEAddr:    ":3000",
				LogPath:    "/var/log/syslog",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables using t.Setenv() for automatic cleanup
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Load configuration
			config := LoadConfig()

			// Verify all fields
			if config.SystemName != tt.expected.SystemName {
				t.Errorf("SystemName = %v, want %v", config.SystemName, tt.expected.SystemName)
			}
			if config.Timezone != tt.expected.Timezone {
				t.Errorf("Timezone = %v, want %v", config.Timezone, tt.expected.Timezone)
			}
			if config.DBPath != tt.expected.DBPath {
				t.Errorf("DBPath = %v, want %v", config.DBPath, tt.expected.DBPath)
			}
			if config.SSEAddr != tt.expected.SSEAddr {
				t.Errorf("SSEAddr = %v, want %v", config.SSEAddr, tt.expected.SSEAddr)
			}
			if config.LogPath != tt.expected.LogPath {
				t.Errorf("LogPath = %v, want %v", config.LogPath, tt.expected.LogPath)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
		{
			name:         "environment variable not set",
			key:          "MISSING_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "empty environment variable",
			key:          "EMPTY_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if specified, t.Setenv handles cleanup
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefault(%q, %q) = %q, want %q", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvOrDefaultInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		expected     int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT",
			defaultValue: 42,
			envValue:     "123",
			expected:     123,
		},
		{
			name:         "invalid integer",
			key:          "TEST_INT_INVALID",
			defaultValue: 42,
			envValue:     "not-a-number",
			expected:     42,
		},
		{
			name:         "missing environment variable",
			key:          "MISSING_INT",
			defaultValue: 42,
			envValue:     "",
			expected:     42,
		},
		{
			name:         "zero value",
			key:          "TEST_ZERO",
			defaultValue: 42,
			envValue:     "0",
			expected:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if specified, t.Setenv handles cleanup
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			result := getEnvOrDefaultInt(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefaultInt(%q, %d) = %d, want %d", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvOrDefaultBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue bool
		envValue     string
		expected     bool
	}{
		{
			name:         "true value",
			key:          "TEST_BOOL_TRUE",
			defaultValue: false,
			envValue:     "true",
			expected:     true,
		},
		{
			name:         "false value",
			key:          "TEST_BOOL_FALSE",
			defaultValue: true,
			envValue:     "false",
			expected:     false,
		},
		{
			name:         "1 as true",
			key:          "TEST_BOOL_ONE",
			defaultValue: false,
			envValue:     "1",
			expected:     true,
		},
		{
			name:         "0 as false",
			key:          "TEST_BOOL_ZERO",
			defaultValue: true,
			envValue:     "0",
			expected:     false,
		},
		{
			name:         "invalid boolean",
			key:          "TEST_BOOL_INVALID",
			defaultValue: true,
			envValue:     "not-a-bool",
			expected:     true,
		},
		{
			name:         "missing environment variable",
			key:          "MISSING_BOOL",
			defaultValue: false,
			envValue:     "",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if specified, t.Setenv handles cleanup
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			result := getEnvOrDefaultBool(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvOrDefaultBool(%q, %t) = %t, want %t", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}
