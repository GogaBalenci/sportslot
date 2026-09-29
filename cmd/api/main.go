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

	"sportslot/internal/bot"
	"sportslot/internal/config"
	"sportslot/internal/handler"
	"sportslot/internal/maxclient"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
	"sportslot/internal/service"
	"sportslot/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	applied, err := postgres.Migrate(ctx, pool, migrations.FS)
	if err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Printf("migrations applied: %v", applied)

	store := postgres.NewStore(pool)
	catalogFile, err := seed.LoadCatalog(ctx, store, cfg.CatalogPath)
	if err != nil {
		log.Fatalf("catalog: %v", err)
	}
	log.Printf("catalog: %d venues from %s (%s)", len(catalogFile.Venues), catalogFile.Source, catalogFile.ExportedAt.Format("2006-01-02"))
	if cfg.DemoData {
		n, err := seed.SyncDemo(ctx, store, time.Now())
		if err != nil {
			log.Fatalf("demo partners: %v", err)
		}
		log.Printf("demo partners: %d studios, %d upcoming lessons checked", len(seed.DemoVenues), n)
	}

	client := maxclient.NewClient(cfg.MaxBotAPIBaseURL, cfg.MaxBotAPIToken)
	if cfg.MiniAppButton != "open_app" {
		client.UseLinksForMiniApp(cfg.MiniAppURL)
	}
	initMaxBot(ctx, client, cfg)

	search := service.NewSearchService(store, cfg.DemoData, catalogFile.ExportedAt)
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, cfg.PartnerDemoCode)
	presenter := bot.NewPresenter(client)
	bookings.SetPresenter(presenter)
	partner.SetPresenter(presenter)
	chatBot := bot.New(client, store, search, bookings, presenter)

	testUsers := map[string]bool{}
	for _, id := range cfg.TestMaxUserIDs {
		testUsers[id] = true
	}
	router := handler.NewRouter(handler.Dependencies{
		Search:   search,
		Bookings: bookings,
		Partner:  partner,
		Bot:      chatBot,
		Auth: handler.UserAuth{
			BotToken:  cfg.MaxBotAPIToken,
			MaxAge:    cfg.InitDataMaxAge,
			TestUsers: testUsers,
			Dev:       cfg.Dev(),
		},
		CORSOrigins:   cfg.CORSOrigins,
		WebhookSecret: cfg.MaxWebhookSecret,
	})

	notifier := service.NewNotifier(store, presenter, cfg.NotifierInterval, cfg.DemoData)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		notifier.Run(ctx)
	}()

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("sportslot api listening on :%s (env=%s, demo=%v)", cfg.HTTPPort, cfg.AppEnv, cfg.DemoData)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	wg.Wait()
	log.Println("shutdown complete")
}

// initMaxBot проверяет токен, задаёт команды и подписывает webhook.
// Ошибки не фатальны: API и мини-приложение работают и без связи с MAX.
func initMaxBot(ctx context.Context, client *maxclient.Client, cfg *config.Config) {
	if !client.Enabled() {
		log.Println("MAX_BOT_API_TOKEN is empty: bot is disabled, API works in local mode")
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if info, err := client.Bot(reqCtx); err != nil {
		log.Printf("MAX GET /me failed: %v", err)
	} else {
		log.Printf("MAX bot @%s (id %d): https://max.ru/%s", info.Username, info.UserID, info.Username)
	}
	commands := []maxclient.Command{
		{Name: "start", Description: "Подобрать тренировку"},
		{Name: "my", Description: "Мои записи"},
		{Name: "help", Description: "Как это работает"},
	}
	if err := client.SetCommands(reqCtx, commands); err != nil {
		log.Printf("MAX set commands failed: %v", err)
	}
	if cfg.MaxWebhookURL == "" {
		log.Println("MAX_WEBHOOK_URL is empty: webhook subscription skipped")
		return
	}
	types := []string{"message_created", "message_callback", "bot_started"}
	if err := client.Subscribe(reqCtx, cfg.MaxWebhookURL, cfg.MaxWebhookSecret, types); err != nil {
		log.Printf("MAX webhook subscribe failed: %v", err)
		return
	}
	log.Printf("MAX webhook subscribed: %s", cfg.MaxWebhookURL)
}
