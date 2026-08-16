# Assignment understanding session
_Exported on 8/16/2026 at 15:23:47 GMT+5:30 from Cursor (3.16.17)_

---

**User**

i m going to wotk on an assignment and this repo conatains the details of the assignment, i have forked this into my git and will be building the solution under submissions /victorsimon25. before proceeding go through the files present. and in this session i m not looking to code rather to understand the problem so dont code anything. lemme know what the assignment wants and quote the imporant details. provide me options along with their tradeoffs with their explaination. Dont come up with soutions as of not i m looking to understand the aspects required first and then proceeed, dont hallucinate and ask me for clarifications 

---

**Cursor**

I'll explore the repository structure and assignment docs first so we can summarize requirements, key quotes, and design tradeoffs without proposing a solution yet.


I've read the repo. Here's a structured breakdown of what the assignment is asking for, the important quoted constraints, and the decision space — without proposing a specific solution.

---

## What this assignment is

**"Promise Them Nothing Twice"** is a take-home for **harness engineering and agentic system design**. You're joining fictional **RelayAPI** (B2B metered HTTP APIs) and must resolve a **deliberate conflict** between two stakeholder memos, then build a **thin vertical slice** that proves your limiter behaves correctly at quota boundaries.

The assignment is explicit that the memos — not the summary page — are the spec:

> "Read the source memos (linked below). They are the assignment. Do not treat this page as a cleaned-up spec that replaces them — **the tension between the memos is intentional**."

> "Your job is **not to make both memos literally true**. Your job is to **resolve the conflict explicitly**, build a thin vertical slice that reflects your resolution, and **prove your limiter behaves correctly at the boundary** — including the places where naive implementations are wrong."

---

## The core conflict (quoted)

### CTO (Priya Nair) — hard quota enforcement

> "**a customer must never exceed their contracted quota**. Not 'mostly,' not 'on average' — **never**."

> "When a customer hits their RPM limit, return `429 Too Many Requests` with a `Retry-After` header."

> "Two customers on the same tier must get the same treatment. **No hidden bypasses**, no manual overrides in code paths that production traffic hits."

> "I am fine with **eventual consistency** as long as the error direction is **under-limiting, not over-limiting**. I would rather reject a few extra legitimate requests than let someone blow past quota because nodes disagreed."

> "Show me a demo where two customers on a 100 RPM tier each get exactly their budget, and a third customer who exceeds 100 RPM gets cut off — **even when I hammer the load balancer randomly across all three nodes**."

### Support (Marcus Webb) — Northwind must not see 429s in batch window

> "**Northwind must never see a 429 during their batch window.**"

> "Northwind's nightly batch job runs **02:00–04:00 UTC**. During that window they send sustained traffic above their contracted RPM."

> "If you need a temporary exception mechanism, fine — but it must be **invisible to the customer**. They should not see errors while we figure out a commercial arrangement."

### Platform context — facts that constrain design

| Fact | Detail |
|------|--------|
| Topology | 3 stateless nodes, round-robin LB, no sticky sessions today |
| Northwind | ~60% ARR, contracted **300 RPM**, batch sends **~800–1200 RPM** for 90–120 min |
| Batch window | **02:00–04:00 UTC** nightly |
| Identity | `X-Customer-Id` header (trusted from gateway) |
| Redis | **may or may not** be available — "do not assume ops will provision new infra for a prototype" |
| Past failures | Previous limiter **allowed traffic well above quota** under LB distribution; staging prototype had **correctness issues at quota boundaries** |

---

## What you must build (minimum)

From `assignment.md`:

1. **Rate-limiting HTTP API or middleware** — per-customer limits, works on **multiple instances** (not single-process-only).
2. **Documented algorithm choice** — token bucket, sliding window, fixed window, leaky bucket, or hybrid — with reasoning tied to fairness, burst behavior, and distributed coordination.
3. **Load harness** — drives the service at quota boundaries; results must be legible; **first-class deliverable**.
4. **Boundary demonstrations** — harness output should make correct/incorrect behavior obvious without reading your code.

