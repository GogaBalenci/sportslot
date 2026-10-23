package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"sportslot/internal/bot"
	"sportslot/internal/geocode"
	"sportslot/internal/service"
)

type Dependencies struct {
	Search        *service.SearchService
	Bookings      *service.BookingService
	Partner       *service.PartnerService
	Studio        *service.StudioService
	Commerce      *service.CommerceService
	Geocoder      *geocode.Geocoder
	Bot           *bot.Bot
	Auth          UserAuth
	CORSOrigins   []string
	WebhookSecret string
	// BotFlood — лимиты событий бота от одного человека; пусто — значения по умолчанию.
	BotFlood *BotFlood
}

// BotFlood: Events событий за Window; одинаковые события чаще Repeat отбрасываются.
type BotFlood struct {
	Events         int
	Window, Repeat time.Duration
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(RequestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: deps.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-MAX-User-ID", "X-MAX-User-Name"},
		MaxAge:         300,
	}))
	r.Use(middleware.Timeout(15 * time.Second))

	api := &API{
		search:   deps.Search,
		bookings: deps.Bookings,
		partner:  deps.Partner,
		studio:   deps.Studio,
		commerce: deps.Commerce,
		auth:     deps.Auth,
		geocoder: deps.Geocoder,
		logins:   newLimiter(10, time.Minute),
		lookups:  newLimiter(40, time.Minute),
		writes:   newLimiter(40, time.Minute),
		now:      time.Now,
	}
	var ping func(ctx context.Context) error
	if deps.Bookings != nil {
		ping = deps.Bookings.Store().Ping
	}
	health := NewHealthHandler(ping)

	r.Get("/health", health.Health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"service": "СпортСлот API",
				"docs":    "https://github.com/GogaBalenci/sportslot/blob/main/openapi/openapi.yaml",
				"health":  "/api/v1/health",
				"endpoints": []string{
					"GET /api/v1/catalog", "GET /api/v1/venues", "GET /api/v1/venues/{id}",
					"POST /api/v1/search", "POST /api/v1/quiz/recommend", "GET /api/v1/stats",
				},
			})
		})
		r.Get("/health", health.Health)
		r.Get("/catalog", api.Catalog)
		r.Post("/quiz/recommend", api.QuizRecommend)
		r.Get("/venues", api.Venues)
		r.Get("/venues/{id}", api.Venue)
		r.Get("/geocode", api.Geocode)
		r.Post("/search", api.Search)
		r.Get("/stats", api.Stats)

		r.Group(func(r chi.Router) {
			r.Use(deps.Auth.Require)
			r.Use(api.limitWrites)
			r.Get("/me", api.Me)
			r.Post("/bookings", api.CreateBooking)
			r.Get("/bookings/{id}", api.GetBooking)
			r.Delete("/bookings/{id}", api.CancelBooking)
			r.Post("/bookings/{id}/cancel", api.CancelBooking)
			r.Patch("/bookings/{id}/reschedule", api.Reschedule)
			r.Post("/bookings/{id}/feedback", api.Feedback)
			r.Get("/bookings/user/{user_id}", api.ListByUser)
			r.Post("/waitlist", api.Waitlist)
			r.Post("/me/location", api.SaveLocation)
			r.Get("/messages", api.ClientMessages)
			r.Post("/messages", api.ClientSendMessage)
			r.Post("/studio/invites/accept", api.AcceptInvite)
			r.Post("/bookings/{id}/review", api.Review)
			r.Get("/bookings/{id}/friend", api.Friend)
			r.Post("/bookings/{id}/friend", api.AddFriend)
			r.Post("/friends/claim", api.ClaimFriend)
			r.Post("/bookings/{id}/pay", api.PayBooking)
			r.Get("/payments/{id}", api.Payment)
			r.Get("/me/passes", api.MyPasses)
			r.Post("/passes/{id}/buy", api.BuyPass)
			r.Post("/me/passes/{id}/pay", api.PayPass)
			r.Post("/me/passes/{id}/cancel", api.CancelPassRequest)
		})

		// Уведомления платёжных систем: магазин определяется по студии в адресе.
		r.With(limitIP(newLimiter(300, time.Minute))).Post("/payments/yookassa/{venueID}", api.YooKassaWebhook)
		r.With(limitIP(newLimiter(300, time.Minute))).Post("/payments/tbank/{venueID}", api.TBankWebhook)
		r.Get("/venues/{id}/logo", api.VenueLogo)

		r.Get("/studio/me", api.StudioMe)
		r.Group(func(r chi.Router) {
			r.Use(api.requireStudio)
			r.Use(api.limitWrites)
			r.Post("/studio/checkin", api.StudioCheckIn)
			r.Get("/studio/bookings/{bookingID}", api.StudioBooking)
			r.Post("/studio/bookings/{bookingID}/remind", api.StudioRemind)
			r.Get("/studio/messages/{messageID}", api.StudioMessageRef)
			r.Route("/studio/{venueID}", func(r chi.Router) {
				r.Get("/bookings", api.StudioBookings)
				r.Get("/templates", api.StudioTemplates)
				r.Post("/templates", api.StudioSaveTemplate)
				r.Put("/templates/{templateID}", api.StudioSaveTemplate)
				r.Delete("/templates/{templateID}", api.StudioDeleteTemplate)
				r.Get("/slots", api.StudioSlots)
				r.Post("/slots/{slotID}/close", api.StudioCloseSlot)
				r.Post("/slots/{slotID}/cancel", api.StudioCancelSlot)
				r.Get("/stats", api.StudioStats)
				r.Patch("/profile", api.StudioProfile)
				r.Get("/threads", api.StudioThreads)
				r.Get("/threads/{userID}", api.StudioThread)
				r.Post("/threads/{userID}", api.StudioReply)
				r.Get("/settings", api.StudioSettings)
				r.Patch("/settings", api.StudioSaveSettings)
				r.Post("/settings/check", api.StudioCheckPayments)
				r.Get("/commerce", api.StudioCommerce)
				r.Post("/passes", api.StudioSavePass)
				r.Put("/passes/{passID}", api.StudioSavePass)
				r.Post("/user-passes/{userPassID}/confirm", api.StudioConfirmPass)
				r.Post("/user-passes/{userPassID}/decline", api.StudioDeclinePass)
				r.Get("/reviews", api.StudioReviews)
				r.Put("/logo", api.StudioUploadLogo)
				r.Delete("/logo", api.StudioDeleteLogo)
			})
			r.Get("/platform/studios", api.PlatformStudios)
			r.Get("/platform/catalog", api.PlatformCatalog)
			r.Post("/platform/studios", api.PlatformAddStudio)
			r.Post("/platform/studios/{venueID}/invites", api.PlatformInvite)
			r.Delete("/platform/studios/{venueID}/admins/{maxUserID}", api.PlatformRemoveAdmin)
		})

		r.Post("/partner/login", api.PartnerLogin)
		r.Group(func(r chi.Router) {
			r.Use(api.requirePartner)
			r.Get("/partner/bookings", api.PartnerBookings)
			r.Post("/partner/checkin", api.PartnerCheckIn)
			r.Post("/partner/bookings/{id}/remind", api.PartnerRemind)
		})
	})
	if deps.Bot != nil {
		api.location = deps.Bot
		flood := BotFlood{Events: 5, Window: 3 * time.Second, Repeat: 2 * time.Second}
		if deps.BotFlood != nil {
			flood = *deps.BotFlood
		}
		webhook := &BotWebhook{bot: deps.Bot, secret: deps.WebhookSecret,
			rate: newLimiter(flood.Events, flood.Window), repeats: newLimiter(1, flood.Repeat),
			warns: newLimiter(1, 30*time.Second), answers: newLimiter(10, 10*time.Second)}
		r.Post("/bot/webhook", webhook.Handle)
	}
	return r
}
