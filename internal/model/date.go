package model

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidDate возвращается, если строку нельзя разобрать как календарную дату.
var ErrInvalidDate = errors.New("invalid date")

// Date — календарная дата без времени суток и без часового пояса.
//
// Для даты рождения время смысла не имеет: «2000-01-02» — один и тот же день
// в любом поясе, а «02.01.2000 03:00 MSK» ничего не добавляет. Поэтому модель
// хранит только год, месяц и день; в БД это DATE (PostgreSQL) или строка
// YYYY-MM-DD (SQLite).
//
// Нулевое значение означает «дата не задана»: в proto — пустое поле,
// в БД — NULL.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// NewDate выделяет календарную дату момента по UTC: время суток и пояс
// отбрасываются. Момент 2000-01-02T00:30+03:00 — это 2000-01-01 по UTC.
func NewDate(moment time.Time) Date {
	utc := moment.UTC()

	return Date{Year: utc.Year(), Month: utc.Month(), Day: utc.Day()}
}

// ParseDate разбирает дату в формате YYYY-MM-DD (time.DateOnly).
//
// Формат совпадает с тем, как дата хранится в БД и отдаётся в логах.
func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return Date{}, fmt.Errorf("%w: %q is not a %s date", ErrInvalidDate, value, time.DateOnly)
	}

	return NewDate(parsed), nil
}

// String возвращает дату в формате YYYY-MM-DD, а для незаданной — пустую строку.
//
// Печатаются ровно те значения, что лежат в модели: несуществующая дата не
// «нормализуется» (31 февраля не превращается в 3 марта), иначе в тексте ошибки
// и в логе клиент увидел бы уже другую дату, а не ту, что отправил.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}

	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// Time возвращает полночь UTC этой даты: нужна для конвертации в proto
// (Timestamp) и для сравнений с текущим днём.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// IsZero сообщает, что дата не задана.
func (d Date) IsZero() bool { return d == Date{} }

// Valid сообщает, что дата существует в календаре.
//
// time.Date нормализует невозможные даты (31 февраля → 3 марта), поэтому
// сравнение с результатом нормализации отсекает именно такие значения.
func (d Date) Valid() bool {
	if d.IsZero() {
		return false
	}

	return NewDate(d.Time()) == d
}

// Compare сравнивает даты как календарные значения: -1, 0 или 1.
func (d Date) Compare(other Date) int {
	switch {
	case d.Year != other.Year:
		return cmpInt(d.Year, other.Year)
	case d.Month != other.Month:
		return cmpInt(int(d.Month), int(other.Month))
	default:
		return cmpInt(d.Day, other.Day)
	}
}

// After сообщает, что дата позже другой.
func (d Date) After(other Date) bool { return d.Compare(other) > 0 }

// cloneDate копирует дату: модель не должна разделять состояние с вызывающей
// стороной через указатель из входных данных.
func cloneDate(value *Date) *Date {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

// cmpInt возвращает знак разницы двух чисел.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
