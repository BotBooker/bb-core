package models

// Booking represents a booking entity
type Booking struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	ServiceID         string  `json:"service_id"`
	StaffID           string  `json:"staff_id"`
	StartTime         string  `json:"start_time"` // ISO 8601 format
	DurationMinutes   int     `json:"duration_minutes"`
	Status            string  `json:"status"`     // pending, confirmed, cancelled
	Price             float64 `json:"price,omitempty"`
	Paid              bool    `json:"paid,omitempty"`
}

// BookingRequest represents a booking request
type BookingRequest struct {
	UserID          string `json:"user_id"`
	ServiceID       string `json:"service_id"`
	StaffID         string `json:"staff_id"`
	StartTime       string `json:"start_time"` // ISO 8601 format
	DurationMinutes int    `json:"duration_minutes"`
}