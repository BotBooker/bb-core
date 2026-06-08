# Go/Chi Router + OpenAPI & Redis Implementation Research

## Executive Summary

This document synthesizes best practices for integrating Go/chi router with OpenAPI specifications, sqlx query patterns for schema generation, Redis bitmap availability caching strategies, Go error handling conventions, and recent Go ecosystem patterns. It focuses on improving implementation confidence for:
- Admin routes documentation
- Cache-first bitmap pattern implementation

---

## 1. Go/Chi Router + OpenAPI Specification Integration

### 1.1 Route Mapping Best Practices

```go
package main

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
)

// Pattern 1: Explicit Route Groups with Tags
func setupRouter() chi.Router {
    r := chi.NewRouter()
    
    // Admin routes group (recommended for OpenAPI tagging)
    admin := r.Group("/admin")
    admin.Use(middleware.RequestID())
    admin.Use(middleware.RealIP())
    admin.Use(logMiddleware()) // Custom logging
    
    // OpenAPI schema generation middleware
    r.Use(openAPISchemaHandler())
    
    return r
}
```

### 1.2 Path Definition Guidelines

| Pattern | Usage | OpenAPI Mapping |
|---------|-------|-----------------|
| `/api/{type}/{id}` | Generic resource | `paths["/api/{type}/{id}"]` |
| `/health.{format}` | Format-specific | `paths["/health.{format}"]` |
| `/v{version}/admin/*` | Versioned paths | Versioned operation IDs |
| `/admin/{action}` | Resource verbs | Action-based paths |

### 1.3 Parameter Types & Conventions

```go
// Query parameters - use chi.URLParam to bind
type AvailabilityQueryParams struct {
    SiteID     string `url:"site_id,omitempty"`     // Optional path/query param
    ServiceID  string `url:"service_id"`            // Required
    Status     string `url:"status"`                // Enum: available/unavailable
}

// Path parameters
type RouteParams struct {
    SiteID    string `path:"site_id"`   // Required
    Resource  string `path:"resource"`  // Alphanumeric
}

// Header parameters (for OpenAPI security)
type AuthHeaderParams struct {
    Token string `header:"Authorization"` 
}
```

### 1.4 Schema Definition Strategy

```go
// OpenAPI component schemas
type AvailabilityResponse struct {
    ID           string                   `json:"id"`
    SiteID       string                   `json:"site_id"`
    ServiceType  string                   `json:"service_type"`
    AvailableAt  time.Time                `json:"available_at"`
    Availability Status                 `json:"availability"`
    Metadata     map[string]interface{}   `json:"metadata,omitempty"`
    CreatedAt    time.Time                `json:"created_at"`
    UpdatedAt    time.Time                `json:"updated_at"`
}

type AdminAvailabilityRequest struct {
    ServiceType    string                      `json:"service_type"`
    TTL            time.Duration               `json:"ttl"`
    OverrideStatus Status                      `json:"override_status" validate:"required"`
    Metadata       map[string]interface{}      `json:"metadata,omitempty"`
    Source         string                      `json:"source,omitempty" validate:"omitempty,alphanumeric"`
}

// OpenAPI components
var components = openapi.Components{
    Schemas: map[string]openapi.Schema{
        "AvailabilityResponse": {
            Type: "object",
            Properties: map[string]openapi.SchemaProps{
                "id": {Type: "string", Format: "uuid", Example: "550e8400-e29b-41d4-a716-446655440000"},
                "site_id": {Type: "string", Example: "site-123"},
                "service_type": {Type: "string", Example: "api"},
                "available_at": {Type: "string", Format: "date-time"},
            },
        },
    },
}
```

### 1.5 Middleware Layering

```go
// Order matters:
// 1. Recovery first
// 2. Request/Response logging
// 3. CORS
// 4. Security/Authentication
// 5. Request ID tracking
// 6. Rate limiting
// 7. OpenAPI schema injection

func middlewarePipeline() []chi.Middleware {
    return []chi.Middleware{
        recovery.HandlerMiddleware, // Must be first for proper stack trace
        middleware.RequestID(),     // For distributed tracing
        middleware.RealIP(),        // For client identification
        middleware.Logger(),        // Access logs
        cors.Handler(),            // Cross-origin requests
        middleware.StripSlashes(), // Canonicalize paths
    }
}
```

---

