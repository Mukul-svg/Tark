package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	Namespace        string
	DatabaseURL      string
	RedisAddr        string
	RedisPassword    string
	Env              string // "development" | "production"
	KubeconfigKey    []byte // AES-256 key for kubeconfig encryption at rest
}

// Load reads configuration from environment variables.
// It first loads a .env file from the working directory if one exists
// (silently ignored if missing, so production is unaffected).
// In production (ENV=production), DATABASE_URL and REDIS_ADDR are required —
// the process exits immediately with a clear message if they are missing.
// In development, sensible localhost defaults are used.
func Load() *Config {
	_ = godotenv.Load() // no-op if .env is absent
	env := strings.ToLower(os.Getenv("ENV"))
	if env == "" {
		env = "development"
	}

	cfg := &Config{
		Env:           env,
		Port:          envOrDefault("PORT", "8080"),
		Namespace:     envOrDefault("NAMESPACE", "default"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		KubeconfigKey: loadKubeconfigKey(env),
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

// loadKubeconfigKey reads KUBECONFIG_ENCRYPTION_KEY (64 hex chars = 32 bytes AES-256).
// In production, a missing or malformed key is fatal. In development, it emits a warning
// and returns nil (encryption disabled — kubeconfigs stored as plaintext).
func loadKubeconfigKey(env string) []byte {
	raw := os.Getenv("KUBECONFIG_ENCRYPTION_KEY")
	if raw == "" {
		if env == "production" {
			fmt.Fprintf(os.Stderr, "FATAL: KUBECONFIG_ENCRYPTION_KEY is required in production\n")
			os.Exit(1)
		}
		slog.Warn("KUBECONFIG_ENCRYPTION_KEY is not set — kubeconfigs will be stored as plaintext (development mode)")
		return nil
	}

	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		fmt.Fprintf(os.Stderr, "FATAL: KUBECONFIG_ENCRYPTION_KEY must be exactly 64 hex characters (32 bytes), got %d bytes\n", len(key))
		os.Exit(1)
	}
	return key
}
