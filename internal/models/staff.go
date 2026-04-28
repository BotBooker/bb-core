package models

// Staff represents a staff member entity
type Staff struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Services     []string          `json:"services"`    // Service IDs this staff member can provide
	WorkingHours map[string][]string `json:"working_hours"` // Day -> array of time slots
	MerchantID   string            `json:"merchant_id"`
}