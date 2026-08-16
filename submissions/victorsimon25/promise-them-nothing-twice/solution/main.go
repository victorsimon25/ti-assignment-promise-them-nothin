package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// PingResponse represents the standard response body for the ping endpoint.
type PingResponse struct {
	Status     string `json:"status"`
	CustomerID string `json:"customer_id"`
	NodeID     string `json:"node_id"`
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

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ping", handlePing(nodeID))

	// Wrap routing with a middleware to validate customer identity.
	handler := validateCustomerHeader(mux)

	log.Printf("Starting server on port %s (Node: %s)...", port, nodeID)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
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
func handlePing(nodeID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get("X-Customer-Id")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(PingResponse{
			Status:     "pong",
			CustomerID: customerID,
			NodeID:     nodeID,
		})
	}
}
