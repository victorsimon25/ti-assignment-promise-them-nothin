# Plan — Distributed Rate Limiter & Verification Harness

This plan defines the step-by-step implementation phases for building the rate limiter system in Go, Docker Compose, and Redis, based on the design specified in [DECISIONS.md](./DECISIONS.md).

At each level, specific verification checks are provided to confirm correctness before moving to the next level.

---

## Prerequisites
- [x] Install Go (v1.23+ verified)
- [ ] Start Docker Desktop (ensure the Docker daemon is running)

---

## Implementation Levels

### Level 1: Foundation (Single Go HTTP Server) [COMPLETED]
* **Goal**: Scaffold the Go project and set up a basic, stateless HTTP server with customer authentication checks.
* **Steps**:
  - [x] Initialize the Go module in `solution/`.
  - [x] Create a basic HTTP server with a `/api/v1/ping` endpoint.
  - [x] Validate incoming requests contain the `X-Customer-Id` header (reject with `400 Bad Request` if missing).
* **Verification**:
  - [x] Run `go run main.go clock.go` locally.
  - [x] Send requests using curl and ensure missing header gets `400`, present header gets `200`.

### Level 2: Docker Environment (Compose Setup) [COMPLETED]
* **Goal**: Package the Go app into a Docker container and set up Redis in `docker-compose.yml`.
* **Steps**:
  - [x] Write a `Dockerfile` for the Go application.
  - [x] Write `docker-compose.yml` with Redis and a single app instance.
  - [x] Set up Redis connection initialization in the Go app.
* **Verification**:
  - [x] Run `docker-compose up --build -d app-1 redis`.
  - [x] Check container logs to verify the app node successfully pings Redis on startup.

### Level 3: Redis-Based Sliding Window Limiter (Single Instance) [COMPLETED]
* **Goal**: Implement the core rate limiter logic using Redis sorted sets (ZSET) inside a Lua script.
* **Steps**:
  - [x] Write rate limiter middleware in Go.
  - [x] Implement Lua script for Sliding Window Log (ZSET, TTL, and Retry-After).
* **Verification**:
  - [x] Configure `customer-starter` with a low limit of 5 RPM.
  - [x] Send 6 rapid requests. Ensure first 5 succeed (`200 OK`) and the 6th fails (`429 Too Many Requests`) with a correct `Retry-After` header.

### Level 4: Multi-Node Load Balanced Topology [COMPLETED]
* **Goal**: Scale up to 3 app nodes behind a round-robin load balancer.
* **Steps**:
  - [x] Add NGINX to `docker-compose.yml` routing to `app-1`, `app-2`, and `app-3`.
  - [x] Update Go app responses to include the server host/node identifier for verification.
* **Verification**:
  - [x] Send rapid requests to the load balancer port.
  - [x] Confirm requests are distributed to all 3 nodes (verify logs/headers), but the collective traffic is rate limited globally using Redis.

### Level 5: Time-Window Switching & Fake Clock [COMPLETED]
* **Goal**: Add logic to increase Northwind's limit to the batch rate (1500 RPM instead of 300 RPM) during 02:00-04:00 UTC using a mockable clock.
* **Steps**:
  - [x] Implement a clock provider that accepts an `X-Fake-Time` header, only if `ALLOW_MOCK_TIME=true` environment variable is configured.
  - [x] Add time window check logic checking if UTC hour is 02 or 03.
  - [x] Dynamically resolve rate limits based on client identity and window logic (Northwind / customer-enterprise limit increases to 1500 RPM in window).
* **Verification**:
  - [x] Run with `ALLOW_MOCK_TIME=true`:
    - [x] Send requests simulating **05:00 UTC** (outside window) using `X-Fake-Time: 2026-08-16T05:00:00Z`. Verify the 6th request gets `429` (limit 5).
    - [x] Send requests simulating **03:00 UTC** (inside window) using `X-Fake-Time: 2026-08-16T03:00:00Z`. Verify that at least 10 requests succeed without getting blocked (limit 10).
  - [x] Run with `ALLOW_MOCK_TIME=false`:
    - [x] Send requests simulating **03:00 UTC** (inside window) using `X-Fake-Time: 2026-08-16T03:00:00Z`. Verify that the 6th request gets `429` (header is ignored).

### Level 6: Fail-Closed Mechanics [COMPLETED]
* **Goal**: Ensure the rate limiter fails closed if Redis is down, preserving the hard cap on traffic.
* **Steps**:
  - [x] Wrap Redis calls in error checking middleware.
  - [x] If Redis ping or Lua execution returns an error/timeout, return `503 Service Unavailable`.
* **Verification**:
  - [x] Verify fail-closed behavior returns `503 Service Unavailable` on Redis failure.

### Level 7: Verification Harness Implementation
* **Goal**: Build a CLI harness that drives load, validates correctness, and prints clear compliance results.
* **Steps**:
  1. Create a Go command line harness.
  2. Implement simulated traffic loads for different customer classes, including time-based shifts.
  3. Validate responses (200, 429, 503) and log stats.
* **Verification**:
  - Run harness and verify it prints a clean test summary proving the core requirements.
