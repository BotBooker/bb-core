package availability

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	MinGranularity = 10 * time.Minute
	MaxGranularity = 24 * time.Hour
)

type BitmapManager struct {
	redisClient *redis.Client
}

func NewBitmapManager(redisClient *redis.Client) *BitmapManager {
	return &BitmapManager{
		redisClient: redisClient,
	}
}

// ValidateGranularity checks if the granularity is valid (10 min to 24h, steps of 5 min)
func ValidateGranularity(granularity int) error {
	if granularity < 10 || granularity > 1440 { // 1440 minutes = 24 hours
		return fmt.Errorf("granularity must be between 10 minutes and 24 hours (1440 minutes), got %d", granularity)
	}
	if granularity%5 != 0 {
		return fmt.Errorf("granularity must be in steps of 5 minutes, got %d", granularity)
	}
	return nil
}

// GetBitmapKey returns the Redis key for a service's availability bitmap on a specific date
func GetBitmapKey(serviceID string, date time.Time) string {
	return fmt.Sprintf("availability:%s:%s", serviceID, date.Format("2006-01-02"))
}

// CalculateBitmapSize calculates the number of bits needed for a day
func CalculateBitmapSize(granularityMinutes int) int {
	minutesInDay := 24 * 60
	return minutesInDay / granularityMinutes
}

// TimeToBitIndex converts a time to a bitmap index based on granularity
func TimeToBitIndex(t time.Time, granularityMinutes int) int {
	minutesSinceMidnight := t.Hour()*60 + t.Minute()
	return minutesSinceMidnight / granularityMinutes
}

// BitIndexToTime converts a bitmap index to the start time of that slot
func BitIndexToTime(date time.Time, index int, granularityMinutes int) time.Time {
	minutesOffset := index * granularityMinutes
	return time.Date(date.Year(), date.Month(), date.Day(),
		minutesOffset/60, minutesOffset%60, 0, 0, date.Location())
}

// SplitBookingIntoSplits converts a booking time range into a list of bitmap indices
func SplitBookingIntoSplits(startTime time.Time, durationMinutes int, granularityMinutes int) []int {
	var indices []int

	startIndex := TimeToBitIndex(startTime, granularityMinutes)
	slotCount := durationMinutes / granularityMinutes

	for i := 0; i < slotCount; i++ {
		indices = append(indices, startIndex+i)
	}

	return indices
}

// SplitWorkingHoursIntoSplits converts working hours intervals into bitmap indices
func SplitWorkingHoursIntoSplits(date time.Time, workingHours []string, granularityMinutes int) ([]int, error) {
	var indices []int

	for _, interval := range workingHours {
		// Parse "HH:MM-HH:MM" format
		var startHour, startMinute, endHour, endMinute int
		_, err := fmt.Sscanf(interval, "%d:%d-%d:%d", &startHour, &startMinute, &endHour, &endMinute)
		if err != nil {
			return nil, fmt.Errorf("invalid working hours format: %s", interval)
		}

		startTime := time.Date(date.Year(), date.Month(), date.Day(),
			startHour, startMinute, 0, 0, date.Location())
		endTime := time.Date(date.Year(), date.Month(), date.Day(),
			endHour, endMinute, 0, 0, date.Location())

		startIndex := TimeToBitIndex(startTime, granularityMinutes)
		endIndex := TimeToBitIndex(endTime, granularityMinutes)

		for i := startIndex; i < endIndex; i++ {
			indices = append(indices, i)
		}
	}

	return indices, nil
}

// GetBit returns the bit value at the given position
func GetBit(bitmap []byte, position int) int {
	byteIndex := position / 8
	bitIndex := uint(position % 8)

	if byteIndex >= len(bitmap) {
		return 0
	}

	return int((bitmap[byteIndex] >> bitIndex) & 1)
}

// SetBit sets the bit at the given position to the given value (0 or 1).
func SetBit(bitmap []byte, position int, value int) {
	byteIndex := position / 8
	bitIndex := uint(position % 8)

	if value == 1 {
		bitmap[byteIndex] |= 1 << bitIndex
	} else {
		bitmap[byteIndex] &^= 1 << bitIndex
	}
}
