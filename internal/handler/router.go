package handler

import (
	"net/http"
	"time"

	"sportslot/internal/maxclient"
	"sportslot/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Dependencies struct {
	Matching      *service.MatchingService
	Booking       *service.BookingService
	Dialog        *service.DialogService
	MaxClient     *maxclient.Client
	CORSOrigins   []string
	WebhookSecret string
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(RequestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "X-MAX-User-ID", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.Timeout(15 * time.Second))

	healthH := NewHealthHandler()
	searchH := NewSearchHandler(deps.Matching)
	bookingH := NewBookingHandler(deps.Booking)
	botH := NewBotWebhookHandler(deps.Dialog, deps.MaxClient, deps.WebhookSecret)

	r.Get("/api/v1/health", healthH.Health)

	r.Route("/api/v1", func(r chi.Router) {
		r.With(OptionalMaxUserID).Post("/search", searchH.Search)
		r.Get("/venues/{id}", searchH.GetVenue)

		r.Group(func(r chi.Router) {
			r.Use(RequireMaxUserID)
			r.Post("/bookings", bookingH.Create)
			r.Delete("/bookings/{id}", bookingH.Cancel)
			r.Patch("/bookings/{id}/reschedule", bookingH.Reschedule)
			r.Get("/bookings/user/{user_id}", bookingH.ListByUser)
		})
	})

	r.Post("/bot/webhook", botH.HandleWebhook)

	return r
}
