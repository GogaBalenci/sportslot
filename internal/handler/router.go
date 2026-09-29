package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"sportslot/internal/bot"
	"sportslot/internal/service"
)

type Dependencies struct {
	Search        *service.SearchService
	Bookings      *service.BookingService
	Partner       *service.PartnerService
	Bot           *bot.Bot
	Auth          UserAuth
	CORSOrigins   []string
	WebhookSecret string
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(RequestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: deps.CORSOrigins,
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-MAX-User-ID", "X-MAX-User-Name"},
		MaxAge:         300,
	}))
	r.Use(middleware.Timeout(15 * time.Second))

	api := &API{
		search:   deps.Search,
		bookings: deps.Bookings,
		partner:  deps.Partner,
		logins:   newLimiter(10, time.Minute),
		now:      time.Now,
	}
	health := NewHealthHandler()

	r.Get("/health", health.Health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health.Health)
		r.Get("/catalog", api.Catalog)
		r.Post("/quiz/recommend", api.QuizRecommend)
		r.Get("/venues", api.Venues)
		r.Get("/venues/{id}", api.Venue)
		r.Post("/search", api.Search)
		r.Get("/stats", api.Stats)

		r.Group(func(r chi.Router) {
			r.Use(deps.Auth.Require)
			r.Get("/me", api.Me)
			r.Post("/bookings", api.CreateBooking)
			r.Get("/bookings/{id}", api.GetBooking)
			r.Delete("/bookings/{id}", api.CancelBooking)
			r.Post("/bookings/{id}/cancel", api.CancelBooking)
			r.Patch("/bookings/{id}/reschedule", api.Reschedule)
			r.Post("/bookings/{id}/feedback", api.Feedback)
			r.Get("/bookings/user/{user_id}", api.ListByUser)
			r.Post("/waitlist", api.Waitlist)
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
		webhook := &BotWebhook{bot: deps.Bot, secret: deps.WebhookSecret}
		r.Post("/bot/webhook", webhook.Handle)
	}
	return r
}