## 2. sqlx Query Patterns & OpenAPI Response Schemas

### 2.1 Pattern-Based Schema Generation

```go
// sqlx query returning structs (recommended)
func getAvailability(ctx context.Context, siteID, serviceID string) (*Availability, error) {
    var availability Availability
    err := db.Get(&availability, query, siteID, serviceID)
    if err != nil && !errors.Is(err, sq.ErrNoRows) {
        return nil, err
    }
    return &availability, nil
}

// Result: Maps directly to AvailabilityResponse schema
// OpenAPI can auto-generate from this struct

// Bulk query pattern
type BatchResult struct {
    Items []Availability `json:"items"`
    Total int64 `json:"total"`
    Offset int `json:"offset"`
    Limit int `json:"limit"`
}

// SQL pagination queries
const listAvailability = `
SELECT id, site_id, service_type, available_at, available_at + interval $2 * seconds AS expires_at, availability, created_at, updated_at
FROM availability
WHERE site_id = $1 AND service_type = $2
  AND available_at <= $3
ORDER BY available_at DESC
LIMIT $4 OFFSET $5
`
```

### 2.2 JSON Tag Conventions

```go
// OpenAPI-compliant Go structs
type Availability struct {
    ID           string        `json:"id"`
    SiteID       string        `json:"site_id"`
    ServiceType  string        `json:"service_type"`
    AvailableAt  time.Time     `json:"available_at"`
    ExpiresAt    time.Time     `json:"expires_at"`       // Derived for OpenAPI expiry calculations
    Availability  Status        `json:"availability"`
    Metadata     json.RawMessage `json:"metadata,omitempty"` // For flexible fields
    CreatedAt    time.Time     `json:"created_at"`
    UpdatedAt    time.Time     `json:"updated_at"`
}

// For OpenAPI enum support
type Status int

const (
    StatusUnknown Status = iota
    StatusAvailable
    StatusUnavailable
    StatusMaintenance
)

func (s Status) MarshalJSON() ([]byte, error) {
    return json.Marshal(statusMap[s])
}

var statusMap = map[Status]string{
    StatusUnknown:     "unknown",
    StatusAvailable:   "available",
    StatusUnavailable: "unavailable",
    StatusMaintenance: "maintenance",
}
```

### 2.3 Nested Query Patterns

```go
// Pattern 1: JOIN for compound resources
type AvailabilityWithSite struct {
    ID       string `json:"id"`
    Availability AvailabilityResp `json:"availability"`
    Site SiteResp `json:"site"` // Nested
}

// Pattern 2: Separate endpoints with relation docs
// /admin/sites/{site-id}/availabilities -> list with filtering
// /admin/sites/{site-id} -> get full site with related data

// Pattern 3: Aggregation queries
type AvailabilityStats struct {
    TotalAvailable   int64            `json:"total_available"`
    TotalUnavailable int64            `json:"total_unavailable"`
    AvailablePct     float64          `json:"available_pct"`
    LastUpdated      time.Time        `json:"last_updated"`
}

// sqlx aggregation
db.QueryRowContext(ctx, `
    SELECT 
        COUNT(*) FILTER (WHERE is_available) as total_available,
        COUNT(*) FILTER (WHERE NOT is_available) as total_unavailable,
        (COUNT(*) FILTER (WHERE is_available) / COUNT(*)) FILTER (WHERE count > 0) as available_pct
    FROM availability WHERE site_id = $1
`, siteID)
```

### 2.4 Response Schema Mapping

| sqlx Result Type | OpenAPI Response Pattern | Use Case |
|-----------------|--------------------------|----------|
| Single struct | `responses["200"]` with schema | Single resource endpoint |
| List with total | `items` array + `x-total-count` | Paginated resources |
| Struct with errors | `anyof` + `one-of` in OpenAPI | Error handling |
| Null results | `204 No Content` + `404` | Optional resources |

