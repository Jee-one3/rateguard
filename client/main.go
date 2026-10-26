package main

import (
	"context"
	"fmt"
	"log"
	"time"

	ratepb "example.com/grpc-rate-limiter/generated"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := ratepb.NewRateLimiterServiceClient(conn)

	// Seven rapid requests from the same user.
	for i := 1; i <= 7; i++ {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)

		response, err := client.CheckLimit(
			ctx,
			&ratepb.CheckLimitRequest{
				UserId:   "user-101",
				Endpoint: "/products",
			},
		)
		cancel()

		if err != nil {
			log.Printf("Request %d failed: %v", i, err)
			continue
		}

		fmt.Printf(
			"Request %d | Allowed: %t | Remaining: %d | Retry: %dms\n",
			i,
			response.Allowed,
			response.RemainingTokens,
			response.RetryAfterMs,
		)
	}
}
