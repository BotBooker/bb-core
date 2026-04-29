-- +goose Up
-- +goose StatementBegin
-- migrations/001_create_tables.sql
CREATE TABLE IF NOT EXISTS merchants (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    welcome_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS services (
    id VARCHAR(50) PRIMARY KEY,
    merchant_id VARCHAR(50) REFERENCES merchants(id),
    name VARCHAR(255) NOT NULL,
    duration_minutes INT NOT NULL,
    time_granularity INT NOT NULL DEFAULT 15,
    price DECIMAL(10,2),
    working_hours JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS staff (
    id VARCHAR(50) PRIMARY KEY,
    merchant_id VARCHAR(50) REFERENCES merchants(id),
    name VARCHAR(255) NOT NULL,
    service_ids JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bookings (
    id VARCHAR(50) PRIMARY KEY,
    user_id VARCHAR(100) NOT NULL,
    service_id VARCHAR(50) REFERENCES services(id),
    staff_id VARCHAR(50) REFERENCES staff(id),
    start_time TIMESTAMP NOT NULL,
    duration_minutes INT NOT NULL,
    price DECIMAL(10,2),
    paid BOOLEAN DEFAULT FALSE,
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_service_date (service_id, start_time),
    INDEX idx_status (status)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- DROP INDEX idx_users_email ON users;
-- DROP INDEX idx_users_role ON users;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS staff;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS merchants;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd

