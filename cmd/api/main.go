package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	matchingSvc := service.NewMatchingService(venueRepo, slotRepo)
	bookingSvc := service.NewBookingService(bookingRepo, slotRepo, userRepo)
	dialogSvc := service.NewDialogService(matchingSvc, bookingSvc, cfg.MiniAppURL)
	notifierSvc := service.NewNotifierService(bookingRepo, maxClient, cfg.NotifierInterval)

	notifierCtx, cancelNotifier := context.WithCancel(rootCtx)
	defer cancelNotifier()
	go notifierSvc.Run(notifierCtx)

	router := handler.NewRouter(handler.Dependencies{
		Matching:      matchingSvc,
		Booking:       bookingSvc,
		Dialog:        dialogSvc,
		MaxClient:     maxClient,
		CORSOrigins:   cfg.CORSOrigins,
		WebhookSecret: cfg.MaxWebhookSecret,
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

	cancelNotifier()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}
	log.Println("shutdown complete")
}