```go
// OpenAPI response structure
operation := openapi.Operation{
    OperationID: "GetAvailability",
    Responses: map[string]openapi.Response{
        "200": {
            Description: "Success - availability data returned",
            Content: map[string]openapi.MediaType{
                "application/json": {
                    Schema: openapi.Schema{
                        Type: "object",
                        Properties: map[string]openapi.SchemaProps{
                            "success": {Type: "boolean", Example: true},
                            "data": {
                                OneOf: []openapi.SchemaProps{
                                    {Schema: &availabilitySchema},       // Success case
                                    {Schema: &notFoundSchema},            // 404 case
                                },
                            },
                        },
                    },
                },
            },
        },
        "404": {
            Description: "Resource not found",
            Content: map[string]openapi.MediaType{
                "application/json": {
                    Schema: openapi.Schema{
                        Type: "object",
                        Properties: map[string]openapi.SchemaProps{
                            "error": {Type: "string", Example: "not found"},
                        },
                    },
                },
            },
        },
    },
}
```

---

## 3. Redis Bitmap & Availability Caching Patterns

### 3.1 Cache-First Strategy Architecture

```go
// Pattern 1: Cache-first with fallback to DB
type AvailabilityCache struct {
    client redis.UniversalClient
    keyBuilder KeyBuilder
    defaultTTL time.Duration
}

func (c *AvailabilityCache) Get(ctx context.Context, siteID, serviceID string) (*Availability, error) {
    // 1. Try cache first
    key := c.keyBuilder(siteID, serviceID)
    data, err := c.client.Get(ctx, key).Result()
    if err == nil && data != "" {
        return c.decodeAvailability(data) // Cache hit
    }

    // 2. Cache miss - fetch from DB
    availability, err := c.fetchFromDatabase(ctx, siteID, serviceID)
    if err != nil {
        if isNotFoundErr(err) {
            // Create cache for non-existent items with very short TTL
            err := c.client.Set(ctx, key, json.Encoded(&Availability{
                Availability: StatusUnavailable,
            }), 10*time.Second).Err()
            if err != nil {
                logRedisErr(ctx, err)
            }
            return availability, nil // Return not-found without error
        }
        // Database error - handle appropriately
        return availability, err
    }

    // 3. Cache miss + DB success
    return c.setWithTTL(ctx, key, availability), nil
}

func (c *AvailabilityCache) setWithTTL(ctx context.Context, key string, availability *Availability) *Availability {
    ttl := c.calculateTTL(availability)
    encoded := json.Encoded(availability)
    
    // Pipeline for efficiency
    pipe := c.client.Pipeline()
    pipe.Set(ctx, key, encoded, ttl)
    pipe.Expire(ctx, key, ttl)
    
    // Always write to Redis for consistency
    pipe.Exec(ctx)
    
    return availability
}
```

### 3.2 TTL Management Strategies

```go
// Strategy 1: Predictable TTL based on service type
func calculateTTL(availability *Availability) time.Duration {
    // Different TTLs by service category
    ttlMap := map[string]time.Duration{
        "api":            5 * time.Minute,     // Fast-changing
        "web":            10 * time.Minute,    // Medium changes
        "authentication": 5 * time.Minute,     // Critical but stable
        "infrastructure": 15 * time.Minute,    // Slow-changing
    }
    
    var baseTTL = 5 * time.Minute
    switch availability.ServiceType {
    case "api", "rest":
        baseTTL = 5 * time.Minute
    case "web", "mobile":
        baseTTL = 10 * time.Minute
    case "infrastructure":
        baseTTL = 15 * time.Minute
    }
    
    // Factor in availability status for maintenance periods
    if availability.Availability == StatusMaintenance {
        baseTTL = 1 * time.Minute // Maintenance changes often
    }
    
    // Dynamic backoff on errors
    if availability.LastUpdatedAt.Before(time.Now().Add(-time.Hour)) {
        baseTTL *= 2 // Double TTL on stale data
    }
    
    return baseTTL
}

// Strategy 2: Exponential backoff on cache errors
func backoffCacheWrite(ctx context.Context, key string, data string) error {
    maxRetries := 3
    baseDelay := 100 * time.Millisecond
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        if err := pipe.Set(ctx, key, data, calculatedTTL).Err(); err == nil {
            return nil
        }
        
        if attempt < maxRetries {
            time.Sleep(baseDelay * time.Duration(attempt))
        }
    }
    
    // Final fallback: Write anyway, accepting potential Redis error
    return redis.Err("write skipped after retries")
}
```

### 3.3 Bitmap Implementation (for high-volume availability)

