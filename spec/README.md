# OpenAPI Specification Plan for `api/`

## 1. Overview
This project is the backend API for a messenger-bot-based scheduling application. It will handle business logic, booking management, and availability calculations, serving both the Telegram (`tg-bot`) and Max Messenger (`max-bot`) clients.

- **Version**: v1
- **Base Path**: `/api/v1`
- **Data Format**: JSON
- **Target Framework**: Golang (likely using `gin` or `chi`)

## 2. Authentication & Security
Since clients are internal services (bots), we will use a simple but secure API Key mechanism.
- **Header**: `X-API-Key`
- **API Keys**: Configured per bot service instance.
- **Customer Identification**: Bots will pass the unique Messenger/Telegram User ID in the request payload or a dedicated header (`X-User-Id`) to associate bookings with users.

## 3. Core Entities
1. **Service**: A bookable item (e.g., "Consultation", "Maintenance", "Appointment"). Includes name, duration, and working hours and price (optional).
2. **Staff**: A service provider (e.g., "Dr. Smith", "Agent 1"). Includes name and assigned services.
3. **Customer**: Linked to a Messenger/Telegram user ID / use name.
4. **Booking**: The core entity representing a reserved time slot. Includes service, staff (for denormalization), customer, starting time, duration and price / paid flag (optional).
5. **Merchant**: A business entity to which services and bookings belong. Includes name, welcome message.
6. **Slot/Availability**: Computed or stored time windows where a Staff member is available for a Service.

### Working hours
Holds array of strings which are time intervals (H:m-H:m). Represents working schedule in JSON format for each day of the week. If Service is unavaiable in this day of the week, record for such day holds an empty array.
Example:
```json
{
    "monday": [ "10:00-20:00" ],
    "tuesday": ["10:00-13:00","14:00-19:00"],
    ...
    "saturday": ["11:00-14:00"],
    "sunday": [],
}
```

## 4. Endpoint Design (RESTful)

### 4.1. Availability & Slots
- `GET /availability/dates?staff_id=...&service_id=...`
  - Returns a list of dates with available slots within a range (e.g., next 7 days).
- `GET /availability/slots?date=...&staff_id=...&service_id=...`
  - Returns specific time windows available for booking on a given date.

### 4.2. Catalog Management
- `GET /catalog/services?merchant_id=...`
  - Lists all available services for a merchant
- `GET /catalog/staff?merchant_id=...`
  - Lists all staff members for a merchant

### 4.3. Booking Operations
- `POST /bookings`
  - **Body**: `{ "user_id": "...", "service_id": "...", "staff_id": "...", "start_time": "...", "duration_minutes": "..." }`
  - Creates a new booking. Returns 409 if the slot is already taken.
- `GET /bookings`
  - **Query**: `?user_id=...&status=pending`
  - Lists bookings for a specific customer or all bookings (admin).
- `GET /bookings/{id}`
  - Returns details of a single booking.
- `PUT /bookings/{id}/cancel`
  - Cancels a booking. Idempotent.

### 4.4. Admin/Settings (Optional v1)
- `POST /admin/services` / `DELETE /admin/services/{id}`
- `POST /admin/staff` / `DELETE /admin/staff/{id}`
- `POST /admin/merchant` / `GET /admin/merchant/{id}` / `DELETE /admin/merchant/{id}`
- `GET /admin/services/list/{filter}`
- `GET /admin/staff/list/{filter}`
- `GET /admin/merchants/list/{filter}`

## 5. Request/Response Models (Draft)

### Booking Payload
```json
{
  "user_id": "string",
  "service_id": "string",
  "staff_id": "string",
  "start_time": "2023-10-27T14:00:00Z",
  "duration_minutes": 30
}
```

### Error Response Standard
```json
{
  "error_code": "string",
  "message": "string",
  "details": "string"
}
```

## 6. Technical Implementation Notes
- **Idempotency**: `POST /bookings` should ideally support an `Idempotency-Key` header to prevent double-bookings due to network retries from bots.
- **Concurrency**: Handling slot availability requires locking or optimistic concurrency checks to prevent race conditions when two users try to book the same slot.
- **Validation**: Use a library like `go-playground/validator` for request struct validation.
- **OpenAPI Tooling**: Consider using `swaggo/swag` for documentation or `oapi-codegen` to generate Go server interfaces directly from the `.yaml` spec file.

## 7. Next Steps for API Development
1. Define and write the full `openapi.yaml` file in `api/spec/`.
2. Initialize the Go project in `api/` with `go mod init`.
3. Set up the router middleware (API Key validation, Logging).
4. Implement the `/availability/slots` endpoint first, as it's critical for the bot flow.
5. Implement `POST /bookings` with database persistence (e.g., PostgreSQL/SQLite).

