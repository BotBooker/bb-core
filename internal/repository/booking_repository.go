package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"bookerbotapi/internal/models"

	"github.com/jmoiron/sqlx"
)

type BookingRepository interface {
	GetBookingsByServiceAndDate(ctx context.Context, serviceID string, date time.Time) ([]models.Booking, error)
	GetBookingsByUserID(ctx context.Context, userID string, status string, limit, offset int) ([]models.Booking, int, error)
	GetBookingByID(ctx context.Context, id string) (*models.Booking, error)
	CreateBooking(ctx context.Context, booking *models.Booking) error
	UpdateBookingStatus(ctx context.Context, id string, status string) error
	GetServiceByID(ctx context.Context, id string) (*models.Service, error)
	GetStaffByID(ctx context.Context, id string) (*models.Staff, error)
	GetMerchantByID(ctx context.Context, id string) (*models.Merchant, error)
}

type PostgresBookingRepository struct {
	db *sqlx.DB
}

func NewPostgresBookingRepository(db *sqlx.DB) *PostgresBookingRepository {
	return &PostgresBookingRepository{
		db: db,
	}
}

func (r *PostgresBookingRepository) GetBookingsByServiceAndDate(ctx context.Context, serviceID string, date time.Time) ([]models.Booking, error) {
	var bookings []models.Booking

	// Query for bookings on the specific date that are not cancelled
	query := `
		SELECT id, user_id, service_id, staff_id, start_time, duration_minutes, 
		       price, paid, status, created_at, updated_at
		FROM bookings
		WHERE service_id = $1 
		  AND DATE(start_time) = $2 
		  AND status != 'cancelled'
		ORDER BY start_time ASC
	`

	err := r.db.SelectContext(ctx, &bookings, query, serviceID, date.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to get bookings: %w", err)
	}

	return bookings, nil
}

func (r *PostgresBookingRepository) GetBookingsByUserID(ctx context.Context, userID string, status string, limit, offset int) ([]models.Booking, int, error) {
	var bookings []models.Booking
	var total int

	// Get total count
	countQuery := `
		SELECT COUNT(*)
		FROM bookings
		WHERE user_id = $1
	`
	args := []interface{}{userID}

	if status != "" {
		countQuery += " AND status = $2"
		args = append(args, status)
	}

	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	// Get paginated results
	query := `
		SELECT id, user_id, service_id, staff_id, start_time, duration_minutes, 
		       price, paid, status, created_at, updated_at
		FROM bookings
		WHERE user_id = $1
	`
	queryArgs := []interface{}{userID}

	if status != "" {
		query += " AND status = $2"
		queryArgs = append(queryArgs, status)
	}

	query += " ORDER BY start_time DESC LIMIT $%d OFFSET $%d"

	queryArgs = append(queryArgs, limit, offset)
	query = fmt.Sprintf(query, len(queryArgs)-1, len(queryArgs))

	err = r.db.SelectContext(ctx, &bookings, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get bookings: %w", err)
	}

	return bookings, total, nil
}

func (r *PostgresBookingRepository) GetBookingByID(ctx context.Context, id string) (*models.Booking, error) {
	var booking models.Booking

	query := `
		SELECT id, user_id, service_id, staff_id, start_time, duration_minutes, 
		       price, paid, status, created_at, updated_at
		FROM bookings
		WHERE id = $1
	`

	err := r.db.GetContext(ctx, &booking, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	return &booking, nil
}

func (r *PostgresBookingRepository) CreateBooking(ctx context.Context, booking *models.Booking) error {
	query := `
		INSERT INTO bookings (id, user_id, service_id, staff_id, start_time, duration_minutes, 
		                      price, paid, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.db.ExecContext(ctx, query,
		booking.ID,
		booking.UserID,
		booking.ServiceID,
		booking.StaffID,
		booking.StartTime,
		booking.DurationMinutes,
		booking.Price,
		booking.Paid,
		booking.Status,
		booking.CreatedAt,
		booking.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create booking: %w", err)
	}

	return nil
}

func (r *PostgresBookingRepository) UpdateBookingStatus(ctx context.Context, id string, status string) error {
	query := `
		UPDATE bookings
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update booking status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("booking not found")
	}

	return nil
}

func (r *PostgresBookingRepository) GetServiceByID(ctx context.Context, id string) (*models.Service, error) {
	var service models.Service

	query := `
		SELECT id, merchant_id, name, duration_minutes, time_granularity, 
		       price, working_hours
		FROM services
		WHERE id = $1
	`

	err := r.db.GetContext(ctx, &service, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	return &service, nil
}

func (r *PostgresBookingRepository) GetStaffByID(ctx context.Context, id string) (*models.Staff, error) {
	var staff models.Staff

	query := `
		SELECT id, merchant_id, name, service_ids
		FROM staff
		WHERE id = $1
	`

	err := r.db.GetContext(ctx, &staff, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}

	return &staff, nil
}

func (r *PostgresBookingRepository) GetMerchantByID(ctx context.Context, id string) (*models.Merchant, error) {
	var merchant models.Merchant

	query := `
		SELECT id, name, welcome_message
		FROM merchants
		WHERE id = $1
	`

	err := r.db.GetContext(ctx, &merchant, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get merchant: %w", err)
	}

	return &merchant, nil
}
