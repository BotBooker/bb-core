package models

// Service represents a service entity
type Service struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	DurationMinutes int               `json:"duration_minutes"`
	WorkingHours    map[string][]string `json:"working_hours"` // Day -> array of time slots
	Price           float64           `json:"price,omitempty"`
	MerchantID      string            `json:"merchant_id"`
}