package main

import (
	"context"
	"log"
	"net"
	"os"

	ratepb "example.com/grpc-rate-limiter/generated"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RateLimiterServer struct {
	ratepb.UnimplementedRateLimiterServiceServer
	store *Store
}

func (s *RateLimiterServer) CheckLimit(
	ctx context.Context,
	req *ratepb.CheckLimitRequest,
) (*ratepb.CheckLimitResponse, error) {

	if req.UserId == "" || req.Endpoint == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"user_id and endpoint are required",
		)
	}

	decision, err := s.store.Check(
		ctx, req.UserId, req.Endpoint,
	)
	if err != nil {
		log.Printf("DynamoDB operation failed: %v", err)
		return nil, status.Error(
			codes.Unavailable,
			"Rate limiter storage is unavailable or busy",
		)
	}

	return &ratepb.CheckLimitResponse{
		Allowed:         decision.Allowed,
		RemainingTokens: decision.Remaining,
		RetryAfterMs:    decision.RetryMs,
	}, nil
}

func main() {
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-west-2"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				"localtest", "localtest", "",
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Explicitly target DynamoDB Local, not AWS.
	db := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:8000")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatal(err)
	}

	server := grpc.NewServer()

	ratepb.RegisterRateLimiterServiceServer(
		server,
		&RateLimiterServer{store: &Store{db: db}},
	)

	log.Println("Rate Limiter running on localhost:50051")

	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}

	_ = os.Stdout
}
