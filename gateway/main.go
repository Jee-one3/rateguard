
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	// Replace this with your actual Go module path.
	ratepb "example.com/grpc-rate-limiter/generated"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Request coming from the browser.
type CheckRequest struct {
	UserID   string `json:"user_id"`
	Endpoint string `json:"endpoint"`
}

// Response returned to the browser.
type CheckResponse struct {
	Allowed         bool  `json:"allowed"`
	RemainingTokens int32 `json:"remaining_tokens"`
	RetryAfterMs    int64 `json:"retry_after_ms"`
}

// Gateway holds our gRPC client.
type Gateway struct {
	client ratepb.RateLimiterServiceClient
}

func (g *Gateway) checkLimit(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("Content-Type", "application/json")

	// Parse incoming JSON.
	var req CheckRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Endpoint == "" {
		http.Error(
			w,
			`{"error":"user_id and endpoint are required"}`,
			http.StatusBadRequest,
		)
		return
	}

	// Give the gRPC call at most 5 seconds.
	ctx, cancel := context.WithTimeout(
		r.Context(),
		5*time.Second,
	)
	defer cancel()

	// Call the existing Go gRPC server.
	result, err := g.client.CheckLimit(
		ctx,
		&ratepb.CheckLimitRequest{
			UserId:   req.UserID,
			Endpoint: req.Endpoint,
		},
	)

	if err != nil {
		log.Printf("gRPC error: %v", err)

		code := status.Code(err)
		log.Printf("gRPC status: %s", code)

		http.Error(
			w,
			`{"error":"Rate limiter service unavailable"}`,
			http.StatusServiceUnavailable,
		)
		return
	}

	// Convert the gRPC response into JSON.
	response := CheckResponse{
		Allowed:         result.Allowed,
		RemainingTokens: result.RemainingTokens,
		RetryAfterMs:    result.RetryAfterMs,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("JSON response error: %v", err)
	}
}

func main() {
	// Connect to our existing gRPC service.
	conn, err := grpc.NewClient(
		"127.0.0.1:50051",
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	gateway := &Gateway{
		client: ratepb.NewRateLimiterServiceClient(conn),
	}

	mux := http.NewServeMux()

	// HTTP API route.
	mux.HandleFunc(
		"POST /api/check-limit",
		gateway.checkLimit,
	)

	// Serve HTML, CSS and JavaScript.
	mux.Handle(
		"/",
		http.FileServer(http.Dir("./frontend")),
	)

	log.Println("Dashboard: http://127.0.0.1:8080")

	server := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}
