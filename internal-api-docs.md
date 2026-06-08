# Internal Admin API Documentation

## Overview

This is the **internal admin API** for the BookerBot scheduling system. It serves the internal administration UI and handles management of merchants, services, and staff.

## Key Differences from Public API

| Feature | Public API (`spec/openapi.yaml`) | Internal Admin API (`spec/openapi-internal.yaml`) |
|---------|----------------------------------|---------------------------------------------------|
| **Caching** | Uses Redis cache with TTL (48h default) | **No caching** - direct DB writes |
| **Idempotency** | Returns 409 on conflicts | May return 409 on conflicts (acceptable) |
| **Auth** | X-API-Key | X-Admin-Key (internal use) |
| **Error Handling** | Sanitized for external use | Clean messages, debug info available |
| **Scope** | Consumer-facing booking API | Internal admin operations only |
| **TTL Strategy** | Adaptive TTL per service | N/A |

## Conventions

### Entity Persistence
- All admin write operations **write directly to PostgreSQL**
- No Redis intermediate layer
- Uses `goose` migrations via `db/migrations/`

### Authentication
- Uses `X-Admin-Key` header (internal API key)
- Single super-admin account (current scope)
- Future: may support role-based access control

### Idempotency
- **Not guaranteed** - acceptable for internal admin use
- May retry operations without strict atomicity
- Cascade deletes are immediate

### Error Handling
- Returns sanitized error messages
- Includes `error_code`, `message`, `optional details`
- No stack traces in production

### Route Patterns
- All admin routes under `/admin`
- UUID format for entity IDs
- Query parameter filtering for list operations

## Entities

### Merchant
Primary entity for organization. Subsequent services and staff belong to a merchant.

**Endpoints:**
- `POST /admin/merchant` - Create merchant
- `GET /admin/merchant/{id}` - Get merchant
- `DELETE /admin/merchant/{id}` - Delete with cascade
- `GET /admin/merchants/list` - List merchants

### Service
Service offerings for a merchant.

**Endpoints:**
- `POST /admin/services` - Create service
- `DELETE /admin/services/{id}` - Delete service
- `GET /admin/services/list?merchant_id={id}` - List services

**Constraints:**
- `duration_minutes`: 10-1440 (steps of 5)
- `working_hours`: Optional, `HH:MM-HH:MM` format

### Staff
Staff members who can provide services.

**Endpoints:**
- `POST /admin/staff` - Create staff
- `DELETE /admin/staff/{id}` - Delete staff
- `GET /admin/staff/list?merchant_id={id}` - List staff

**Constraints:**
- `service_ids`: Optional array of service UUIDs
- Working hours optional

## Database Design

Migrations are managed via Goose:

```bash
goose -dir ./db/migrations create <name> sql
```

Current tables:
- `merchants` - Primary entity
- `services` - Service offerings
- `staff` - Staff members
- `bookings` - Booking records
- `migration_lock` - Migration tracking

### Indexes
- `idx_merchant_id` - For filtering
- `idx_service_date` - Availability queries
- `idx_status` - Booking status

## Caching Strategy

**No Redis for admin operations.**

All admin operations:
1. Validate input
2. Check conflicts (naming, duplicates)
3. Write directly to PostgreSQL
4. Return response

This is acceptable for internal admin use where:
- Consistency > Performance
- Admin users expect immediate writes
- Cache complexity not needed

## Testing

The admin endpoints are implemented with:
- Table-style tests (Testify)
- Mock repository implementations
- No race conditions expected (internal use)

## Implementation Notes

### TODOs (Known Issues)
- ✅ CRUD operations for entities
- ❌ Cascade delete implemented on admin actions
- ❌ Admin persistence for new entities
- 🔄 Filtering queries implemented

### Error Patterns
```go
type ErrorResponse struct {
    Code      string `json:"error_code"`
    Message   string `json:"message"`
    Details   string `json:"details,omitempty"`
    RequestID string `json:"request_id"`
}
```

## Security

- Internal API keys only
- No external exposure
- Stack traces sanitized
- Debug endpoints restricted

## Migration Guide

To add new admin functionality:

1. Create database migration via goose
2. Implement handler in `handlers/admin_*`
3. Document in `spec/openapi-internal.yaml`
4. Test with table-style tests
5. Update README as needed

## References

- Public API: `spec/openapi.yaml`
- Internal spec: `spec/openapi-internal.yaml`
- Project context: `context.md`
- Implementation plan: `plan.md`
