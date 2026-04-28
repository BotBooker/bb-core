package models

type WorkingHours struct {
	Monday    []string `json:"monday,omitempty"`
	Tuesday   []string `json:"tuesday,omitempty"`
	Wednesday []string `json:"wednesday,omitempty"`
	Thursday  []string `json:"thursday,omitempty"`
	Friday    []string `json:"friday,omitempty"`
	Saturday  []string `json:"saturday,omitempty"`
	Sunday    []string `json:"sunday,omitempty"`
}

type Service struct {
	ID              string       `json:"id"`
	MerchantID      string       `json:"merchant_id"`
	Name            string       `json:"name"`
	DurationMinutes int          `json:"duration_minutes"`
	TimeGranularity int          `json:"time_granularity"` // in minutes (10 minimum, max 1440, steps of 5)
	Price           *float64     `json:"price,omitempty"`
	WorkingHours    WorkingHours `json:"working_hours,omitempty"`
}

