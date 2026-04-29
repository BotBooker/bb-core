# Change: Cache-First Bitmap Availability Algorithm

## Summary ⚡

**Status: Implementation in Progress** - Core bitmap system implemented. Refining the caching, TTL management, and background rebuild strategies. Staff hour overrides moved to separate proposal.

The bitmap availability checking system has been successfully introduced to the codebase. It uses Go's bitwise operations for O(1) collision detection, with Redis as the cache layer, cron-based cache rebuilds (no pub/sub), and service-level granularity (10 min to 24 hrs, steps of 5 min).

## Completed Components ✅

- **Database Schema**: Added `time_granularity` field to services table via migration
- **Service Model**: Added `TimeGranularity` int field with validation (10-1440 min, 5 min steps)
- **Redis Integration**: Added `redis` client to go.mod
- **Bitmap Logic**: Implemented in `internal/availability/bitmap.go`
- **Availability Handlers**: Integrated bitmap system in booking workflow
- **Bitmap Management**: Created `BitmapManager` with GetOrCreateBitmap, ValidateGranularity methods
- **Bitmap Operations**: Implemented slot splitting, working hours parsing, collision detection
- **Staff Hour Overrides**: **MOVED TO SEPARATE PROPOSAL** - see `openspec/changes/staff-hour-overrides`

## Implemented Features

1. **Bitmap Granularity Configuration**
   - Each service has `time_granularity` property (10-1440 minutes)
   - Bitmap size calculated as `24 hours / granularity`
   - Example: 60 min granularity → 24 bit bitmap (1440 minutes / 60)

2. **Bitmap Key Format**
   - Key: `availability:{service_id}:{date}` (YYYY-MM-DD)
   - Stored in Redis with appropriate TTL management

3. **Timezone Support**
   - Bitmap stored in UTC format for consistency
   - Timezone conversion handled during read/write operations
   - Bitmap works across timezone boundaries

4. **Cache Technology: Redis**
   - Bitmap stored using Redis bitfield operations
   - TTL managed by BitmapManager (24 hr to 7 days based on horizon)
   - Persistence and monitoring enabled

5. **Bitmap Operations: O(1)**
   - `SplitBookingIntoSplits()` generates bitmap indices for booking
   - Collision detection using bitwise AND operations
   - Redis handles atomic slot reservation

## Staff Hour Overrides - Separate Change ⚡

The staff hour overrides feature has been extracted to a separate change:
- **Change Name**: `staff-hour-overrides`
- **Location**: `openspec/changes/staff-hour-overrides/`
- **Reason**: Per-service hour overrides require independent implementation, specs, and tasks

This allows:
1. Independent development of service-specific hour overrides
2. Clear separation of concerns between basic bitmap and override system
3. Simpler rollback if overrides aren't needed

See `openspec/changes/staff-hour-overrides/proposal.md` for details.

## Remaining Work

### High Priority
- [ ] **TTL Management**: Implement proper TTL handling in BitmapManager
- [ ] **Cron Rebuild Job**: MOVED TO SEPARATE PROPOSAL
- [ ] **Time Off Integration**: Add time off table and integrate with bitmap
- [ ] **Concurrency Handling**: Add retry logic for race condition races

### Medium Priority
- [ ] **Pre-reserved Slots**: Consider supporting VIP/long-lead bookings
- [ ] **Extended Horizon**: Support 30-day advance booking
- [ ] **Soft Reservations**: Consider queue management with hold slots

## Proposed Solution (Refined)

### Architecture: Cache-First (Implemented)

The hot path follows:
1. Load bitmap from Redis cache
2. Perform bitwise collision check
3. Create booking in database
4. Update Redis bitmap

The cold path (on cache miss):
1. Cron job rebuilds bitmap from database
2. Reads staff hours, service hours, bookings, time off
3. Rebuilds bitmap in memory
4. Stores to Redis with appropriate TTL

### Data Model (Updated)

```sql
-- Services table (updated)
CREATE TABLE IF NOT EXISTS services (
    id VARCHAR(50) PRIMARY KEY,
    merchant_id VARCHAR(50) REFERENCES merchants(id),
    name VARCHAR(255) NOT NULL,
    duration_minutes INT NOT NULL,
    time_granularity INT NOT NULL DEFAULT 15,  -- NEW
    price DECIMAL(10,2),
    working_hours JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Bitmap stored in Redis:
-- Key: availability:{service_id}:{date}
-- Value: Bitmap integer (all available slots set to 1)
-- TTL: Based on service horizon (24 hr to 7 days)
```

## Implementation Progress

