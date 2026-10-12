package model

import (
	"errors"
	"testing"
	"time"
)

func TestNewDate(t *testing.T) {
	tests := []struct {
		name   string
		moment time.Time
		want   Date
	}{
		{
			name:   "UTC",
			moment: time.Date(2000, time.January, 2, 15, 4, 5, 0, time.UTC),
			want:   Date{Year: 2000, Month: time.January, Day: 2},
		},
		{
			name:   "время суток отбрасывается",
			moment: time.Date(2000, time.January, 2, 23, 59, 59, 999, time.UTC),
			want:   Date{Year: 2000, Month: time.January, Day: 2},
		},
		{
			name:   "пояс переводится в UTC",
			moment: time.Date(2000, time.January, 2, 0, 30, 0, 0, time.FixedZone("MSK", 3*60*60)),
			want:   Date{Year: 2000, Month: time.January, Day: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDate(tt.moment); got != tt.want {
				t.Errorf("NewDate(%v) = %v, want %v", tt.moment, got, tt.want)
			}
		})
	}
}

func TestDate_ParseDate(t *testing.T) {
	t.Run("разбирает YYYY-MM-DD", func(t *testing.T) {
		date, err := ParseDate("1990-05-01")
		if err != nil {
			t.Fatalf("ParseDate() unexpected error = %v", err)
		}

		want := Date{Year: 1990, Month: time.May, Day: 1}
		if date != want {
			t.Errorf("ParseDate() = %v, want %v", date, want)
		}
		if date.String() != "1990-05-01" {
			t.Errorf("String() = %q, want %q", date.String(), "1990-05-01")
		}
	})

	t.Run("неверный формат", func(t *testing.T) {
		for _, value := range []string{"", "01.05.1990", "1990-5-1", "1990-05-01T00:00:00Z", "not-a-date"} {
			if _, err := ParseDate(value); !errors.Is(err, ErrInvalidDate) {
				t.Errorf("ParseDate(%q) error = %v, want %v", value, err, ErrInvalidDate)
			}
		}
	})

	t.Run("несуществующая дата", func(t *testing.T) {
		if _, err := ParseDate("2000-02-31"); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(2000-02-31) error = %v, want %v", err, ErrInvalidDate)
		}
	})
}

func TestDate_String(t *testing.T) {
	t.Run("форматирует в YYYY-MM-DD", func(t *testing.T) {
		date := Date{Year: 2000, Month: time.January, Day: 2}

		if got := date.String(); got != "2000-01-02" {
			t.Errorf("String() = %q, want %q", got, "2000-01-02")
		}
	})

	t.Run("нулевая дата печатается пустой строкой", func(t *testing.T) {
		if got := (Date{}).String(); got != "" {
			t.Errorf("String() = %q, want пустую строку", got)
		}
	})

	t.Run("несуществующую дату печатает как есть", func(t *testing.T) {
		// Нормализация time.Date превратила бы 31 февраля в 3 марта, и клиент
		// увидел бы в ошибке не ту дату, которую отправил.
		date := Date{Year: 2000, Month: time.February, Day: 31}

		if got := date.String(); got != "2000-02-31" {
			t.Errorf("String() = %q, want %q", got, "2000-02-31")
		}
	})

	t.Run("частичную дату печатает как есть", func(t *testing.T) {
		date := Date{Year: 2000}

		if got := date.String(); got != "2000-00-00" {
			t.Errorf("String() = %q, want %q", got, "2000-00-00")
		}
	})
}

func TestDate_Time(t *testing.T) {
	date := Date{Year: 2000, Month: time.January, Day: 2}

	got := date.Time()

	if !got.Equal(time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Time() = %v, want полночь UTC 2000-01-02", got)
	}
	if got.Location() != time.UTC {
		t.Errorf("Time() location = %v, want UTC", got.Location())
	}
}

func TestDate_IsZero(t *testing.T) {
	tests := []struct {
		name string
		date Date
		want bool
	}{
		{name: "нулевая дата", date: Date{}, want: true},
		{name: "заданная дата", date: Date{Year: 2000, Month: time.January, Day: 2}, want: false},
		{name: "только год", date: Date{Year: 2000}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDate_Valid(t *testing.T) {
	tests := []struct {
		name string
		date Date
		want bool
	}{
		{name: "обычная дата", date: Date{Year: 2000, Month: time.January, Day: 2}, want: true},
		{name: "29 февраля високосного года", date: Date{Year: 2000, Month: time.February, Day: 29}, want: true},
		{name: "29 февраля невисокосного года", date: Date{Year: 2001, Month: time.February, Day: 29}, want: false},
		{name: "31 февраля", date: Date{Year: 2000, Month: time.February, Day: 31}, want: false},
		{name: "31 апреля", date: Date{Year: 2000, Month: time.April, Day: 31}, want: false},
		{name: "13-й месяц", date: Date{Year: 2000, Month: time.Month(13), Day: 1}, want: false},
		{name: "нулевой день", date: Date{Year: 2000, Month: time.January}, want: false},
		{name: "нулевая дата", date: Date{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.date.Valid(); got != tt.want {
				t.Errorf("Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDate_Compare(t *testing.T) {
	base := Date{Year: 2000, Month: time.January, Day: 2}

	tests := []struct {
		name  string
		other Date
		want  int
	}{
		{name: "та же дата", other: Date{Year: 2000, Month: time.January, Day: 2}, want: 0},
		{name: "позже по году", other: Date{Year: 1999, Month: time.December, Day: 31}, want: 1},
		{name: "раньше по году", other: Date{Year: 2001, Month: time.January, Day: 1}, want: -1},
		{name: "раньше по месяцу и дню", other: Date{Year: 2000, Month: time.February, Day: 1}, want: -1},
		{name: "позже по месяцу", other: Date{Year: 1999, Month: time.December, Day: 1}, want: 1},
		{name: "раньше по дню", other: Date{Year: 2000, Month: time.January, Day: 3}, want: -1},
		{name: "позже по дню", other: Date{Year: 2000, Month: time.January, Day: 1}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.Compare(tt.other); got != tt.want {
				t.Errorf("Compare(%v) = %d, want %d", tt.other, got, tt.want)
			}
		})
	}
}

func TestDate_After(t *testing.T) {
	base := Date{Year: 2000, Month: time.January, Day: 2}

	tests := []struct {
		name  string
		other Date
		want  bool
	}{
		{name: "позже", other: Date{Year: 1999, Month: time.December, Day: 31}, want: true},
		{name: "та же дата", other: Date{Year: 2000, Month: time.January, Day: 2}, want: false},
		{name: "раньше", other: Date{Year: 2000, Month: time.January, Day: 3}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.After(tt.other); got != tt.want {
				t.Errorf("After(%v) = %v, want %v", tt.other, got, tt.want)
			}
		})
	}
}
