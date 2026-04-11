package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port          string
	Namespace     string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	Env           string // "development" | "production"
}

// Load reads configuration from environment variables.
// In production (ENV=production), DATABASE_URL and REDIS_ADDR are required —
// the process exits immediately with a clear message if they are missing.
// In development, sensible localhost defaults are used.
func Load() *Config {
	env := strings.ToLower(os.Getenv("ENV"))
	if env == "" {
		env = "development"
	}

	cfg := &Config{
		Env:           env,
		Port:          envOrDefault("PORT", "8080"),
		Namespace:     envOrDefault("NAMESPACE", "default"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}

	if env == "production" {
		cfg.DatabaseURL = requireEnv("DATABASE_URL")
		cfg.RedisAddr = requireEnv("REDIS_ADDR")
	} else {
		cfg.DatabaseURL = envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ai_paas?sslmode=disable")
		cfg.RedisAddr = envOrDefault("REDIS_ADDR", "localhost:6379")
	}

	return cfg
}

// envOrDefault returns the value of the environment variable or the given default.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// requireEnv returns the value of the environment variable or exits the process
// immediately with a descriptive error message pointing to the missing variable.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "FATAL: required environment variable %q is not set (ENV=production)\n", key)
		os.Exit(1)
	}
	return v
}