```go
// Bitmap pattern for scale (when needed)
func setupBitmapCache(ctx context.Context, client *redis.Client) {
    // Create bitmap keys: bitmap:site:{siteID}
    bitmapKeys := redis.NewBitmapBuilder(ctx, client)
    
    for _, siteID := range siteIDs {
        bitmapKey := fmt.Sprintf("bitmap:site:%s", siteID)
        // Use bitmap to track availability state (0/1)
        if _, err := client.BitSet(ctx, bitmapKey, "service:web", 1).Result(); err != nil {
            logRedisErr(ctx, err)
        }
    }
    
    // Pattern: Read bitmap, decode to availability map
    func retrieveBitmapAvailability(ctx context.Context, siteID string) ([]string, error) {
        bitmapKey := fmt.Sprintf("bitmap:site:%s", siteID)
        pattern := fmt.Sprintf("bitmap:site:%s:*", siteID)
        
        var services []byte
        var err error
        
        // Get all set bits (available services)
        services, err = client.BitPosLR(ctx, bitmapKey, 1).Result()
        if err != nil {
            return nil, nil
        }
        
        // For full bitmap: retrieve status per service
        // Bitmap is for fast membership checks (is this service available?)
        // Not for full availability objects
        return services, err
    }
}
```

### 3.4 Cache Consistency Patterns

```go
// Pattern 1: Write-through (recommended for admin routes)
func writeAvailabilityDirectly(ctx context.Context, availability *Availability) error {
    // Always write availability changes synchronously
    // This is critical for admin routes
    err := cache.Set(ctx, key, encoded, availability.ExpiresAt.Sub(time.Now()))
    if err != nil {
        // Log but don't fail - cache is best-effort
        logRedisErr(ctx, err)
        return nil // Continue anyway
    }
    
    // Update database too
    return db.Update(ctx, availability)
}

// Pattern 2: Write-behind with async queue (for high-write scenarios)
func setupAsyncAvailabilityCache(ctx context.Context, queue *events.QueuedEvent) error {
    // For admin routes, we prefer synchronous writes
    // But for high-traffic read-endpoints, async batching helps
    
    // This is NOT recommended for admin routes where state consistency is needed
    return nil
}

// Pattern 3: Cache invalidation
func invalidateAvailabilityCache(ctx context.Context, siteID, serviceID string) error {
    key := cache.KeyBuilder.SiteID(serviceID, serviceID)
    
    // Delete specific key
    if err := cache.Client.Del(ctx, key).Err(); err != nil {
        logRedisErr(ctx, err)
    }
    
    // For admin actions, invalidate with warm-up
    if adminAction {
        return cache.Warm(ctx, siteID, serviceID)
    }
    
    return nil
}

// Cache warming for admin routes
func warmCacheAfterAdminUpdate(ctx context.Context) error {
    // After admin routes modify availability, always warm cache
    var sites []string
    if err := db.Query(ctx, &sites, "SELECT site_id FROM availability WHERE updated_at > NOW() - INTERVAL '1 hour'"); err != nil {
        return err
    }
    
    for _, siteID := range sites {
        // Trigger cache warm-up for this site's services
        go cache.Warm(ctx, siteID, "web", "api")
    }
    
    return nil
}
```

### 3.5 Cache-First Pseudocode Reference

```
┌─────────────────────────────────────────┐
│         CACHE-FIRST STRATEGY            │
└─────────────────────────────────────────┘
│                                          │
│    ┌─────┐    ┌──────────┐    ┌─────────┐  │
│    │ READ│───▶│ CACHE HIT│───▶│ RETURN   │  │
│    │ Req│    └──────────┘    │ CACHE    │  │
│    └─────┘                   └─────────┘  │
│        │                              │   │
│        │  MISS                       MISS│
│        │  ─────────────┐               │   │
│        │               │  ┌────────────┐│   │
│        │               │  │ DB FETCH   ││   │
│        │         ┌──────┤            ││   │
│        └─────────►     └──────┤      ││     │
│                      │ DB    │────┘     │
│                      ↓ MISS ─────────►   │
│                  ┌──────────┐          │
│                   │ UPDATE  │ ◄────────┘
│                  └──────────┘  │
│                             │  FALLBACK
│                             │ to DB
│                             └──────────┘
```

---

## 4. Go Error Handling & HTTP Response Codes

### 4.1 Error Classification Convention

