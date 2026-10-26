# RateGuard - Distributed API Rate Limiter

**Go · gRPC · Protocol Buffers · DynamoDB Local · HTML/CSS/JavaScript**

A Go-based API rate limiter that uses the **Token Bucket algorithm** to enforce per-user, per-endpoint quotas. The service exposes a **gRPC unary RPC**, stores shared bucket state in **DynamoDB**, and uses **conditional writes** to protect token updates under concurrent traffic. A lightweight browser dashboard lets users generate clients, send requests, and visualize allow/deny decisions.

> **Project scope:** Local development with DynamoDB Local. Multi-server operation is a validation scenario, not the primary purpose. Performance numbers and production readiness are not claimed.

## 1. Problem & Solution

- **Problem:** Bursty or excessive API traffic can overwhelm backend services; concurrent requests must not consume the same allowance twice.
- **Solution:** Check each request against a token bucket identified by **user ID + endpoint**, then return an allow/deny decision before the protected operation runs.
- **Default policy:** **5-token capacity**, **1 token refilled per second**, **1 token consumed per allowed request**.
- **Response:** Whether the request is allowed, the number of remaining whole tokens, and a recommended retry delay in milliseconds.

## 2. Request Flow & Architecture

```text
Browser Dashboard (HTML / CSS / JavaScript)
        │ POST /api/check-limit (HTTP/JSON)
        ▼
Go HTTP Gateway :8080
        │ gRPC CheckLimit()
        ▼
Go Rate Limiter Service :50051
        │ Token Bucket + conditional write
        ▼
DynamoDB Local :8000 (RateLimits table)
        │
        └── Shared token balance, last-refill time, version
```

- **Go gRPC service:** Validates requests, calculates token availability, and returns structured decisions.
- **HTTP gateway:** Bridges browser `fetch()` requests to the generated Go gRPC client.
- **DynamoDB Local:** Persists bucket state outside the Go process, allowing server restarts without resetting the stored balance.
- **Optional distributed test:** Run a second gRPC instance on `:50052`; the gateway routes calls round-robin to both, using the same DynamoDB table.

## 3. Core Rate-Limiting Logic

- **Token Bucket:** Refill tokens based on elapsed time, capped at five; subtract one token for an allowed request; deny requests when fewer than one token is available.
- **Precision:** Store `tokens_milli` as an integer (`5000` = 5 tokens), avoiding floating-point token accounting; use millisecond timestamps for refill calculations.
- **Isolation:** Create independent buckets for each **user + endpoint** combination.
- **Retry guidance:** Return `retry_after_ms` when denied instead of requiring clients to guess when to retry.
- **Example:** A fresh bucket receiving ten rapid requests can allow five and deny five, assuming no full token refills and all checks complete successfully.

## 4. Data Consistency & Concurrency

- **DynamoDB record:** `PK`, `tokens_milli`, `last_refill_ms`, `version`.
- **First request:** Conditional `PutItem` creates a bucket only when its key does not exist.
- **Subsequent requests:** Version-checked `UpdateItem` applies token changes only if the stored version still matches the value previously read.
- **Conflict resolution:** On a conditional-write conflict, re-read the latest bucket state and recalculate the decision; never reuse a stale token balance.
- **Why it matters:** Multiple concurrent clients or Go server instances must compete for **one shared allowance**, rather than each receiving a separate quota.
- **Current limitation:** High contention can exhaust the configured retry attempts; retry backoff and clock-skew handling are future improvements.

## 5. gRPC Contract & Failure Handling

```protobuf
service RateLimiterService {
  rpc CheckLimit(CheckLimitRequest) returns (CheckLimitResponse);
}

message CheckLimitRequest {
  string user_id = 1;
  string endpoint = 2;
}

message CheckLimitResponse {
  bool allowed = 1;
  int32 remaining_tokens = 2;
  int64 retry_after_ms = 3;
}
```

- **Communication:** Protocol Buffers over a unary gRPC method (`1 request → 1 response`).
- **Denied limit:** Normal response with `allowed=false`; denial is a valid business decision, not a gRPC transport failure.
- **Invalid input:** Missing `user_id` or `endpoint` returns `INVALID_ARGUMENT`.
- **Backend failure:** DynamoDB errors or exhausted update retries are surfaced as service errors rather than silently allowing requests; the HTTP gateway reports an error to the browser.