Expected slice from platform context:

> "A thin vertical slice: one endpoint (e.g. `GET /api/v1/ping` or a mock resource), real limiter middleware, config for at least two fake customer IDs including a stand-in for Northwind, and a harness that can simulate multi-node deployment (processes, containers, or documented equivalent)."

---

## What you submit (and what gets graded)

Your path: `submissions/victorsimon25/promise-them-nothing-twice/` (you already have a placeholder `DECISIONS.md` there — still the template).

```
submissions/victorsimon25/promise-them-nothing-twice/
├── solution/          # service + harness + README (≤15 min to run)
├── sessions/          # AI session exports — PRIMARY deliverable
└── DECISIONS.md       # one page: conflict, algorithm, verification, next steps
```

Important evaluation notes:

> "**This is the primary deliverable we evaluate.**" (referring to `sessions/`)

> "Heavy AI use is **required**."

> "We score how you **frame problems**, pack context the agent cannot know, **decompose work**, catch bluffs, and recover from failures."

> "We do **not** grade variable names, micro-optimizations, or framework fashion."

Timebox: **4–6 hours**, hard cap one weekend. Unfinished work with honest `DECISIONS.md` beats a polished façade.

---

## Aspects you need to understand before building

These are the dimensions the assignment forces you to decide on — not solutions, but the problem space.

### 1. Conflict resolution (business/product stance)

You must pick a **explicit** stance in `DECISIONS.md`: what you decided, what you rejected, and why. The memos cannot both be literally true because:

- CTO: never exceed 300 RPM → Northwind batch at 800–1200 RPM **must** get 429s if enforced literally.
- Support: Northwind **must never** see 429 during 02:00–04:00 UTC.

**Decision axes (options + tradeoffs):**