```go
// Define error types for OpenAPI mapping
package errors

import (
    "errors"
    "net/http"
)

// Error types for OpenAPI response mapping
var (
    // Validation errors
    ErrInvalidParams      = errors.New("invalid parameters")
    ErrMissingSiteID      = errors.New("missing required site_id parameter")
    ErrInvalidServiceType = errors.New("invalid service type")
    
    // Authorization errors  
    ErrUnauthorized       = errors.New("unauthorized access")
    ErrForbidden          = errors.New("forbidden operation")
    
    // Resource errors
    ErrNotFound           = errors.New("resource not found")
    ErrResourceInUse     = errors.New("resource is in use")
    ErrResourceConflict  = errors.New("resource conflict detected")
    
    // Infrastructure
    ErrRedisUnavailable   = errors.New("redis cache unavailable")
    ErrDatabaseError      = errors.New("database operation failed")
    ErrInternal           = errors.New("internal server error")
)

// Error response structs
type ErrorResponse struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    Details   string `json:"details,omitempty"`
    RequestID string `json:"request_id"`
}

func (e *ErrorResponse) Error() string {
    return e.Message
}
```

### 4.2 HTTP to Error Code Mapping

| HTTP Code | Go Error Type | OpenAPI Response Pattern | Admin Route Considerations |
|-----------|---------------|--------------------------|---------------------------|
| 200 OK | `nil` or success | Success response | Standard |
| 201 Created | Custom creation | 201 with created entity | Admin create operations |
| 204 No Content | `nil` | 204 (optional) | Admin delete operations |
| 400 Bad Request | `ErrInvalidParams` | BadRequest schema | Parameter validation |
| 401 Unauthorized | `ErrUnauthorized` | Unauthorized schema | Admin auth failures |
| 403 Forbidden | `ErrForbidden` | Forbidden schema | Admin permission failures |
| 404 Not Found | `ErrNotFound` | NotFound schema | Admin resource lookups |
| 409 Conflict | `ErrResourceConflict` | Conflict schema | Admin updates |
| 422 Unprocessable Entity | Validation | Validation schema | Complex requests |
| 500 Internal Error | Generic | Error schema | Cache/Redis unavailable |
| 503 Unavailable | Service unavailable | Service down | Redis down |

### 4.3 Error Response Handler

```go
// OpenAPI-aligned error handling middleware
func errorHandler() chi.Middleware {
    return func(next chi.Handler) chi.Handler {
        return func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if err := recover(); err != nil {
                    // Panic recovery for OpenAPI documentation
                    http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
                }
            }()
            
            rw := &responseWriter{ResponseWriter: w, Status: http.StatusOK}
            
            statusCode, err := next.ServeHTTP(rw, r)
            
            if statusCode >= 400 {
                var resp ErrorResponse
                
                // Map HTTP status to error code
                switch statusCode {
                case http.StatusBadRequest:
                    // Validation errors
                    if strings.Contains(err.Error(), "invalid") {
                        resp = ErrorResponse{
                            Code:      "VALIDATION_ERROR",
                            Message:   err.Error(),
                            Details:   errDetails(err),
                            RequestID: requestIDFromContext(r.Context()),
                        }
                    }
                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(statusCode)
                    json.NewEncoder(w).Encode(resp)
                    return
                case http.StatusUnauthorized:
                    resp = ErrorResponse{
                        Code:      "UNAUTHORIZED",
                        Message:   "Authentication required",
                        RequestID: requestIDFromContext(r.Context()),
                    }
                case http.StatusNotFound:
                    resp = ErrorResponse{
                        Code:      "RESOURCE_NOT_FOUND",
                        Message:   err.Error(),
                        RequestID: requestIDFromContext(r.Context()),
                    }
                }
                
                // Log errors for monitoring
                if err != nil {
                    logError(ctx, err, "Error occurred while handling request")
                }
            } else if statusCode == 0 {
                // No error, no content
                return
            }
            
            // 200+ responses don't need error body
            w.WriteHeader(statusCode)
            if body := errBody(err); body != "" {
                w.Write([]byte(body))
            }
        }
    }
}
```

### 4.4 Admin Route Specific Error Patterns

