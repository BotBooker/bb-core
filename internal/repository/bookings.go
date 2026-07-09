// internal/repository/bookings.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"bookerbotapi/internal/models"

	"github.com/jmoiron/sqlx"
)

type PostgresAdminRepository struct {
	db *sqlx.DB
}

func NewPostgresAdminRepository(db *sqlx.DB) *PostgresAdminRepository {
	return &PostgresAdminRepository{
		db: db,
	}
}

func (r *PostgresAdminRepository) GetBookingsByServiceAndDate(ctx context.Context, serviceID string, date time.Time) ([]models.Booking, error) {
	var bookings []models.Booking

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

func (r *PostgresAdminRepository) GetBookingsByUserID(ctx context.Context, userID string, status string, limit, offset int) ([]models.Booking, int, error) {
	var bookings []models.Booking
	var total int

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

func (r *PostgresAdminRepository) GetBookingByID(ctx context.Context, id string) (*models.Booking, error) {
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

func (r *PostgresAdminRepository) CreateBooking(ctx context.Context, booking *models.Booking) error {
	query := `
		INSERT INTO bookings (id, user_id, service_id, staff_id, start_time, duration_minutes, 
		       price, paid, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
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
	)

	if err != nil {
		return fmt.Errorf("failed to create booking: %w", err)
	}

	return nil
}

func (r *PostgresAdminRepository) UpdateBookingStatus(ctx context.Context, id string, status string) error {
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

func (r *PostgresAdminRepository) GetServiceByID(ctx context.Context, id string) (*models.Service, error) {
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

func (r *PostgresAdminRepository) CreateService(ctx context.Context, service *models.Service) error {
	query := `
		INSERT INTO services (id, merchant_id, name, duration_minutes, time_granularity, 
		       price, working_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(ctx, query,
		service.ID,
		service.MerchantID,
		service.Name,
		service.DurationMinutes,
		service.TimeGranularity,
		service.Price,
		service.WorkingHours,
	)

	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

func (r *PostgresAdminRepository) DeleteService(ctx context.Context, id string) error {
	query := `DELETE FROM services WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	return nil
}

func (r *PostgresAdminRepository) ListServicesFiltered(ctx context.Context, merchantID string, nameContains string) ([]models.Service, error) {
	var services []models.Service
	query := `
		SELECT id, merchant_id, name, duration_minutes, time_granularity, 
		       price, working_hours
		FROM services
		WHERE merchant_id = $1
		  AND (name ILIKE $2 OR $2 = '')
	`

	args := []interface{}{merchantID}
	if nameContains != "" {
		args = append(args, nameContains)
	}

	err := r.db.SelectContext(ctx, &services, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	return services, nil
}

// GetServicesByMerchantID returns all services for a merchant (convenience wrapper).
func (r *PostgresAdminRepository) GetServicesByMerchantID(ctx context.Context, merchantID string) ([]models.Service, error) {
	return r.ListServicesFiltered(ctx, merchantID, "")
}

func (r *PostgresAdminRepository) GetStaffByID(ctx context.Context, id string) (*models.Staff, error) {
	var staff models.Staff

	query := `
		SELECT id, merchant_id, name, services
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

func (r *PostgresAdminRepository) CreateStaff(ctx context.Context, staff *models.Staff) error {
	servicesJSON, _ := json.Marshal(staff.Services)

	query := `
		INSERT INTO staff (id, merchant_id, name, services, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`

	_, err := r.db.ExecContext(ctx, query,
		staff.ID,
		staff.MerchantID,
		staff.Name,
		servicesJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to create staff: %w", err)
	}

	return nil
}

func (r *PostgresAdminRepository) DeleteStaff(ctx context.Context, id string) error {
	query := `DELETE FROM staff WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete staff: %w", err)
	}
	return nil
}

func (r *PostgresAdminRepository) ListStaffFiltered(ctx context.Context, merchantID string, nameContains string) ([]models.Staff, error) {
	var staff []models.Staff
	query := `
		SELECT id, merchant_id, name, services
		FROM staff
		WHERE merchant_id = $1
		  AND (name ILIKE $2 OR $2 = '')
	`

	args := []interface{}{merchantID}
	if nameContains != "" {
		args = append(args, nameContains)
	}

	err := r.db.SelectContext(ctx, &staff, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list staff: %w", err)
	}

	return staff, nil
}

// GetStaffByMerchantID returns all staff for a merchant (convenience wrapper).
func (r *PostgresAdminRepository) GetStaffByMerchantID(ctx context.Context, merchantID string) ([]models.Staff, error) {
	return r.ListStaffFiltered(ctx, merchantID, "")
}

func (r *PostgresAdminRepository) GetMerchantByID(ctx context.Context, id string) (*models.Merchant, error) {
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

func (r *PostgresAdminRepository) CreateMerchant(ctx context.Context, merchant *models.Merchant) error {
	query := `
		INSERT INTO merchants (id, name, welcome_message)
		VALUES ($1, $2, $3)
	`

	_, err := r.db.ExecContext(ctx, query,
		merchant.ID,
		merchant.Name,
		merchant.WelcomeMessage,
	)

	if err != nil {
		return fmt.Errorf("failed to create merchant: %w", err)
	}

	return nil
}

func (r *PostgresAdminRepository) DeleteMerchant(ctx context.Context, id string) error {
	query := `DELETE FROM merchants WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete merchant: %w", err)
	}
	return nil
}

func (r *PostgresAdminRepository) ListMerchantsFiltered(ctx context.Context, nameContains string) ([]models.Merchant, error) {
	var merchants []models.Merchant
	query := `
		SELECT id, name, welcome_message
		FROM merchants
		WHERE (name ILIKE $1 OR $1 = '')
	`

	args := []interface{}{}
	if nameContains != "" {
		args = append(args, nameContains)
	}

	err := r.db.SelectContext(ctx, &merchants, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list merchants: %w", err)
	}

	return merchants, nil
}