| Stance | What it means | Tradeoffs |
|--------|---------------|-----------|
| **Align with CTO (strict contract enforcement)** | Northwind gets 429s when over 300 RPM, including batch window | Honest to SLA/legal language; risks losing 60% ARR narrative; Marcus's escalation is "unresolved" — you must say that explicitly |
| **Align with Support (operational guarantee for Northwind)** | No 429s for Northwind in batch window | Saves the revenue story; violates literal "never exceed contracted quota" unless you reframe what "quota" means |
| **Explicit commercial/config exception (CTO's allowed path)** | CTO said: "If we ever grant a commercial exception, it goes through **config and audit** — not a midnight commit" | Reconciles both memos partially; you must document the exception transparently in `DECISIONS.md` even if invisible to customer; still need to explain how this doesn't break "same tier = same treatment" for other Enterprise customers |
| **Reject both as stated; propose contract/process change** | e.g. renegotiate Northwind tier to match batch RPM | Most honest long-term fix; doesn't fit Marcus's "invisible, works now" ask; may be valid if you show why engineering alone can't resolve it |

### 2. Rate-limiting algorithm

Assignment requires a documented choice among: **token bucket, sliding window, fixed window, leaky bucket, hybrid**.

**Tradeoffs to think about (general, not a recommendation):**

| Algorithm | Strengths | Weaknesses / boundary pitfalls |
|-----------|-----------|--------------------------------|
| **Fixed window** | Simple, auditable counting | Classic **boundary spike** at window rollovers (2× burst at edges); hard to explain "fair" to enterprise |
| **Sliding window** | Smoother, fairer over time | More complex; distributed sliding windows are easy to get wrong; off-by-one at boundaries |
| **Token bucket** | Natural burst handling | "Burst" may conflict with CTO's "never exceed" if interpreted as hard cap vs average rate |
| **Leaky bucket** | Steady output rate | Less intuitive for RPM semantics; burst behavior differs from token bucket |
| **Hybrid** | Can tune fairness + burst | Harder to audit; more to prove in harness |

CTO's demo asks for customers on **100 RPM** getting "exactly their budget" — algorithm choice affects whether "exactly" is achievable or approximate.

### 3. Distributed coordination (3 nodes)

> "Nodes do not share memory."

> "Whatever you build must work when requests land on different nodes between seconds."

**Coordination options (tradeoffs only):**

| Approach | Pros | Cons |
|----------|------|------|
| **Central shared store** (e.g. Redis, if you assume it) | Single source of truth; easier to reason about global count | Redis "may or may not" be available; latency; eventual consistency still matters |
| **No shared store** (per-node counters) | No infra dependency | Almost always **over-admits** (each node allows full quota) unless you add something else — matches the **deprecated limiter's failure mode** |
| **Probabilistic / approximate** (e.g. gossip, local estimate) | Scales well | Hard to satisfy CTO's audit + "never exceed" without careful bias toward under-limiting |
| **External rate-limit service / sidecar** | Clean separation | More moving parts for a 4–6 hour slice |
| **Sticky sessions / affinity** | Per-node limits become meaningful | Platform says **no sticky sessions requirement today**; changes LB behavior; uneven load |

CTO explicitly prefers **under-limiting over over-limiting** when nodes disagree — that's a design constraint on any distributed approach.

### 4. Northwind exception mechanism (if you go that route)

Marcus allows: "If you need a temporary exception mechanism, fine — but it must be **invisible to the customer**."

CTO forbids: "Special-case hacks buried in `if (customerId === ...)` blocks."

**Mechanism types (tradeoffs):**

| Mechanism | Fits CTO's config/audit rule? | Fits Marcus's invisibility? | Risk |
|-----------|------------------------------|----------------------------|------|
| **Time-window override in config** (elevated limit 02:00–04:00) | Yes, if config-driven and documented | Yes, customer sees no errors | Other Enterprise customers may ask why Northwind differs; fairness question |
| **Separate effective quota per customer in config** | Yes | Yes | Commercially honest but contradicts "same tier same treatment" unless tier is redefined |
| **Hardcoded customer ID bypass** | No — CTO explicitly rejects | Yes | Disqualifying in spirit of the memo |
| **Don't exception — change contract RPM in demo config** | Yes | Yes | Honest for demo; implies commercial team action in real life |

### 5. Harness and verification scope

The harness must prove boundary behavior; `DECISIONS.md` must say **what it proves and what it does not**.

**Dimensions to decide:**

| Harness scope | Proves | Does not prove |
|---------------|--------|----------------|
| Single process | Algorithm correctness locally | LB + multi-node disagreement |
| Multi-process / multi-container | Distributed behavior under random routing | Production scale, real Redis cluster failure modes |
| Two fair customers + one over-quota (CTO demo) | Isolation and cutoff | Northwind batch narrative |
| Northwind batch simulation (800–1200 RPM in window) | Support scenario | Whether you're "cheating" via exception vs true limiter |
| 429 + `Retry-After` presence | CTO requirement #1 | Retry storm behavior (Support mentions aggressive retries amplify load) |

Assignment warns: "tests that prove nothing" and "subtly wrong distributed designs" are exactly what reviewers look for you to catch.

### 6. Process / meta requirements (easy to underweight)

- **Session exports are the main grade** — this conversation could become `01-framing.md` if you export it.
- **Decomposition** — avoid one mega-prompt; sequenced sessions expected.
- **Honesty** — incomplete OK if documented; sanitized sessions **disqualify**.

---

## Tensions you cannot ignore (they're the assignment)

1. **300 RPM contract vs 800–1200 RPM batch** — pure engineering cannot satisfy both memos literally.
2. **Strict fairness vs Northwind exception** — same tier, same treatment vs 60% ARR survival.
3. **Never exceed vs under-limiting bias** — CTO allows rejecting extra legit requests but not exceeding quota; distributed systems often briefly over-admit before converging.
4. **Auditable semantics vs invisible exception** — enterprise prospects want counting explained; Marcus wants customer-blind fixes.
5. **Redis optional** — strong coordination vs laptop-only prototype in ≤15 minutes.

---

## Clarifications for you before you proceed

I won't assume your preferences on these:

1. **Conflict stance:** Are you leaning toward strict CTO enforcement, a config-based Northwind exception, or explicitly rejecting one memo? That choice drives everything else.

2. **Infrastructure assumptions:** For your slice, do you plan to assume Redis (or another shared store), or constrain yourself to no new infra as the platform doc hints?

3. **Stack:** Any language/runtime preference, or purely "fastest to demo in 4–6 hours"?

4. **Multi-node simulation:** Containers (Docker Compose), multiple local processes, or something else — do you have a preference for how you'll simulate 3 nodes + LB?

5. **Session workflow:** Will you export this chat and future ones as your `sessions/` folder? (That's what they're evaluating.)

6. **Fork/PR path:** Confirm your GitHub username is `victorsimon25` and you'll PR to upstream `ti-hiring` — I only see the template under your submission dir so far, no `solution/` or `sessions/` yet.

7. **Interpretation of "never exceed quota":** Do you read that as a hard cap on every rolling minute, an average over time, or a billing/legal statement that might allow a documented commercial override? Your read affects algorithm and exception design.

Once you answer those, we can go deeper on any one dimension (e.g. only distributed coordination, or only how to structure the harness claims) — still without writing code in this session if you prefer.

---

**User**

to answer your questions 
1)do a conifg so that noethwind wont get a 429 during that interval 
2)ill ship reddis along with docker compose and also give me details on what may happen when the reddis is not available during the request time then how is the limiter supposed to respond, same here give me all viable options along with the detaisl
3)i thought of going with go if there a better option that you would suggest in accordance with this problem also let me know 
4) docker compose 
5)yes ill be exporting evry session 
6)yes my github username is victorsimon25 and when the project is done ill be forking it to https://github.com/trilogy-group/ti-assignment-promise-them-nothin.git 
7)i see it as a hardcap and the limiter never lets anyone above the configured limit 

