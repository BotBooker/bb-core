// internal/server/server.go (updated)
package server

import (
	"net/http"
	"time"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/config"
	"bookerbotapi/internal/handlers"
	"bookerbotapi/internal/middleware"
	"bookerbotapi/internal/repository"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Server struct {
	http.Server
	cfg *config.Config
}

func New(cfg *config.Config, bookingRepo repository.AdminRepository, availabilityManager *availability.AvailabilityManager) *Server {
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

	// Initialize handlers with dependencies
	healthHandler := handlers.NewHealthHandler()
	availabilityHandler := handlers.NewAvailabilityHandler(availabilityManager, bookingRepo)
	catalogHandler := handlers.NewCatalogHandler(bookingRepo)
	bookingHandler := handlers.NewBookingHandler(bookingRepo, availabilityManager)
	adminHandler := handlers.NewAdminHandler(bookingRepo)

	// Health check endpoint
	r.Get("/health", healthHandler.HealthCheck)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Apply API key authentication
		r.Use(middleware.APIKeyAuth(cfg.Auth.APIKeys))

		// Availability endpoints
		r.Get("/availability/dates", availabilityHandler.GetAvailableDates)
		r.Get("/availability/slots", availabilityHandler.GetAvailableSlots)

		// Catalog endpoints
		r.Get("/catalog/services", catalogHandler.ListServices)
		r.Get("/catalog/staff", catalogHandler.ListStaff)

		// Booking endpoints
		r.Post("/bookings", bookingHandler.CreateBooking)
		r.Get("/bookings", bookingHandler.ListBookings)
		r.Get("/bookings/{id}", bookingHandler.GetBooking)
		r.Put("/bookings/{id}/cancel", bookingHandler.CancelBooking)

		// Admin endpoints
		r.Route("/admin", func(r chi.Router) {
			// Services
			r.Post("/services", adminHandler.CreateService)
			r.Delete("/services/{id}", adminHandler.DeleteService)
			r.Get("/services/list", adminHandler.ListServicesFiltered)

			// Staff
			r.Post("/staff", adminHandler.CreateStaff)
			r.Delete("/staff/{id}", adminHandler.DeleteStaff)
			r.Get("/staff/list", adminHandler.ListStaffFiltered)

			// Merchant
			r.Post("/merchant", adminHandler.CreateMerchant)
			r.Get("/merchant/{id}", adminHandler.GetMerchant)
			r.Delete("/merchant/{id}", adminHandler.DeleteMerchant)
			r.Get("/merchants/list", adminHandler.ListMerchantsFiltered)
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
