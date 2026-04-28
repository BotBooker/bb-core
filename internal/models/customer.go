package models

// Customer represents a customer entity
type Customer struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
}