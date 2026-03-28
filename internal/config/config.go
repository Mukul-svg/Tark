package config

import "os"

type Config struct {
	Port          string
	Namespace     string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
}

func Load() *Config {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		// Default to Docker Compose service name or localhost depending on where it runs
		// Assuming running locally with exposed ports for now
		databaseURL = "postgres://postgres:postgres@localhost:5432/ai_paas?sslmode=disable"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	return &Config{
		Port:          port,
		Namespace:     namespace,
		DatabaseURL:   databaseURL,
		RedisAddr:     redisAddr,
		RedisPassword: redisPassword,
	}
}
