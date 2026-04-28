package models

import "time"

type Booking struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	ServiceID       string    `json:"service_id"`
	StaffID         string    `json:"staff_id"`
	StartTime       string    `json:"start_time"` // ISO 8601 format
	DurationMinutes int       `json:"duration_minutes"`
	Price           float64   `json:"price,omitempty"`
	Paid            bool      `json:"paid"`
	Status          string    `json:"status"` // pending, confirmed, cancelled
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// BookingRequest represents a booking request
type BookingRequest struct {
	UserID          string `json:"user_id"`
	ServiceID       string `json:"service_id"`
	StaffID         string `json:"staff_id"`
	StartTime       string `json:"start_time"` // ISO 8601 format
	DurationMinutes int    `json:"duration_minutes"`
}