```go
// Admin route error handling
func adminResponseHandler(w http.ResponseWriter, err error) {
    var resp ErrorResponse
    
    switch {
    case errors.Is(err, ErrUnauthorized):
        w.WriteHeader(http.StatusUnauthorized)
        resp = ErrorResponse{
            Code:    "UNAUTHORIZED",
            Message: "Admin access required",
        }
    case errors.Is(err, ErrForbidden):
        w.WriteHeader(http.StatusForbidden)
        resp = ErrorResponse{
            Code:    "FORBIDDEN",
            Message: "Insufficient permissions",
        }
    case errors.Is(err, ErrNotFound):
        w.WriteHeader(http.StatusNotFound)
        resp = ErrorResponse{
            Code:    "NOT_FOUND",
            Message: "Resource not found",
        }
    case err != nil:
        w.WriteHeader(http.StatusInternalServerError)
        resp = ErrorResponse{
            Code:      "INTERNAL_ERROR",
            Message:  err.Error(),
            Details:   detailsFromErr(err),
        }
    default:
        w.WriteHeader(http.StatusOK)
        return
    }
    
    // Set CORS headers for all responses
    SetCORSHeaders(w)
    
    json.NewEncoder(w).Encode(resp)
}
```

---

## 5. Go 1.26+ Features Impact

### 5.1 Available Features (1.26+)

```go
// Feature 1: Generics improvements for type safety
// More flexible type parameters for reusable patterns
func cacheWithConfig[T any](config Config[T]) T {
    return config.Get()
}

// Feature 2: New collection APIs (when available)
// map[string]int64 -> maps.Keys(), maps.Values() (hypothetical)

// Feature 3: Better error wrapping
// New error wrapping primitives when released

// Feature 4: Improved compile-time checks
// Better support for build constraints

// Feature 5: Go modules improvements
// Better dep management

// Example: Using newer generics for type-safe caches
type AvailabilityCache[K any] struct {
    client redis.UniversalClient
}

func (c *AvailabilityCache[K]) Get(key K) (*Availability, error) {
    // Type-safe cache operations
    return c.cache.Get(key)
}
```

### 5.2 Impact on Implementation

| Feature | Implementation Impact | Confidence Improvement |
|---------|----------------------|------------------------|
| Better generics | Type-safe cache operations | ✓ Higher confidence |
| Improved docs | Better API generation | ✓ Easier OpenAPI docs |
| Better error types | More precise error mapping | ✓ Better OpenAPI |
| Collection APIs | Simplified result handling | ✓ Cleaner code |
| Type inference | More concise API definitions | ✓ Better readability |

---

## 6. Latest Go Ecosystem Patterns for API Documentation

### 6.1 OpenAPI Generation Tools

```go
// Pattern 1: Go-OpenAPI-Gen with chi
// https://github.com/danielgtaylor/openapi-gen

// Pattern 2: github.com/go-chi/cors with openapi
// Pattern 3: github.com/go-swagger/go-swagger

// Example using modern patterns:
func setupDocumentation(ctx context.Context) error {
    // Use chi-openapi for automatic route-to-docs mapping
    docs, err := generateDocsFromRoutes(r)
    if err != nil {
        return err
    }
    
    // Register swagger endpoints
    r.Get("/swagger", swagger.Handler.HTTPHandler)
    r.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(docs)
    })
    
    return nil
}
```

### 6.2 Schema Definition Standards

```go
// Use Go struct tags that map to OpenAPI
type Availability struct {
    ID           string `json:"id" doc:"Unique identifier"`
    Name         string `json:"name" doc:"Service name"`
    AvailableAt  time.Time `json:"available_at" doc:"When availability begins"`
    ExpiresAt    time.Time `json:"expires_at" doc:"Computed expiration"`
    Description  string `json:"description" doc:"Description of service"`
}

// Pattern: Use `bson` tags for database mapping
// Pattern: Use `json` tags for API mapping
// Pattern: Use custom `x-` tags for OpenAPI extensions

type AvailabilityResponse struct {
    Success bool          `json:"success"`
    Data    *Availability `json:"data"`
    Errors  []string      `json:"errors,omitempty"`
}
```

### 6.3 Common Documentation Patterns

```go
// Pattern 1: Request/Response examples in comments
// Pattern 2: Versioned schemas with x-extensions
// Pattern 3: Operation IDs for OpenAPI
// Pattern 4: Component schemas for reusability
// Pattern 5: Tag-based route groups

// Example tag-based routing
func routes() chi.Router {
    r := chi.NewRouter()
    
    // Admin tags
    admin := r.Group("/admin")
    admin.Use(adminAuth)
    
    // Use chi.Middleware for OpenAPI tracing
    admin.Use(tracing.Middleware("admin"))
    
    // Register routes with operation IDs
    admin.Post("/availability", 
        func(w http.ResponseWriter, r *http.Request) {
            createAvailability(w, r)
        },
        "CreateAvailability", // Operation ID
        "admin availability create", // Tag
    )
    
    return r
}
```