---

**Cursor**

Good — your answers narrow the design space a lot. Below is how those choices fit together, what they imply, and the option menus (especially Redis failure) — still without prescribing a full implementation.

---

## Your stated positions (locked in)

| # | Your choice | What it means in practice |
|---|-------------|---------------------------|
| 1 | Config-based Northwind exception for 02:00–04:00 UTC | You are **not** picking Marcus over Priya in a raw sense — you are using Priya’s allowed path: commercial exception via **config + audit**, not a hardcoded `if (customerId === "northwind")`. |
| 2 | Redis in Docker Compose | Distributed counting has a shared source of truth; reviewers can run the full stack locally. |
| 3 | Go | Reasonable default for this problem (see language section). |
| 4 | Docker Compose | Matches assignment expectation for multi-node simulation. |
| 5 | Export every session | Aligns with what they actually grade. |
| 6 | `victorsimon25`, PR to upstream | Small logistics note below. |
| 7 | Hard cap — never above **configured** limit | Enforcement is strict; the Northwind story becomes **what is configured when**, not “allow overages.” |

**Logistics clarification:** You **fork** [trilogy-group/ti-assignment-promise-them-nothin](https://github.com/trilogy-group/ti-assignment-promise-them-nothin) **to your GitHub account**, build under `submissions/victorsimon25/promise-them-nothing-twice/`, then open a PR **back to** the upstream repo — not fork into `trilogy-group`.

---

## How your conflict resolution fits the memos

You are essentially choosing:

> **Enforce hard caps on the configured limit. Northwind’s batch window uses a higher configured limit (documented exception). Everyone else stays on tier defaults.**

That is compatible with both memos **if you document it honestly** in `DECISIONS.md`:

**What you can claim:**
- Priya: hard enforcement, 429 + `Retry-After`, per-customer isolation, auditable config, no midnight code hacks.
- Marcus: Northwind sees no 429 during batch **if** configured limit ≥ their sustained batch rate (~800–1200 RPM).

**What you must explicitly reject or qualify:**
- Priya’s literal “never exceed **contracted** quota” for Northwind during batch — you reject that **unless** contract is updated; instead you enforce **configured operational quota**.
- Priya’s “same tier, same treatment” — Northwind gets a **documented commercial exception**; other Enterprise 300 RPM customers do not get the window unless config says so.

**Hard cap + time window creates boundary problems you must understand (not solve yet):**

| Moment | Risk |
|--------|------|
| **01:59 → 02:00 UTC** | Limit jumps from 300 → e.g. 1200. Harmless direction for Northwind. |
| **04:00 → 04:01 UTC** | Limit drops while batch traffic may still be ~800–1200 RPM → **429s can appear immediately** unless batch actually ends or config tapers. Marcus only asked for the window, not the tail — worth calling out in `DECISIONS.md`. |
| **Clock skew** | Nodes disagree on window boundaries → brief wrong limit. Needs UTC and consistent time source. |
| **Retry amplification** | Platform doc: Northwind retries aggressively on 429 → more load. A spike at 04:00 can get ugly fast. |

**Config shape options (tradeoffs only):**

| Config model | Pros | Cons |
|--------------|------|------|
| **Per-customer schedule** (`northwind`: 300 default, 1200 in 02:00–04:00 UTC) | Clear audit story; Marcus-invisible | Breaks literal tier fairness; must document in `DECISIONS.md` |
| **Separate “contract RPM” vs “enforcement RPM”** | Clean commercial narrative | Two numbers to explain to enterprise security reviews |
| **Global “batch mode” flag** | Simple | Less auditable per-customer; weaker isolation story |
| **Elevated limit only when header + time match** | Flexible | More moving parts; easier to get wrong in harness |

For your stance, **per-customer scheduled enforcement limit** is the most aligned with Priya’s “config and audit, not code hacks.”

---

## Redis + Docker Compose: what you are committing to

Typical compose topology for the assignment:

- **1× load balancer** (nginx/traefik) → round-robin to **3× app replicas**
- **1× Redis** (limiter state)
- **Harness** (host or compose service) → hits LB, random or round-robin, drives quota boundaries

This directly addresses Priya’s demo requirement:

> “even when I hammer the load balancer randomly across all three nodes”

And the platform failure mode you must not repeat:

> Previous limiter “did not correctly enforce limits under load-balanced distribution” — decommissioned after it allowed traffic **well above contracted quota**.

Redis gives you a path to a **global count** instead of 3× independent counters.

---

## When Redis is unavailable at request time — all viable options

This is one of the most important design choices given your **hard cap** stance. There is no free lunch: **availability vs strict never-exceed** conflicts when the coordination store is down.

CTO’s relevant guidance:

> “I am fine with eventual consistency as long as the error direction is **under-limiting, not over-limiting**.”

That biases away from anything that **admits extra traffic** when Redis is down.

### Option A — Fail closed (reject requests)

**Behavior:** Redis unreachable → return **503 Service Unavailable** (or 429 — see below).

| Pros | Cons |
|------|------|
| Cannot exceed quota via limiter bypass | Northwind batch can fail even with window exception |
| Simplest to reason about and audit | Violates Marcus if he interprets “never errors” broadly (429 **and** possibly 503) |
| Matches “hard cap” strictly | Bad UX for all customers during Redis outage |

**429 vs 503:** Priya mandates 429 for **quota exceeded**. Redis down is **infrastructure failure**, not quota — 503 is semantically cleaner. Marcus didn’t mention 503; clarify in `DECISIONS.md` what you chose and why.

---

### Option B — Fail open (allow traffic through)

**Behavior:** Redis down → skip limiting, pass to handler.

| Pros | Cons |
|------|------|
| Maximum availability | **Directly violates your hard cap** — worst case 3 nodes × full traffic |
| Northwind batch might “work” during Redis outage | Same failure class as the **deprecated limiter** |
| Simple implementation | Unacceptable for Priya’s “never exceed quota” |

Given your #7, this option is **inconsistent** unless you explicitly reject hard cap during infra failure (you said you wouldn’t).

---

### Option C — Conservative per-node local fallback (static shard cap)

**Behavior:** Each node enforces **floor(enforcement_rpm / node_count)** locally when Redis is unavailable (e.g. 300 RPM → 100 RPM per node with 3 nodes).

| Pros | Cons |
|------|------|
| **Upper bound** if traffic spreads: 3 × 100 = 300 max under perfect spread | **Imbalanced LB** → one hot node caps at 100 while others idle → heavy **under-limiting** |
| No over-admit vs single-node full quota on each replica | Northwind at 1200 configured: per-node 400; still need global cap story — **400 × 3 = 1200** only if perfectly balanced |
| Aligns with Priya’s under-limit bias | Uneven load → customer gets far less than configured |
| Works without Redis for survival mode | Not equivalent to global algorithm; window transitions harder |

**Important nuance:** This prevents the classic **3× quota** bug but does **not** guarantee “exactly configured RPM” — it guarantees **≤ configured RPM under worst-case routing assumptions** only if math is done correctly (and even then, imbalance causes under-limiting, not over-limiting).

---

### Option D — Local fallback with cached Redis state (stale reads)

**Behavior:** Keep last known counter/tokens in memory; continue with TTL/staleness bounds when Redis errors.

| Pros | Cons |
|------|------|
| Smoother degradation than pure static cap | Stale cache across nodes can **over-admit** briefly |
| Can preserve algorithm semantics approximately | Hard to prove hard cap; easy to get wrong |
| Better throughput during blips | Audit story gets muddy (“we guessed”) |

Risky for your **hard cap** claim unless cache is deliberately pessimistic (which pushes toward under-limiting).

---

### Option E — Redis required for startup; no traffic until healthy

**Behavior:** App doesn’t serve (or health check fails) until Redis is up; at runtime, same as A or C if Redis drops.

| Pros | Cons |
|------|------|
| Clear ops model | Runtime Redis loss still needs A/C policy |
| Avoids “cold start split brain” | Compose demo must wait for dependencies |

---

### Option F — Hybrid: Redis primary + always-on local “safety bucket”

**Behavior:** Redis authoritative when healthy; each node also runs a **strict local emergency bucket** (very conservative) that never exceeds its shard budget regardless of Redis.

| Pros | Cons |
|------|------|
| Redis up → accurate global limiting | More complex; two limiters in one path |
| Redis down → still bounded | Emergency bucket usually **under-limits** |
| Good story for “never over, sometimes under” | Must define which limiter wins when both active (typically **min** of the two) |

This is a common production pattern expressed in assignment-friendly terms.

---

### Option G — Queue / sync barrier (block until Redis responds)

**Behavior:** Wait/retry Redis with short timeout; fail closed if timeout exceeded.

| Pros | Cons |
|------|------|
| Avoids immediate fail open | Latency spikes; retry storms under load |
| Might recover from sub-second blips | Under load, blocking threads/goroutines hurts Go service |

---

### Summary matrix (for your hard cap + Northwind goals)

| Option | Over-quota risk when Redis down | Northwind batch during outage | Fits Priya under-limit bias | Fits your hard cap |
|--------|----------------------------------|-------------------------------|----------------------------|-------------------|
| A Fail closed | None | Fails (503/429) | Yes | Yes |
| B Fail open | **High** | Works | No | **No** |
| C Per-node shard cap | Low if math correct | Partial (imbalance) | Yes | Mostly yes |
| D Stale cache | Medium | Uncertain | Weak | Weak |
| E Startup gate only | Depends on runtime policy | Depends | Partial | Partial |
| F Hybrid safety bucket | Low | Partial | **Strong** | **Strong** |
| G Block/retry | Low if timeout → A | Poor under load | Yes | Yes |

**For your stated priorities (hard cap + Northwind window):** Options **A, C, F, or G→A** are viable. **B is ruled out.** Most submissions that claim hard cap pick **F** or **C** with explicit harness proof that Redis-up behavior is correct and Redis-down behavior **never exceeds** configured RPM.

**What to document in `DECISIONS.md`:** Redis failure is **out of scope for full production DR** or you **prove** bounded behavior in harness with Redis paused/killed — assignment asks what harness proves and **does not** prove.

---

## Algorithm choice (given Redis + hard cap + scheduled limits)

You still must pick and document one of: token bucket, sliding window, fixed window, leaky bucket, hybrid.

**Dimensions that matter for *your* choices:**

| Algorithm | Hard cap fit | Distributed + Redis | Window boundary bugs | Audit story |
|-----------|--------------|---------------------|----------------------|-------------|
| **Fixed window** | Strong if “RPM” = calendar minute | Easy in Redis (`INCR` + TTL) | **Classic 2× burst** at minute rollover | Easy to explain |
| **Sliding window** | Strongest “never exceed N per minute” semantics | Redis sorted sets / sliding log; more complex | Off-by-one under load | Good for enterprise |
| **Token bucket** | Hard cap only if bucket size = refill rate (no burst) | Easy in Redis | Burst config can violate “hard cap” if bucket > 1 | Must explain burst=0 |
| **Leaky bucket** | Smooth output | Moderate complexity | Different semantics vs RPM | Harder SLA mapping |
| **Hybrid** | Tunable | Common in production | Must prove in harness | Needs clear doc |

With **hard cap** and **no burst above configured limit**, token bucket with **zero burst** collapses toward fixed/sliding window behavior — worth knowing so you don’t accidentally introduce burst via algorithm defaults.

**Northwind schedule interaction:** Effective limit must be resolved **per request** (customer ID + timestamp UTC) **before** Redis check, so the same algorithm applies with different numeric caps.

---

## Go vs other languages

**Go is a strong choice** for this assignment — not because others fail, but because the problem fits Go well:

| Factor | Go | Alternatives |
|--------|-----|--------------|
| HTTP middleware/service | `net/http`, chi, echo, fiber — all fine | Python FastAPI, Node Express — fine |
| Concurrency / load harness | Goroutines + `net/http` client | Python asyncio OK; Node OK |
| Docker image size / startup | Small static binary | Python/Node heavier |
| Time to “correct distributed demo” in 4–6h | High if you know Go | Python may be faster **if** that’s your strongest language |
| Reviewer familiarity | Common for infra-style assignments | Any is acceptable per assignment |

**I would not switch away from Go** unless you are materially slower in Go than another language. The assignment explicitly says:

> “You choose the language, framework, and dependencies.”

> “We do **not** grade variable names, micro-optimizations, or framework fashion.”

Pick what lets you spend time on **harness + distributed correctness + session quality**. Go is well aligned with that.

---

## Docker Compose — what you should plan to demonstrate

Without prescribing files, your compose story should make these **observable**:

1. **Three app replicas** behind one LB URL (harness never pins to a single replica).
2. **Two fake customers** in config — Northwind stand-in + at least one other (CTO demo: two fair customers + one over-quota, or two at 100 RPM per Priya’s example).
3. **Harness scenarios** (conceptual):
   - At-limit traffic across random LB → no over-admit
   - Over-limit → 429 + `Retry-After`
   - Northwind profile during 02:00–04:00 UTC (mock clock or env override) → no 429 below configured batch cap
   - Outside window → 300 cap enforced
   - Optional: Redis stopped → document actual behavior matches `DECISIONS.md`

**Harness honesty:** Mocking time for UTC window is standard in a take-home; state in `DECISIONS.md` that you simulated the window rather than waiting until 02:00 UTC.

---

## Tensions still on you (given all answers)

1. **Hard cap at 04:00** when configured limit drops — Marcus’s “never 429 in window” does not cover the minute after; retries may still hit 429.
2. **Northwind configured at 1200** vs sustained 800–1200 — pick headroom (e.g. 1200 or 1500) and say why; too low → 429 in window; too high → weak “limiter” demo for Northwind.
3. **503 vs 429 on Redis failure** vs Marcus’s “invisible errors.”
4. **Fairness** — documented exception vs Priya’s tier fairness.
5. **Harness scope** — proving Redis-down behavior is valuable but adds time; assignment rewards honesty about what you skip.

---

## Clarifications still useful before you build

1. **Redis failure policy:** Are you leaning **fail closed (503)**, **per-node shard cap (C)**, or **hybrid safety bucket (F)**? That choice affects Northwind during outages.
2. **Post-window behavior:** Do you care about 429s at 04:00+ for Northwind, or only strictly inside 02:00–04:00?
3. **Northwind enforcement RPM during batch:** Fixed at 1200, 1500, or “match peak + margin”? Platform says 800–1200 sustained.
4. **Go framework:** Bare `net/http` vs chi/echo — preference for minimal deps?
5. **Time simulation:** Env var fake clock vs injecting window in config for demo — affects harness design.

Answer those when ready and we can go deeper on one slice (e.g. only Redis failure semantics, or only harness claims) — still no code unless you switch to Agent mode.

---

**User**

give me 300 rpm vs 809-1200 rpm as a ratio and hard enforcement always and for northwind limit is raised during the interval via config. if redis is unreachable just return 503 and post window 429 as out of scope and go with Bare net/http and finally injectable clock via env var so the harness can be stimulated through the window. to finish this session provide me a summary with in 300 words with the important decisons made and the situations thatwere decidced slong with the reasoning and the situaitions that was decalred to decide later 

---

**Cursor**

## Ratio (contract vs batch)

Northwind: **300 RPM contracted** vs **~800–1200 RPM** batch (platform doc). That is roughly **2.7×–4×** over contract (800/300 ≈ 2.67; 1200/300 = 4). Your batch-window config must sit **at or above** the upper bound if hard enforcement should produce no 429s in-window.

---

## Session summary (~300 words)

**Problem:** Build a distributed per-customer rate limiter for RelayAPI; resolve CTO vs Support conflict explicitly; prove boundary behavior via harness + session exports ([assignment repo](https://github.com/trilogy-group/ti-assignment-promise-them-nothin)).

**Conflict resolution:** Hard enforcement on the **configured** limit always. Reject literal “never exceed contracted quota” for Northwind during batch — instead, a **config-driven, auditable** higher limit during **02:00–04:00 UTC** (Priya’s allowed exception path; not hardcoded customer checks). Other customers stay on tier defaults. Document tier-fairness tradeoff in `DECISIONS.md`.

**Stack & delivery:** Go, **bare `net/http`**, Docker Compose (3 app replicas + LB + Redis). Submit under `submissions/victorsimon25/promise-them-nothing-twice/`; export all AI sessions; PR to upstream when done.

**Distributed limiting:** Redis as coordination store for global counts across nodes. **Redis unreachable → 503** (fail closed; preserves hard cap; infra failure, not quota). No fail-open, no local shard fallback.

**Verification:** Injectable **clock via env var** so harness simulates the batch window without waiting for real UTC. Harness hits LB across replicas. Post-window 429s when limit drops (e.g. 04:00) declared **out of scope** for this slice.

**Deferred:** Exact batch enforcement RPM (1200 vs headroom); algorithm choice (sliding window vs fixed window vs token bucket with zero burst); Redis key/schema design; config file format; `Retry-After` semantics; LB choice; harness scenario list and pass/fail thresholds; whether to demo Redis-down in harness or document only.

**Core reasoning:** Hard cap + multi-node requires shared state; 503 on Redis loss avoids over-admit; Northwind handled commercially via config, not code hacks; honesty about scope beats fake completeness for reviewers.

---

**User**

now update the @ti-assignment-promise-them-nothin/submissions/victorsimon25/promise-them-nothing-twice/DECISIONS.md   file with the summary provided above keeping the palce holders in the decions.md and the template same , keep it short and fill in only the necessary details 

---

**Cursor**

Updating `DECISIONS.md` with our session decisions while preserving the template structure.


Updated `DECISIONS.md` with the session decisions while keeping the same template structure:

- Header comment and all four section headings unchanged
- HTML placeholder comments retained under each section
- Bullet content added only where needed
- Deferred items grouped under **Technical design** and **If I had four more hours**

You can review the file in your editor — it should stay within one page. When you start building, revisit **Technical design** once you pick the algorithm and Northwind batch RPM.

