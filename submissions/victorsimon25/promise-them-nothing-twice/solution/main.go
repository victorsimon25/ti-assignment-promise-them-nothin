package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// PingResponse represents the standard response body for the ping endpoint.
type PingResponse struct {
	Status     string `json:"status"`
	CustomerID string `json:"customer_id"`
	NodeID     string `json:"node_id"`
	RedisPing  string `json:"redis_ping"`
}

// ErrorResponse represents the standard error response body.
type ErrorResponse struct {
	Error string `json:"error"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "local-node"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	log.Printf("Connecting to Redis at %s...", redisURL)
	rdb, err := initRedis(redisURL)
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v. Continuing in degraded/unhealthy state...", err)
	} else {
		log.Println("Successfully connected to Redis!")
		defer rdb.Close()
	}

	clock := RealClock{}
	limiter := NewLimiter(rdb, clock)

	mux := http.NewServeMux()

	// Sliding Window endpoint
	slidingHandler := nodeHeaderMiddleware(nodeID, validateCustomerHeader(limiter.LimitMiddleware(handlePing(nodeID, rdb))))
	mux.Handle("/api/v1/ping", slidingHandler)

	// Naive Fixed Window endpoint
	fixedHandler := nodeHeaderMiddleware(nodeID, validateCustomerHeader(limiter.FixedLimitMiddleware(handlePing(nodeID, rdb))))
	mux.Handle("/api/v1/ping-fixed", fixedHandler)

	log.Printf("Starting server on port %s (Node: %s)...", port, nodeID)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// initRedis initializes and tests connection to Redis.
func initRedis(addr string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}

// nodeHeaderMiddleware injects the X-Node-Id header into all HTTP responses.
func nodeHeaderMiddleware(nodeID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Node-Id", nodeID)
		next.ServeHTTP(w, r)
	})
}

// validateCustomerHeader is a middleware verifying the X-Customer-Id header is present.
func validateCustomerHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get("X-Customer-Id")
		if customerID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "missing X-Customer-Id header"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handlePing returns the handler function for /api/v1/ping.
func handlePing(nodeID string, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get("X-Customer-Id")

		redisStatus := "disconnected"
		if rdb != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := rdb.Ping(ctx).Err(); err == nil {
				redisStatus = "connected"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(PingResponse{
			Status:     "pong",
			CustomerID: customerID,
			NodeID:     nodeID,
			RedisPing:  redisStatus,
		})
	}
}
