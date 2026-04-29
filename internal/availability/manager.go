package availability

import (
	"context"
	"fmt"
	"time"

	"bookerbotapi/internal/models"
	"bookerbotapi/internal/repository"

	"github.com/redis/go-redis/v9"
)

type AManager interface {
	GetOrCreateBitmap(ctx context.Context, service *models.Service, date time.Time) ([]byte, error)
	CheckAvailability(ctx context.Context, service *models.Service, startTime time.Time, durationMinutes int) (bool, error)
	ReserveBooking(ctx context.Context, service *models.Service, startTime time.Time, durationMinutes int) error
}

type AvailabilityManager struct {
	bitmapManager *BitmapManager
	repo          repository.BookingRepository
	redis         *redis.Client
}

func NewAvailabilityManager(redisClient *redis.Client, repo repository.BookingRepository) *AvailabilityManager {
	return &AvailabilityManager{
		bitmapManager: NewBitmapManager(redisClient),
		repo:          repo,
		redis:         redisClient,
	}
}

// GetOrCreateBitmap retrieves or creates the availability bitmap for a service on a specific date
func (am *AvailabilityManager) GetOrCreateBitmap(ctx context.Context, service *models.Service, date time.Time) ([]byte, error) {
	key := GetBitmapKey(service.ID, date)

	// Try to get from Redis
	bitmap, err := am.redis.Get(ctx, key).Bytes()
	if err == nil {
		return bitmap, nil
	}

	if err != redis.Nil {
		return nil, fmt.Errorf("failed to get bitmap from redis: %w", err)
	}

	// Create new bitmap
	bitmap, err = am.createBitmap(ctx, service, date)
	if err != nil {
		return nil, err
	}

	// Save to Redis with TTL of 48 hours
	err = am.redis.Set(ctx, key, bitmap, 48*time.Hour).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to save bitmap to redis: %w", err)
	}

	return bitmap, nil
}

// createBitmap creates a new availability bitmap by combining working hours and existing bookings
func (am *AvailabilityManager) createBitmap(ctx context.Context, service *models.Service, date time.Time) ([]byte, error) {
	granularity := service.TimeGranularity
	bitmapSize := CalculateBitmapSize(granularity)
	bitmapBytes := (bitmapSize + 7) / 8 // Calculate bytes needed
	bitmap := make([]byte, bitmapBytes)

	// Mark available slots based on working hours for this day of week
	workingHours := getWorkingHoursForDay(service.WorkingHours, date.Weekday())
	if len(workingHours) == 0 {
		// No working hours on this day, return empty bitmap (all unavailable)
		return bitmap, nil
	}

	availableIndices, err := SplitWorkingHoursIntoSplits(date, workingHours, granularity)
	if err != nil {
		return nil, err
	}

	// Mark available slots as 1
	for _, idx := range availableIndices {
		setBit(bitmap, idx, 1)
	}

	// Get existing bookings for this date
	bookings, err := am.repo.GetBookingsByServiceAndDate(ctx, service.ID, date)
	if err != nil {
		return nil, fmt.Errorf("failed to get bookings: %w", err)
	}

	// Mark booked slots as 0
	for _, booking := range bookings {
		startTime, err := time.Parse(time.RFC3339, booking.StartTime)
		if err != nil {
			continue
		}

		bookedIndices := SplitBookingIntoSplits(startTime, booking.DurationMinutes, granularity)
		for _, idx := range bookedIndices {
			setBit(bitmap, idx, 0)
		}
	}

	return bitmap, nil
}

// CheckAvailability checks if a booking time slot is available
func (am *AvailabilityManager) CheckAvailability(ctx context.Context, service *models.Service, startTime time.Time, durationMinutes int) (bool, error) {
	date := startTime.Truncate(24 * time.Hour)

	// Get or create bitmap
	bitmap, err := am.GetOrCreateBitmap(ctx, service, date)
	if err != nil {
		return false, err
	}

	granularity := service.TimeGranularity
	indices := SplitBookingIntoSplits(startTime, durationMinutes, granularity)

	// Check all required slots are available (bit = 1)
	for _, idx := range indices {
		if getBit(bitmap, idx) == 0 {
			return false, nil
		}
	}

	return true, nil
}

// ReserveBooking reserves time slots for a new booking
func (am *AvailabilityManager) ReserveBooking(ctx context.Context, service *models.Service, startTime time.Time, durationMinutes int) error {
	date := startTime.Truncate(24 * time.Hour)
	key := GetBitmapKey(service.ID, date)

	// Use Redis transaction to ensure atomicity
	err := am.redis.Watch(ctx, func(tx *redis.Tx) error {
		// Get current bitmap
		bitmap, err := tx.Get(ctx, key).Bytes()
		if err != nil && err != redis.Nil {
			return err
		}

		// If bitmap doesn't exist, create it
		if err == redis.Nil {
			bitmap, err = am.createBitmap(ctx, service, date)
			if err != nil {
				return err
			}
		}

		// Check availability again within transaction
		granularity := service.TimeGranularity
		indices := SplitBookingIntoSplits(startTime, durationMinutes, granularity)

		for _, idx := range indices {
			if getBit(bitmap, idx) == 0 {
				return fmt.Errorf("slot unavailable")
			}
		}

		// Mark slots as booked (set to 0)
		for _, idx := range indices {
			setBit(bitmap, idx, 0)
		}

		// Update bitmap in Redis
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, bitmap, 48*time.Hour)
			return nil
		})

		return err
	}, key)

	if err != nil {
		return fmt.Errorf("failed to reserve booking: %w", err)
	}

	return nil
}

// getWorkingHoursForDay returns working hours for a specific weekday
func getWorkingHoursForDay(workingHours models.WorkingHours, weekday time.Weekday) []string {
	switch weekday {
	case time.Monday:
		return workingHours.Monday
	case time.Tuesday:
		return workingHours.Tuesday
	case time.Wednesday:
		return workingHours.Wednesday
	case time.Thursday:
		return workingHours.Thursday
	case time.Friday:
		return workingHours.Friday
	case time.Saturday:
		return workingHours.Saturday
	case time.Sunday:
		return workingHours.Sunday
	default:
		return []string{}
	}
}

// Bit manipulation helper functions
func setBit(bitmap []byte, position int, value int) {
	byteIndex := position / 8
	bitIndex := uint(position % 8)

	if value == 1 {
		bitmap[byteIndex] |= 1 << bitIndex
	} else {
		bitmap[byteIndex] &^= 1 << bitIndex
	}
}

func getBit(bitmap []byte, position int) int {
	byteIndex := position / 8
	bitIndex := uint(position % 8)

	if byteIndex >= len(bitmap) {
		return 0
	}

	return int((bitmap[byteIndex] >> bitIndex) & 1)
}
