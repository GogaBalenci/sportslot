package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort    string
	DatabaseURL string
	// AppEnv: "prod" (по умолчанию) или "dev". В dev мини-приложение можно
	// открыть в обычном браузере без подписи MAX — только для локальной отладки.
	AppEnv string

	MaxBotAPIBaseURL string
	MaxBotAPIToken   string
	MaxWebhookSecret string
	MaxWebhookURL    string
	// MiniAppButton: "open_app" — мини-приложение открывается внутри MAX
	// (нужна привязка к боту в кабинете), "link" — обычной ссылкой.
	MiniAppButton string
	MiniAppURL    string
	CORSOrigins   []string

	// DGISAPIKey — ключ 2ГИС для матрицы времени в пути; пусто — время оценивается.
	DGISAPIKey  string
	DGISBaseURL string
	// DGISCatalogURL — геокодер 2ГИС (тот же ключ).
	DGISCatalogURL string

	CatalogPath     string
	DemoData        bool
	PartnerDemoCode string
	TestMaxUserIDs  []string
	// PlatformAdminIDs — id пользователей MAX, которые подключают студии и выдают приглашения.
	PlatformAdminIDs []string
	InitDataMaxAge   time.Duration
	NotifierInterval time.Duration

	// PaymentSecretKey — ключ AES-256 (64 hex-символа) для ключей магазинов студий.
	PaymentSecretKey string
	YooKassaAPIURL   string
	TBankAPIURL      string
	// PublicURL — адрес, на который платёжные системы шлют уведомления (https://домен).
	PublicURL string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		AppEnv:           getEnv("APP_ENV", "prod"),
		MaxBotAPIBaseURL: getEnv("MAX_BOT_API_BASE_URL", "https://platform-api2.max.ru"),
		MaxBotAPIToken:   os.Getenv("MAX_BOT_API_TOKEN"),
		MaxWebhookSecret: os.Getenv("MAX_WEBHOOK_SECRET"),
		MaxWebhookURL:    os.Getenv("MAX_WEBHOOK_URL"),
		MiniAppButton:    getEnv("MINI_APP_BUTTON", "link"),
		MiniAppURL:       getEnv("MINI_APP_URL", "http://localhost:5173"),
		CORSOrigins:      splitList(getEnv("CORS_ORIGINS", "http://localhost:5173")),
		DGISAPIKey:       os.Getenv("DGIS_API_KEY"),
		DGISBaseURL:      getEnv("DGIS_ROUTING_URL", "https://routing.api.2gis.com"),
		DGISCatalogURL:   getEnv("DGIS_CATALOG_URL", "https://catalog.api.2gis.com"),
		CatalogPath:      getEnv("CATALOG_PATH", "seed-data/rostov_catalog.json"),
		DemoData:         getEnv("DEMO_DATA", "on") != "off",
		PartnerDemoCode:  os.Getenv("PARTNER_DEMO_CODE"),
		TestMaxUserIDs:   splitList(getEnv("TEST_MAX_USER_IDS", "max-test-user-001")),
		PlatformAdminIDs: splitList(os.Getenv("PLATFORM_ADMIN_IDS")),
		PaymentSecretKey: os.Getenv("PAYMENT_SECRET_KEY"),
		YooKassaAPIURL:   getEnv("YOOKASSA_API_URL", "https://api.yookassa.ru"),
		TBankAPIURL:      getEnv("TBANK_API_URL", "https://securepay.tinkoff.ru"),
	}
	cfg.PublicURL = strings.TrimRight(getEnv("PUBLIC_URL", cfg.MiniAppURL), "/")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AppEnv != "dev" && cfg.AppEnv != "prod" {
		return nil, fmt.Errorf("APP_ENV must be dev or prod")
	}
	var err error
	if cfg.NotifierInterval, err = seconds("NOTIFIER_INTERVAL_SECONDS", 60); err != nil {
		return nil, err
	}
	if cfg.InitDataMaxAge, err = seconds("INIT_DATA_MAX_AGE_SECONDS", 3600); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Dev() bool { return c.AppEnv == "dev" }

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func seconds(key string, fallback int) (time.Duration, error) {
	n, err := strconv.Atoi(getEnv(key, strconv.Itoa(fallback)))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return time.Duration(n) * time.Second, nil
}

func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
