package models

// Staff represents a staff member entity
type Staff struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Services   []string `json:"services"` // Service IDs this staff member can provide
	MerchantID string   `json:"merchant_id"`
}

