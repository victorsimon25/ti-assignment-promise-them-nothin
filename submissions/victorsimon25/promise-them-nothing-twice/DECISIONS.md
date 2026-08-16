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
- **Clock:** Injectable via env var so the harness can simulate the batch window without waiting for real UTC.
- **Deferred:** Rate-limit algorithm (sliding vs fixed window vs zero-burst token bucket), exact Northwind batch RPM in config, Redis key schema, config file format.

## Verification

<!-- What your harness proves and what it does not. -->

- **Proves:** Per-customer hard cap at quota boundaries; 429 + `Retry-After` when over limit; multi-node behavior via LB; Northwind no-429 in-window when configured above batch peak; time-window switching via fake clock.
- **Does not prove:** Post-window 429s when limit drops at 04:00 UTC (explicitly out of scope). Production-scale Redis failure modes or retry-storm behavior.

## If I had four more hours

- Pick and document the rate-limit algorithm with boundary tests; set Northwind batch enforcement RPM with headroom above 1200; demo or harness Redis-down 503; taper or grace at window end; `Retry-After` semantics and enterprise counting paragraph for security reviews.