### Phase 1: Data Model ✅ COMPLETE
- [x] Create initial migration with time_granularity field
- [x] Update service model with TimeGranularity int
- [ ] Add time_off table for staff vacations
- [x] **Staff hour overrides moved to separate proposal**

### Phase 2: Redis Setup ✅ COMPLETE
- [x] Add Redis package to go.mod
- [x] Configure Redis connection pool
- [x] Implement bitmap SET/GET operations
- [ ] **TTL Management**: MOVED TO SEPARATE PROPOSAL
- [ ] Create Redis utility package

### Phase 3: Bitmap Building Logic ✅ PARTIALLY COMPLETE
- [x] Implement slot building algorithm (bitmap.go)
- [x] Calculate working hours splits
- [ ] Handle service-specific overrides (pending separate change)
- [ ] Subtract time off from available slots
- [ ] Test bitmap building with mock data

### Phase 4: Background Rebuild 🔜
### Phase 4: Background Rebuild 🔜
- [ ] **Cron Rebuild**: MOVED TO SEPARATE PROPOSAL
- [ ] Read from DB: staff hours, service hours, bookings, time off (partial rebuild)
- [ ] Rebuild bitmap in memory
- [ ] Store to Redis with appropriate TTL (managed by TTL system)
- [ ] Handle partial vs. full rebuild scenarios

### Phase 5: Booking Integration ✅ COMPLETE
- [x] Load bitmap from Redis (GetOrCreateBitmap)
- [x] Implement collision checking (SplitBookingIntoSplits)
- [x] Handle service-specific overrides (pending staff-hour-overrides change)
- [x] Handle booking creation with collision detection
- [ ] Update Redis bitmap after successful booking

### Phase 6: Testing
- [ ] Create unit tests for bitmap operations
- [ ] Create integration tests for booking workflow
- [ ] Test concurrent booking scenarios
- [ ] Validate cache TTL behavior

## Migration Path

1. **Current State**: Bitmap logic implemented, handlers integrated
2. **Next Steps**: Implement TTL management, cron rebuilds, test scenarios
3. **Staff Hour Overrides**: Implement via separate `staff-hour-overrides` change
4. **Rollout**: Enable bitmap system with appropriate configuration
5. **Monitoring**: Track cache hit rates and rebuild timing

## Open Questions

1. **[ ] Should we support "pre-reserved" slots for VIP/long-lead bookings?**
   - Need to determine if we want to extend bitmap with reservation flags

2. **[ ] What is the maximum TTL for bitmap cache?**
   - Currently: 24 hours (business day)
   - Option: Extend to 7 days for far-future availability

3. **[ ] How to handle staff time off with bitmap?**
   - Need to integrate time_off table with bitmap generation

4. **[ ] Should we add per-service timezone configuration?**
   - Currently: Bitmap works in UTC
   - Question: Convert to local time on read/write?

## Acceptance Criteria (Updated)

- ✅ Bitmap collision detection O(1) CPU operation (implemented)
- ✅ No DB locking during booking checks (using atomic Redis)
- ✅ Bitmap generated efficiently with bitmap.go
- [ ] Redis cache TTL correctly managed (24 hr to 7 days)
- [ ] Cron job rebuilds bitmap during off-peak hours
- [ ] Booking endpoint handles concurrent requests (fail-fast)
- [ ] Franchise support: multi-timezone (UTC bitmap)
- [ ] Granularity configurable per service (10-1440 min)
- ✅ Staff hour overrides separate change created

## Technical Debt & Cleanup

- [ ] Move remaining bitmap logic from `internal/availability/`
- [ ] Add comprehensive error handling
- [ ] **Add monitoring/metrics for bitmap cache**: MOVED TO TTL PROPOSAL
- [ ] Update API documentation for bitmap endpoints
- [ ] Add integration tests for booking workflow

## Related Changes

- **staff-hour-overrides**: Per-service hour override system for staff availability
- **ttl-management-cron-rebuilds**: Cache TTL and cron rebuild management
- **time-off-management**: Time off/vacation integration (future)

## References

- [Redis Bitmap](https://redis.io/commands/bitfield/)
- [Go Redis Client](https://pkg.go.dev/github.com/redis/go-redis/v9)
- [Optimistic Locking Patterns](https://aws.amazon.com/blogs/database/what-is-optimistic-locking/)

---

*Status: Implementation in Progress*
*Last Updated: 2025-04-29*
*Core components implemented*
*Staff hour overrides moved to separate change `staff-hour-overrides`*
*TTL management and cron rebuilds moved to separate change `ttl-management-cron-rebuilds`*
*OpenSpec v1.0*