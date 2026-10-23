package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"sportslot/internal/bot"
	"sportslot/internal/config"
	"sportslot/internal/geocode"
	"sportslot/internal/handler"
	"sportslot/internal/maxclient"
	"sportslot/internal/model"
	"sportslot/internal/payments"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/routing"
	"sportslot/internal/secret"
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
		log.Printf("demo partners: %d studios, %d new lessons", len(seed.DemoVenues), n)
	} else if _, err := seed.ExpandSchedules(ctx, store, time.Now(), ""); err != nil {
		log.Fatalf("schedules: %v", err)
	}

	client := maxclient.NewClient(cfg.MaxBotAPIBaseURL, cfg.MaxBotAPIToken)
	if cfg.MiniAppButton != "open_app" {
		client.UseLinksForMiniApp(cfg.MiniAppURL)
	}
	initMaxBot(ctx, client, cfg)

	search := service.NewSearchService(store, cfg.DemoData, catalogFile.ExportedAt)
	if cfg.DGISAPIKey != "" {
		search.SetRouter(routing.NewDGIS(cfg.DGISBaseURL, cfg.DGISAPIKey))
		log.Println("travel time: 2GIS distance matrix")
	} else {
		log.Println("DGIS_API_KEY is empty: travel time is estimated from distance")
	}
	bookings := service.NewBookingService(store)
	partner := service.NewPartnerService(store, cfg.PartnerDemoCode)
	presenter := bot.NewPresenter(client)
	bookings.SetPresenter(presenter)
	partner.SetPresenter(presenter)
	chatBot := bot.New(client, store, search, bookings, presenter)
	geocoder := geocode.New(cfg.DGISCatalogURL, cfg.DGISAPIKey)
	chatBot.SetGeocoder(geocoder)

	link := deepLinks(ctx, client, cfg)
	studio := service.NewStudioService(store, partner, cfg.PlatformAdminIDs)
	studio.SetPresenter(presenter)
	studio.SetGeocoder(geocoder)
	studio.SetInviteURL(func(token string) string { return link("studio_" + token) })
	bookings.SetStudioNotifier(studio)
	bookings.SetClientPresenter(presenter)
	bookings.SetDeepLink(link)
	chatBot.SetStudio(studio)
	log.Printf("platform admins: %d", len(cfg.PlatformAdminIDs))

	box, err := secret.New(cfg.PaymentSecretKey)
	if err != nil {
		log.Fatalf("payments: %v", err)
	}
	if !box.Ready() {
		log.Println("PAYMENT_SECRET_KEY is empty: studios cannot save payment keys")
	}
	commerce := service.NewCommerceService(store, payments.NewGateway(cfg.YooKassaAPIURL, cfg.TBankAPIURL), box, studio)
	commerce.SetClientPresenter(presenter)
	commerce.SetDeepLink(link)
	commerce.SetPublicURL(cfg.PublicURL)
	chatBot.SetCommerce(commerce)
	presenter.SetPayable(func(ctx context.Context, v *model.BookingView) bool {
		if v.Venue.TrialPrice == nil || *v.Venue.TrialPrice <= 0 {
			return false
		}
		st, err := store.GetSettings(ctx, v.Venue.ID)
		return err == nil && st.PaymentsReady() && st.TrialOnline
	})
	started := time.Now()
	chatBot.SetStatus(func(ctx context.Context) string { return statusReport(ctx, store, started) })

	testUsers := map[string]bool{}
	for _, id := range cfg.TestMaxUserIDs {
		testUsers[id] = true
	}
	router := handler.NewRouter(handler.Dependencies{
		Search:   search,
		Bookings: bookings,
		Partner:  partner,
		Studio:   studio,
		Commerce: commerce,
		Geocoder: geocoder,
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
	notifier.AddJob(commerce.Tick)
	notifier.AddJob(bookings.ReleaseUnclaimedFriends)
	notifier.AddJob(studio.StudioTick)
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

// deepLinks — ссылка, которая открывает мини-приложение бота с параметром
// запуска (приглашение студии, место для друга, возврат после оплаты).
// Имя бота запрашивается у MAX один раз и запоминается.
func deepLinks(ctx context.Context, client *maxclient.Client, cfg *config.Config) func(string) string {
	var (
		mu       sync.Mutex
		username string
	)
	return func(param string) string {
		mu.Lock()
		defer mu.Unlock()
		if username == "" && client.Enabled() {
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if info, err := client.Bot(reqCtx); err == nil {
				username = info.Username
			}
			cancel()
		}
		if username != "" {
			return "https://max.ru/" + username + "?startapp=" + param
		}
		return strings.TrimRight(cfg.MiniAppURL, "/") + "/?start=" + param
	}
}

// statusReport — ответ на /status для владельцев платформы.
func statusReport(ctx context.Context, store *postgres.Store, started time.Time) string {
	var b strings.Builder
	up := int(time.Since(started).Minutes())
	fmt.Fprintf(&b, "🟢 <b>API работает</b> %d ч %d мин\n", up/60, up%60)
	if err := store.Ping(ctx); err != nil {
		b.WriteString("🔴 База недоступна\n")
	} else {
		b.WriteString("🟢 База отвечает\n")
	}
	ops, err := store.OpsStatus(ctx)
	if err != nil {
		return b.String()
	}
	if v, ok := ops["backup_ok"]; ok {
		fmt.Fprintf(&b, "💾 Последний бэкап: %s, %s\n", v.UpdatedAt.In(time.Local).Format("02.01 15:04"), v.Value)
	} else {
		b.WriteString("💾 Бэкапов ещё не было\n")
	}
	if v, ok := ops["backup_error"]; ok && v.UpdatedAt.After(ops["backup_ok"].UpdatedAt) {
		fmt.Fprintf(&b, "⚠️ Ошибка бэкапа %s: %s\n", v.UpdatedAt.In(time.Local).Format("02.01 15:04"), v.Value)
	}
	if v, ok := ops["watchdog"]; ok {
		fmt.Fprintf(&b, "👀 Мониторинг: проверка %d с назад (%s)\n", int(time.Since(v.UpdatedAt).Seconds()), v.Value)
	} else {
		b.WriteString("👀 Мониторинг не запущен\n")
	}
	return b.String()
}