### 6.4 Community Best Practices (2024)

| Pattern | Recommended Approach |
|---------|---------------------|
| Versioning | URL path: `/v1/` |
| Error formatting | Standardized JSON |
| Authentication | Bearer in header |
| Pagination | Offset/limit query params |
| Filtering | Query params with operators |
| Filtering | `?status=available` (equals) |
| Filtering | `?status=available,unavailable` (multiple) |
| Sorting | `?sort=-created_at` (desc) |
| Search | `?q=` with filters |

---

## 7. Implementation Confidence Patterns

### 7.1 Admin Routes Documentation Checklist

- [ ] All admin routes use consistent tagging pattern
- [ ] Route groups separate by resource type (`/admin/sites`, `/admin/services`)
- [ ] Error responses are consistently structured
- [ ] OpenAPI schemas have example values
- [ ] All required parameters are documented
- [ ] Versioning strategy is clear (`/v1/`)
- [ ] Authentication headers are consistent
- [ ] Rate limiting headers are present
- [ ] CORS headers are set for all admin routes
- [ ] Request/response examples are comprehensive

### 7.2 Cache-First Pattern Checklist

- [ ] Cache-first strategy is used for availability reads
- [ ] TTL is calculated dynamically per service type
- [ ] Database queries include cache warm-up after admin actions
- [ ] Cache invalidation happens after admin mutations
- [ ] Redis errors don't break admin requests
- [ ] Cache misses fallback to database with graceful error
- [ ] Bitmap used appropriately (membership vs object storage)
- [ ] Cache stats monitored for performance
- [ ] Write operations go through cache (for consistency)

### 7.3 Code Examples for Implementation

```go
// Admin route with full documentation
func adminCreateAvailability(w http.ResponseWriter, r *http.Request) {
    // 1. Extract and validate
    var req AdminAvailabilityRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{
            Code: "VALIDATION_ERROR",
            Message: "Invalid request body",
        })
        return
    }
    
    // 2. Check permissions
    siteID := r.Context().Value(siteIDKey).(string)
    if !isAdminAllowed(siteID) {
        w.WriteHeader(http.StatusForbidden)
        json.NewEncoder(w).Encode(ErrorResponse{
            Code: "FORBIDDEN",
            Message: "You don't have permission to modify this site",
        })
        return
    }
    
    // 3. Create availability
    availability, err := createAvailability(r.Context(), siteID, req)
    if err != nil {
        if errors.Is(err, ErrResourceInUse) {
            w.WriteHeader(http.StatusConflict)
        } else {
            w.WriteHeader(http.StatusInternalServerError)
        }
        json.NewEncoder(w).Encode(ErrorResponse{
            Code:      err.Error(),
            Message:   err.Error(),
        })
        return
    }
    
    // 4. Cache-write immediately
    cache.Set(r.Context(), key, encoded, availability.ExpiresAt.Sub(time.Now()))
    
    // 5. Return success
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(AvailabilityResponse{
        Success: true,
        Data:    availability,
    })
}
```

---

## 8. Conclusion

### Key Takeaways

1. **Route Organization**: Use chi router groups with consistent tagging for OpenAPI clarity
2. **Schema Mapping**: Go struct JSON tags directly map to OpenAPI responses
3. **Cache-First**: Always try cache, then DB, with graceful fallbacks
4. **Error Handling**: Standardize errors for consistent OpenAPI responses
5. **Go 1.26+**: Leverage improved generics and error handling
6. **Documentation**: Use Go struct tags + comments for auto-generation

### Recommended Pattern Combos

| Use Case | Pattern |
|----------|---------|
| Admin routes | Cache-first + immediate cache write + consistent error codes |
| Public endpoints | Cache-first with adaptive TTL + error fallback |
| Bitmap operations | Membership queries (bitmap) vs object storage (keys) |
| Documentation | Go struct tags + chi-openapi generation |

---

*Last updated: Based on Go 1.26+ and latest OpenAPI standards*
*Research scope: Admin routes, cache-first patterns, OpenAPI integration*
