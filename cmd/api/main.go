package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // часовой пояс из TZ (Europe/Moscow) без tzdata в образе

	"sportslot/internal/config"
	"sportslot/internal/handler"
	"sportslot/internal/maxclient"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
	"sportslot/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config load error: %v", err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(rootCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connection error: %v", err)
	}
	defer pool.Close()

	migrationBytes, err := os.ReadFile("migrations/001_init_schema.up.sql")
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}
	migrationSQL := string(migrationBytes)
	if _, err := pool.Exec(rootCtx, migrationSQL); err != nil {
		log.Fatalf("apply migration: %v", err)
	}
	log.Println("database migration complete")

	venueRepo := postgres.NewVenueRepo(pool)
	slotRepo := postgres.NewSlotRepo(pool)
	bookingRepo := postgres.NewBookingRepo(pool)
	userRepo := postgres.NewUserRepo(pool)

	seedLoader := seed.NewLoader(venueRepo, slotRepo, userRepo)
	if err := seedLoader.LoadIfEmpty(rootCtx, cfg.SeedDataPath); err != nil {
		log.Fatalf("seed data load error: %v", err)
	}
	log.Println("seed data check complete")

	maxClient := maxclient.NewClient(cfg.MaxBotAPIBaseURL, cfg.MaxBotAPIToken)
	initMaxBot(rootCtx, maxClient, cfg)
	matchingSvc := service.NewMatchingService(venueRepo, slotRepo)
	bookingSvc := service.NewBookingService(bookingRepo, slotRepo, userRepo)
	dialogSvc := service.NewDialogService(matchingSvc, bookingSvc, cfg.MiniAppURL)
	notifierSvc := service.NewNotifierService(bookingRepo, maxClient, cfg.NotifierInterval)

	var notifierWG sync.WaitGroup
	notifierCtx, cancelNotifier := context.WithCancel(rootCtx)
	defer cancelNotifier()

	notifierWG.Add(1)
	go func() {
		defer notifierWG.Done()
		notifierSvc.Run(notifierCtx)
	}()

	router := handler.NewRouter(handler.Dependencies{
		Matching:      matchingSvc,
		Booking:       bookingSvc,
		Dialog:        dialogSvc,
		MaxClient:     maxClient,
		CORSOrigins:   cfg.CORSOrigins,
		WebhookSecret: cfg.MaxWebhookSecret,
		MiniAppButton: cfg.MiniAppButton,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("sportslot api listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-rootCtx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}

	cancelNotifier()
	notifierWG.Wait()
	log.Println("shutdown complete")
}

// initMaxBot проверяет токен (GET /me) и регистрирует webhook, если задан
// MAX_WEBHOOK_URL. Ошибки не фатальны: REST API и мини-приложение работают
// и без связи с MAX, а подписку можно повторить скриптом deploy/max-webhook.sh.
func initMaxBot(ctx context.Context, client *maxclient.Client, cfg *config.Config) {
	if !client.Enabled() {
		log.Println("MAX_BOT_API_TOKEN is empty: bot messages are disabled (local demo mode)")
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	info, err := client.Bot(reqCtx)
	if err != nil {
		log.Printf("MAX bot check failed (GET /me): %v", err)
	} else {
		log.Printf("MAX bot: @%s (id %d), link: https://max.ru/%s", info.Username, info.UserID, info.Username)
	}

	if err := client.SetCommands(reqCtx, map[string]string{"start": "Подобрать тренировку"}); err != nil {
		log.Printf("MAX set commands failed: %v", err)
	}

	if cfg.MaxWebhookURL == "" {
		log.Println("MAX_WEBHOOK_URL is empty: webhook subscription skipped")
		return
	}
	updateTypes := []string{"message_created", "message_callback", "bot_started"}
	if err := client.Subscribe(reqCtx, cfg.MaxWebhookURL, cfg.MaxWebhookSecret, updateTypes); err != nil {
		log.Printf("MAX webhook subscribe failed: %v", err)
		return
	}
	log.Printf("MAX webhook subscribed: %s", cfg.MaxWebhookURL)
}
