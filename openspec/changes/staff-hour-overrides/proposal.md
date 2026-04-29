# Change: Staff Hour Overrides for Per-Service Availability

## Why ⚡

The current system calculates availability based solely on generic `working_hours` JSON stored in the staff model. However, staff may have per-service-specific availability variations (e.g., a staff member available for "Haircuts" 9am-5pm but only available for "Massages" 10am-4pm due to certification requirements or specialization). This change allows service-specific hour overrides while maintaining the generic working hours as a baseline.

**Why now?**
- Staff specialization varies by service type
- Current system doesn't support different hours per service for same staff member
- Franchise locations may need different hour structures per service
- Enables more granular staff scheduling and availability calculation

## What Changes

### New Capabilities
- **staff-hour-overrides**: Per-service hour override system for staff availability calculation

**BREAKING** - Availability calculation now considers service-specific overrides when they exist, falling back to generic working hours when no override exists.

### Modified Capabilities
- **availability**: Adds service-specific hour override lookup during bitmap generation

## Impact

### Code Changes
- **internal/models/staff.go**: Add `staff_hours_override` table/models
- **internal/availability/bitmap.go**: Modify `SplitWorkingHoursIntoSplits()` to check overrides
- **db/migrations/**: New migration for `staff_hours_override` table
- **internal/repository/**: Implement override lookup queries

### API Changes
- POST `/api/v1/admin/staff`: Accept `hour_overrides` JSON for staff hourly overrides
- POST `/api/v1/admin/staff/{id}`: Accept `hour_overrides` per-service

### Data Model
```sql
-- staff_hours_override table
CREATE TABLE IF NOT EXISTS staff_hours_override (
    id VARCHAR(50) PRIMARY KEY,
    staff_id VARCHAR(50) REFERENCES staff(id),
    service_id VARCHAR(50) REFERENCES services(id),
    date DATE NOT NULL,  -- Specific date or use pattern for recurring
    monday JSONB,  -- Monday hours override
    tuesday JSONB,  -- Tuesday hours override
    wednesday JSONB,
    thursday JSONB,
    friday JSONB,
    saturday JSONB,
    sunday JSONB
);

-- Example: Staff John can only do Haircuts, never Massages
-- Staff ID: staff123
-- Service ID: haircut → monday {"M": ["09:00-17:00"]}
-- Service ID: massage   → null (inherits from generic working_hours)
```

### Behavior Changes
- Staff with no override gets generic `working_hours`
- Staff with override uses override hours for that specific service on that day
- Overriding Monday is ignored if generic working_hours also empty
- Default (no overrides): Use generic `working_hours` for all services, all days

### Staff Model
```go
type Staff struct {
    ID              string       `json:"id"`
    Name            string       `json:"name"`
    MerchantID      string       `json:"merchant_id"`
    ServiceIDs      []string     `json:"service_ids"`  // Services this staff can provide
    WorkingHours    WorkingHours  `json:"working_hours"`  // Generic hours (may be empty)
    ServiceOverride map[string][]string // service_id → hours per day
    CreatedAt       time.Time    `json:"created_at"`
    UpdatedAt       time.Time    `json:"updated_at"`
}
```

### Override Validation
- Override hours must be within or narrower than generic working hours
- Invalid override rejected (e.g., override ending after generic hours end)
- Override JSONB format: `{"M":["09:00-17:00"], "F":["10:00-16:00"]}`

### Fallback Hierarchy (for availability calculation)
1. Staff service override (if exists and set)
2. Staff generic working hours (if set)
3. Default: No availability

## Capabilities

### New Capabilities
- **staff-hour-overrides**: Per-service hour override system for staff availability

## Impact

- **Code**: `internal/availability/bitmap.go`, `internal/models/staff.go`, `internal/handlers/admin_staff.go`
- **API**: `/api/v1/admin/staff` endpoints modified
- **Model**: `staff_hours_override` table, `Staff` model updated
- **Dependencies**: No new dependencies
- **Data Schema**: `staff_hours_override` table added

## References

- Redis Bitmap Documentation for availability calculation
- Bitmap slot splitting for day/time calculations

---

*Status: New proposal for staff hour overrides*  
*Last Updated: 2025-04-29*  
*OpenSpec v1.0*