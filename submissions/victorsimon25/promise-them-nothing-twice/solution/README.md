# RelayAPI Distributed Rate Limiter & Verification Harness

This directory contains the thin vertical slice submission for the RelayAPI distributed rate-limiting prototype.

The rate limiter uses a **Sliding Window Log** algorithm implemented atomically in Redis Sorted Sets (`ZSET`) via Lua scripting. This guarantees a strict hard cap over any rolling 60-second window across a distributed topology, preventing boundary burst failures.

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (v20+ recommended)
- [Go](https://go.dev/dl/) (v1.24+ recommended)

---

## 1. Starting the Rate Limiter Services

To start the multi-node backend topology (3 application node replicas behind NGINX round-robin load balancer, coordinated via Redis):

```bash
docker-compose up --build -d
```

Verify that all services started successfully:
```bash
docker ps
```
You should see `rlt-lb` (running on port `8080`), `rlt-redis`, `rlt-app-1`, `rlt-app-2`, and `rlt-app-3` up and healthy.

---

## 2. Running the Verification Harness

A Go CLI load-generator is provided under `harness/` that runs concurrent request bursts against the load balancer, validating limits and fake clock window behaviors.

To execute the suite while services are online:
```bash
go run harness/main.go
```

### Scenario Assertions:
1. **Starter Client (60 RPM Limit)**: Sends 75 rapid requests. Asserts exactly 60 succeed and 15 fail with HTTP 429.
2. **Enterprise Client Outside Window (300 RPM Limit)**: Sends 320 requests simulating 10:00 UTC. Asserts exactly 300 succeed and 20 fail with HTTP 429.
3. **Enterprise Client Inside Window (1500 RPM Limit Proof)**: Sends 1700 requests simulating 03:00 UTC. Asserts exactly 1500 succeed and 200 fail (proving the 1500 override cap is active).
4. **Priya's Demo (Per-Customer Isolation)**: Hammers three distinct clients concurrently. Asserts `priya-1` and `priya-2` hit their 100 RPM quotas (exactly 100 succeed, 20 get blocked), while `priya-3` continues to receive 100% of its requests (80/80 OK), proving complete tenant isolation.
5. **Sliding Window Log Boundary Test**: Sends two consecutive bursts of 300 requests to `customer-growth` (limit 300). Asserts that the first burst succeeds (300/300) and the immediate second burst is blocked (300/300 429s), proving boundary bursting is blocked.

---

## 3. Verifying Redis Outage (Fail-Closed)

To verify that the rate limiter correctly fails closed (rejects traffic rather than admitting over-quota requests when state is unavailable):

1. Stop the Redis container:
   ```bash
   docker stop rlt-redis
   ```

2. Re-run the verification harness:
   ```bash
   go run harness/main.go
   ```
   *The harness will detect Redis is offline and execute Scenario 6, asserting that all 10 requests return `503 Service Unavailable`.*

3. Restart Redis to recover the service state:
   ```bash
   docker start rlt-redis
   ```

---

## 4. Manual Verification (via Curl)

You can also run requests manually against the load balancer endpoint on `http://localhost:8080/api/v1/ping`.

#### Valid Request:
```bash
curl.exe -i -H "X-Customer-Id: customer-starter" http://localhost:8080/api/v1/ping
```

#### Invalid Request (Missing ID):
```bash
curl.exe -i http://localhost:8080/api/v1/ping
```

#### Mocking Time Window (Requires `ALLOW_MOCK_TIME=true` env var):
```bash
curl.exe -i -H "X-Customer-Id: customer-enterprise" -H "X-Fake-Time: 2026-08-16T03:00:00Z" http://localhost:8080/api/v1/ping
```
