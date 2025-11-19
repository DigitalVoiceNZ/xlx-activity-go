// Package main provides configuration management for the activity monitor.
package main

import (
	"os"
	"strconv"
)

// Config holds all configuration values for the application.
type Config struct {
	SystemName string // System identifier (e.g., "299")
	Timezone   string // Timezone location (e.g., "Pacific/Auckland")
	DBPath     string // Path to SQLite database file
	SSEAddr    string // Address and port for SSE server (e.g., ":8080")
	LogPath    string // Path to system log file to tail
}

// LoadConfig loads configuration from environment variables with sensible defaults.
//
// Environment variables recognized:
//
//	SYSTEM_NAME, TIMEZONE, DB_PATH, SSE_ADDR, LOG_PATH
func LoadConfig() *Config {
	return &Config{
		SystemName: getEnvOrDefault("SYSTEM_NAME", "299"),
		Timezone:   getEnvOrDefault("TIMEZONE", "Pacific/Auckland"),
		DBPath:     getEnvOrDefault("DB_PATH", "./pb_data/data.db"),
		SSEAddr:    getEnvOrDefault("SSE_ADDR", ":8080"),
		LogPath:    getEnvOrDefault("LOG_PATH", "/var/log/syslog"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvOrDefaultBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
