# Decisions — Promise Them Nothing Twice

<!-- Candidates: copy this file to submissions/<your-github-username>/promise-them-nothing-twice/DECISIONS.md and replace the prompts below. Keep it to one page. -->

## Conflict resolution

<!-- What you decided, what you rejected, and why. -->

- **Decided:** Hard enforcement on the **configured** limit — never admit traffic above it. Northwind gets an auditable, config-driven higher limit during **02:00–04:00 UTC** (not a hardcoded customer bypass). Other customers stay on tier defaults.
- **Rejected:** Literal “never exceed contracted quota” for Northwind during batch (300 RPM vs ~800–1200 RPM sustained, ~2.7×–4× over contract). Fail-open when Redis is down. Marcus-style invisible errors via quota overages.
- **Tradeoff accepted:** Northwind is a documented commercial exception; same-tier fairness does not apply during the batch window.

## Technical design

<!-- Algorithm, coordination across nodes, and the tradeoffs you accepted. -->

- **Stack:** Go, bare `net/http`, Docker Compose — 3 app replicas, load balancer, Redis for global counts across nodes.
- **Redis unavailable:** Return **503** (fail closed). Preserves hard cap; avoids the deprecated limiter’s over-admit failure mode.
- **Algorithm:** Sliding Window Log via Redis Sorted Sets (`ZSET`). Guarantees a strict hard cap over any rolling 60-second window, preventing boundary-burst failures (e.g., 600 requests in 2s) seen in previous limiters.
- **Clock Decoupling:** The Go app uses an injectable fake clock to evaluate the 02:00–04:00 UTC business-logic window. The Redis Lua rate-limit script uses a centralized `redis.call('TIME')` to perfectly synchronize counting across all nodes and prevent clock-drift over-admission.
- **ZSET Mechanics:** To prevent concurrent request collisions (undercounting), the ZSET score is the Redis timestamp and the member is a Go-generated UUID. Keys are aggressively expired (`PEXPIRE`) after 60s.
- **Rejection & Retry-After:** Rejected requests are *not* added to the ZSET to avoid a penalty-box effect for aggressive clients. `Retry-After` is calculated by finding the oldest successful request in the 60s window and returning the integer seconds until it expires.
- **Deferred:** Exact Northwind batch RPM in config, config file format.

## Verification

<!-- What your harness proves and what it does not. -->

- **Proves:** Per-customer hard cap at quota boundaries; 429 + `Retry-After` when over limit; multi-node behavior via LB; Northwind no-429 in-window when configured above batch peak; time-window switching via fake clock.
- **Does not prove:** Post-window 429s when limit drops at 04:00 UTC (explicitly out of scope). Production-scale Redis failure modes or retry-storm behavior.

## If I had four more hours

- Implement boundary tests for the ZSET Sliding Window Log; set Northwind batch enforcement RPM with headroom above 1200; demo or harness Redis-down 503; taper or grace at window end.
