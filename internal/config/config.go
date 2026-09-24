package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort         string
	DatabaseURL      string
	MaxBotAPIBaseURL string
	MaxBotAPIToken   string
	MaxWebhookSecret string
	CORSOrigins      []string
	MiniAppURL       string
	NotifierInterval time.Duration
	SeedDataPath     string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		MaxBotAPIBaseURL: getEnv("MAX_BOT_API_BASE_URL", "https://botapi.max.ru"),
		MaxBotAPIToken:   os.Getenv("MAX_BOT_API_TOKEN"),
		MaxWebhookSecret: os.Getenv("MAX_WEBHOOK_SECRET"),
		CORSOrigins:      splitList(getEnv("CORS_ORIGINS", "http://localhost:5173")),
		MiniAppURL:       os.Getenv("MINI_APP_URL"),
		SeedDataPath:     getEnv("SEED_DATA_PATH", "seed-data/venues.json"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.MaxWebhookSecret == "" {
		return nil, fmt.Errorf("MAX_WEBHOOK_SECRET is required")
	}

	intervalSec := getEnvInt("NOTIFIER_INTERVAL_SECONDS", 120)
	cfg.NotifierInterval = time.Duration(intervalSec) * time.Second

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
