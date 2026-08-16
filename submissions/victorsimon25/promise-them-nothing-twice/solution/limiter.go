package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Lua script for Sliding Window Log using Redis ZSET
const rateLimitLua = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local uuid = ARGV[2]

local time = redis.call('TIME')
local now_seconds = tonumber(time[1])
local now_microseconds = tonumber(time[2])
local now_ms = (now_seconds * 1000) + math.floor(now_microseconds / 1000)
local window_boundary_ms = now_ms - 60000

-- Remove elements older than 60 seconds
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_boundary_ms)

-- Count remaining elements
local current_count = redis.call('ZCARD', key)

if current_count < limit then
    -- Add the new request
    redis.call('ZADD', key, now_ms, uuid)
    redis.call('PEXPIRE', key, 60000)
    return {1, 0}
else
    -- Compute retry-after based on the oldest element in the window
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry_after = 1
    if oldest and #oldest >= 2 then
        local oldest_ms = tonumber(oldest[2])
        local ms_to_wait = (oldest_ms + 60000) - now_ms
        retry_after = math.max(1, math.ceil(ms_to_wait / 1000))
    end
    return {0, retry_after}
end
`

// Limiter coordinates Redis and customer configs to apply rate limits.
type Limiter struct {
	rdb        *redis.Client
	clock      Clock
	limitsHash map[string]int // maps customerID -> default limit (RPM)
}

// NewLimiter creates a new Limiter instance.
func NewLimiter(rdb *redis.Client, clock Clock) *Limiter {
	return &Limiter{
		rdb:   rdb,
		clock: clock,
		limitsHash: map[string]int{
			"customer-starter":    60,
			"customer-growth":     300,
			"customer-enterprise": 300, // Northwind Enterprise default
		},
	}
}

// getLimitForCustomer returns the current RPM limit for a customer.
func (l *Limiter) getLimitForCustomer(customerID string, now time.Time) int {
	if customerID == "customer-enterprise" {
		hour := now.Hour()
		if hour == 2 || hour == 3 {
			// Auditable override action
			log.Printf("[AUDIT] Customer %s is operating within the nightly batch window (%02d:%02d UTC). Enforcing higher limit of 1500 RPM.", customerID, now.Hour(), now.Minute())
			return 1500
		}
	}

	limit, exists := l.limitsHash[customerID]
	if !exists {
		// Starter default fallback
		return 60
	}
	return limit
}

// LimitMiddleware returns a middleware that rate-limits incoming HTTP requests.
func (l *Limiter) LimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get("X-Customer-Id")
		if customerID == "" {
			// Should be caught by validateCustomerHeader middleware, but fail safe.
			next.ServeHTTP(w, r)
			return
		}

		if l.rdb == nil {
			// Fail-closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Rate limiter state unavailable (Redis down)"})
			return
		}

		// Resolve limit
		now := l.clock.Now(r)
		limit := l.getLimitForCustomer(customerID, now)

		// Call Lua script
		key := fmt.Sprintf("rate_limit:%s", customerID)
		requestUUID := uuid.New().String()

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		res, err := l.rdb.Eval(ctx, rateLimitLua, []string{key}, limit, requestUUID).Result()
		if err != nil {
			log.Printf("Redis error during rate limiting for customer %s: %v", customerID, err)
			// Fail-closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Rate limiter state unavailable (Redis error)"})
			return
		}

		// Lua returns a list of two values: allowed (1 or 0), and retry_after (int seconds)
		resSlice, ok := res.([]interface{})
		if !ok || len(resSlice) < 2 {
			log.Printf("Unexpected script response format: %v", res)
			// Fail-closed
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "Rate limiter internal error"})
			return
		}

		allowed := resSlice[0].(int64) == 1
		retryAfter := resSlice[1].(int64)

		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("rate limit exceeded. Try again in %d seconds", retryAfter)})
			return
		}

		next.ServeHTTP(w, r)
	})
}
