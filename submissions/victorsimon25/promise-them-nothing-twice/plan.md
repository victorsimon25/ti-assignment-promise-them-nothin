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

### Level 2: Docker Environment (Compose Setup)
* **Goal**: Package the Go app into a Docker container and set up Redis in `docker-compose.yml`.
* **Steps**:
  1. Write a `Dockerfile` for the Go application.
  2. Write `docker-compose.yml` with Redis and a single app instance.
  3. Set up Redis connection initialization in the Go app.
* **Verification**:
  - Run `docker-compose up --build -d app-1 redis`.
  - Check container logs to verify the app node successfully pings Redis on startup.

### Level 3: Redis-Based Sliding Window Limiter (Single Instance)
* **Goal**: Implement the core rate limiter logic using Redis sorted sets (ZSET) inside a Lua script.
* **Steps**:
  1. Write rate limiter middleware in Go.
  2. Implement Lua script for Sliding Window Log:
     - Remove old entries beyond 60s window (`ZREMRANGEBYSCORE`).
     - Check current count (`ZCARD`).
     - If count < limit, add current request (`ZADD`) and update expiry (`PEXPIRE`).
     - If limit exceeded, return rejection and compute `Retry-After` using oldest entry timestamp.
* **Verification**:
  - Configure `customer-starter` with a low limit of 5 RPM.
  - Send 6 rapid requests. Ensure first 5 succeed (`200 OK`) and the 6th fails (`429 Too Many Requests`) with a correct `Retry-After` header.

### Level 4: Multi-Node Load Balanced Topology
* **Goal**: Scale up to 3 app nodes behind a round-robin load balancer.
* **Steps**:
  1. Add Caddy, NGINX, or HAProxy to `docker-compose.yml` routing to `app-1`, `app-2`, and `app-3`.
  2. Update Go app responses to include the server host/node identifier for verification.
* **Verification**:
  - Send rapid requests to the load balancer port.
  - Confirm requests are distributed to all 3 nodes (verify logs/headers), but the collective traffic is rate limited globally using Redis.

### Level 5: Time-Window Switching & Fake Clock
* **Goal**: Add logic to increase Northwind's limit to the batch rate (1500 RPM instead of 300 RPM) during 02:00-04:00 UTC using a mockable clock.
* **Steps**:
  1. Implement a clock provider that accepts an `X-Fake-Time` header for manual/test time overrides, but only evaluates this header if the container's environment has `ALLOW_MOCK_TIME=true` configured.
  2. Add time window check logic: check if UTC hour is 02 or 03.
  3. Dynamically resolve rate limits based on client identity and window logic.
* **Verification**:
  - Run with `ALLOW_MOCK_TIME=true`:
    - Send requests simulating **05:00 UTC** (outside window) using `X-Fake-Time: 2026-08-16T05:00:00Z`. Verify the 6th request gets `429`.
    - Send requests simulating **03:00 UTC** (inside window) using `X-Fake-Time: 2026-08-16T03:00:00Z`. Verify that at least 15 requests succeed without getting blocked.
  - Run with `ALLOW_MOCK_TIME=false` or unset:
    - Send requests simulating **03:00 UTC** (inside window) using `X-Fake-Time: 2026-08-16T03:00:00Z`. Verify that the 6th request gets `429` (header is ignored).

### Level 6: Fail-Closed Mechanics
* **Goal**: Ensure the rate limiter fails closed if Redis is down, preserving the hard cap on traffic.
* **Steps**:
  1. Wrap Redis calls in error checking middleware.
  2. If Redis ping or Lua execution returns an error/timeout, return `503 Service Unavailable`.
* **Verification**:
  - Run `docker-compose stop redis`.
  - Send a request to the server/load balancer. Ensure it returns `503 Service Unavailable`.
  - Start Redis again and verify normal traffic resumes.

### Level 7: Verification Harness Implementation
* **Goal**: Build a CLI harness that drives load, validates correctness, and prints clear compliance results.
* **Steps**:
  1. Create a Go command line harness.
  2. Implement simulated traffic loads for different customer classes, including time-based shifts.
  3. Validate responses (200, 429, 503) and log stats.
* **Verification**:
  - Run harness and verify it prints a clean test summary proving the core requirements.
