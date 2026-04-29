# Change: TTL Management and Cron Rebuilds for Bitmap Cache

## Why ⚡

The bitmap availability system requires proper cache management to maintain accuracy without exhausting Redis memory or rebuilding too frequently. Currently, bitmaps need TTL management to handle far-future availability efficiently, and cron-based rebuilds are needed for periodic cache refresh without pub/sub complexity.

**Problem**: 
- Bitmaps grow indefinitely without TTL → Memory exhaustion
- Stale bitmaps cause incorrect availability (users see unavailable slots or vice versa)
- Manual cache management not feasible → Need automation
- Pub/sub complexity → Cron is simpler for availability systems

**Why now?**
- Bitmap logic is already implemented → Ready for TTL management
- Core bitmap operations working → Need cache lifecycle management
- Production systems require proper monitoring and maintenance
- Far-future availability needs separate TTL strategy

## What Changes

### New Capabilities
- **ttl-management**: Cache TTL handling for bitmap availability data
- **cron-rebuilds**: Background job for periodic bitmap rebuild strategy

### BREAKING
None. These additions enhance existing bitmap system without changing behavior.

## TTL Management Strategy

```
┌─────────────────────────────────────────────────────────┐
│                    TTL HIERARCHY                          │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  Current Day → 5 Days → 10+ Days                          │
│    ↓                   ↓                                    │
│  TTL: 24 hrs        TTL: 7 days                          │
│                     ↓                                      │
│                     TTL: 14 days                          │
│                                                           │
│  Why?:                                                      │
│  - Near-term: High churn → Short TTL                      │
│  - Mid-term: Moderate churn → Medium TTL                   │
│  - Far-term: Low churn → Long TTL                         │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

## Cron Rebuild Strategy

```
┌─────────────────────────────────────────────────────────────┐
│                     CRON SCHEDULES                            │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Business Hours (8am-6pm):                                    │
│    - Every 5 minutes → Rebuild bitmap                         │
│    - High-frequency updates                                   │
│                                                             │
│  Off-Peak Hours (6pm-8am):                                    │
│    - Every 5 minutes → Rebuild bitmap                         │
│    - No bookings expected → Full rebuild safe                 │
│                                                             │
│  Weekend:                                                     │
│    - Every 5 minutes → Rebuild bitmap                         │
│    - Low activity (unless staff work)                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## What Changes (Detailed)

### Database Schema

```sql
-- Add TTL tracking bitmap (optional, for admin monitoring)
CREATE TABLE IF NOT EXISTS bitmap_cache_metadata (
    service_id VARCHAR(50) PRIMARY KEY,
    current_ttl INT NOT NULL,        -- in seconds
    next_rebuild AT TIME WITH TIME ZONE,
    last_rebuild_at TIMESTAMP DEFAULT NOW(),
    cache_hits INT DEFAULT 0,
    cache_misses INT DEFAULT 0
);
```

### Go Implementation

```go
type TTLManager struct {
    redisClient    *redis.Client
    minTTL         int64  // 24 hours
    maxTTL         int64  // 14 days
    rebuildCron    string // "*/5 * * * *"
}

type RebuildScheduler struct {
    redisClient      *redis.Client
    nextRebuild      time.Time
    rebuildInterval  time.Duration
}
```

## Capabilities

### New Capabilities
- **ttl-management**: Bitmap cache TTL handling (24 hr to 14 day TTLs)
- **cron-rebuilds**: Background rebuild job (every 5 min with monitoring)

## Impact

### Code Changes
- **internal/availability/bitmap.go**: TTL management in BitmapManager
- **cmd/bookerbotapi**: Cron job integration
- **internal/rebuild/**: New rebuild scheduler and manager packages
- **db/migrations/**: Add metadata table for cache tracking

### API Changes
- GET `/api/admin/cache/{service_id}`: Cache status endpoint
- GET `/api/admin/cache/stats`: Overall cache metrics
- DELETE `/api/admin/cache/{service_id}`: Clear bitmap for rebuild

### Monitoring
- Cache hit/miss rate metrics
- Rebuild success/failure alerts
- TTL aging warnings
- Redis memory usage thresholds

### Behavior Changes

### Bitmap Cache Lifecycle
1. **On Create**: 
   - New bitmap → TTL = 24 hours
   - Load from DB → TTL preserved

2. **On Update**: 
   - Partial rebuild → TTL reset to 24 hours
   - Full rebuild → TTL reset to 7 days (then decays)

3. **On Access**:
   - Cache hit → Increment hit counter
   - Cache miss → Rebuild triggered

### Fallback Behavior
- **TTL Expiry**: Bitmap removed from cache
  - Next access triggers lazy rebuild
  - Graceful degradation to full rebuild
- **Cron Failure**: Bitmap not rebuilt
  - Next successful cron triggers rebuild
  - Log warning, continue operation

```
┌─────────────────────────────────────────────────────────────┐
│                     CACHE EXPIRATION                         │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Scenario 1: TTL expires before cron rebuild                 │
│  - Bitmap removed from cache                                  │
│  - Next access triggers lazy rebuild                          │
│  - Logs: "Bitmap expired, rebuilding from DB"                │
│                                                               │
│  Scenario 2: Cron rebuild succeeds                           │
│  - TTL reset based on rebuild type                            │
│  - Monitoring counters updated                                │
│                                                               │
│  Scenario 3: Multiple TTL expirations                         │
│  - Rebuild queue builds (not concurrent rebuilds)             │
│  - Prioritized by cache age                                   │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Service-Level Granularity
Each service maintains separate TTL:

```go
service1 := &Service{
    ID:         "service1",
    TimeGranularity: 30,  // 30 min steps
    TTL:        24 * hours,      // 24h for near-term
}

service2 := &Service{
    ID:         "service2",
    TimeGranularity: 15,        // 15 min steps (more granular → rebuild more often)
    TTL:        48 * hours,
}
```

### Timezone Handling
- All TTLs stored in UTC
- Bitmap key includes timezone for multi-timezone services
- Rebuild cron runs in primary timezone, processes all services

## Cron Implementation

```bash
#!/bin/bash
# /usr/local/bin/bookerbot-bitmap-rebuild.sh

# Run every 5 minutes
*/5 * * * * /usr/local/bin/bookerbot-bitmap-rebuild.sh
```

### Rebuild Commands
```bash
# Start rebuild
bookerbot-api bitmap-rebuild --service=svc123

# Status
bookerbot-api bitmap-status --service=svc123

# Force rebuild
bookerbot-api bitmap-rebuild --service=svc123 --force
```

## References

- [Redis TTL Commands](https://redis.io/commands/ttl/)
- [Cron Scheduling](https://en.wikipedia.org/wiki/Cron)
- [Bitmap Operations](https://redis.io/commands/bitfield/)

---

*Status: New proposal for TTL and cron management*  
*Last Updated: 2025-04-29*  
*OpenSpec v1.0*