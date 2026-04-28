// internal/server/server.go
package server

import (
	"net/http"
	"time"

	"bookerbotapi/internal/config"
	"bookerbotapi/internal/handlers"
	"bookerbotapi/internal/middleware"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	http.Server
	cfg *config.Config
}

func New(cfg *config.Config) *Server {
	r := chi.NewRouter()

	// Setup global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// CORS middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-API-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check endpoint
	r.Get("/health", handlers.HealthCheck)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Apply API key authentication
		r.Use(middleware.APIKeyAuth(cfg.Auth.APIKeys))

		// Availability endpoints
		r.Get("/availability/dates", handlers.GetAvailableDates)
		r.Get("/availability/slots", handlers.GetAvailableSlots)

		// Catalog endpoints
		r.Get("/catalog/services", handlers.ListServices)
		r.Get("/catalog/staff", handlers.ListStaff)

		// Booking endpoints
		r.Post("/bookings", handlers.CreateBooking)
		r.Get("/bookings", handlers.ListBookings)
		r.Get("/bookings/{id}", handlers.GetBooking)
		r.Put("/bookings/{id}/cancel", handlers.CancelBooking)

		// Admin endpoints
		r.Route("/admin", func(r chi.Router) {
			// Services
			r.Post("/services", handlers.CreateService)
			r.Delete("/services/{id}", handlers.DeleteService)
			r.Get("/services/list", handlers.ListServicesFiltered)

			// Staff
			r.Post("/staff", handlers.CreateStaff)
			r.Delete("/staff/{id}", handlers.DeleteStaff)
			r.Get("/staff/list", handlers.ListStaffFiltered)

			// Merchant
			r.Post("/merchant", handlers.CreateMerchant)
			r.Get("/merchant/{id}", handlers.GetMerchant)
			r.Delete("/merchant/{id}", handlers.DeleteMerchant)
			r.Get("/merchants/list", handlers.ListMerchantsFiltered)
		})
	})

	return &Server{
		Server: http.Server{
			Addr:         ":" + cfg.Server.Port,
			Handler:      r,
			ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
			IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
		},
		cfg: cfg,
	}
}
