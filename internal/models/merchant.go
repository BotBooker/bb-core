package models

type Merchant struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	WelcomeMessage string `json:"welcome_message"`
}