## 6. Interactive Dashboard & Testing

- **Dynamic users:** Enter a user count to generate separate user cards and request buttons.
- **Single requests:** Click **Send Request** to issue a real HTTP → gRPC → DynamoDB rate-limit check.
- **Concurrent bursts:** Click **Send 10 Concurrent** to start ten requests using JavaScript `Promise.allSettled()`.
- **Live feedback:** Display allowed/denied counts, errors, per-user attempts, recent request logs, and last-reported token snapshots.
- **Shared-state experiment:** Use two Go server processes with round-robin gateway routing to check that a single bucket is respected across instances.
- **Verification cases:** Fresh-bucket burst, independent user quotas, token refill, server restart, conditional-write conflicts, and DynamoDB outage behavior.
- **Measurement caveat:** The UI reports responses and snapshots; it is not an authoritative live bucket monitor or a throughput benchmark.

## 7. Project Layout & Local Setup

```text
grpc-rate-limiter/
├── proto/ratelimiter.proto       # gRPC API contract
├── generated/                    # Generated Go Protobuf/gRPC code
├── server/
│   ├── main.go                   # gRPC service entry point
│   ├── token_bucket.go           # Refill and allow/deny algorithm
│   └── store.go                  # DynamoDB conditional operations
├── gateway/main.go              # HTTP-to-gRPC bridge, optional round robin
├── client/main.go               # CLI gRPC client
├── frontend/
│   ├── index.html
│   ├── style.css
│   └── script.js
├── docker-compose.yml           # DynamoDB Local
├── go.mod
└── go.sum
```

- **Prerequisites:** Go, Docker Compose, AWS CLI; `protoc` and Go plugins if regenerating protobuf code.
- **Start DynamoDB Local** from the repository root:
  ```bash
  docker compose up -d
  ```
- **Create the table once** if it does not already exist:
  ```bash
  export AWS_ACCESS_KEY_ID=localtest
  export AWS_SECRET_ACCESS_KEY=localtest
  export AWS_DEFAULT_REGION=us-west-2

  aws dynamodb create-table \
    --table-name RateLimits \
    --attribute-definitions AttributeName=PK,AttributeType=S \
    --key-schema AttributeName=PK,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --endpoint-url http://localhost:8000
  ```
- **Run the rate limiter** in a separate terminal:
  ```bash
  go run ./server
  ```
- **Run the gateway and dashboard** in another terminal:
  ```bash
  go run ./gateway
  ```
- **Open:** `http://127.0.0.1:8080`.
- **Two-server demonstration:** Start `GRPC_PORT=50051 go run ./server` and `GRPC_PORT=50052 go run ./server` separately; use the gateway implementation configured for both backends.
- **Configuration note:** Use your actual GitHub module path in `go.mod`, generated protobuf `go_package`, and Go imports. DynamoDB Local runs on port `8000` and does not create an AWS-hosted table.

## 8. Engineering Highlights & Next Steps

- **Backend engineering:** Go services, gRPC and Protocol Buffers, HTTP-to-gRPC integration, request validation, and structured responses.
- **Algorithms:** Time-based token replenishment, burst control, per-user limits, and retry-time calculation.
- **Consistency:** DynamoDB conditional writes, version-based optimistic concurrency, and shared state across independent server processes.
- **Testing:** Browser-driven concurrent bursts, decision analytics, failure scenarios, and optional multi-instance verification.
- **Trade-offs:** No automatic failover, bounded conflict retries, and potential clock skew between instances; DynamoDB Local is for development, not production benchmarking.
- **Next improvements:** Automated concurrency tests, exponential backoff with jitter, health-aware backend routing, configurable quotas, and metrics for latency, throughput, and contention.

**Resume-ready description (use after validating the behavior):**

> Built a Go-based gRPC API rate limiter using a token-bucket algorithm and DynamoDB-backed optimistic concurrency control to enforce per-user request quotas; developed an HTML/JavaScript dashboard for burst testing and validated shared limits across multiple service instances.
