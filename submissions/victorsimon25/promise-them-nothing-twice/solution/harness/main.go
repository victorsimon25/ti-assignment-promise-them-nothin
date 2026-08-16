package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestResult stores the statistics of a scenario execution.
type TestResult struct {
	TotalRequests int
	SuccessCount  int
	BlockedCount  int
	ErrorCount    int // 503s
	OtherCount    int // network errors, timeouts, etc.
	Failed        bool
	Details       string
	NodeHits      map[string]int
}

// reqResult holds the metadata returned from a single worker request.
type reqResult struct {
	statusCode int
	nodeID     string
}

func main() {
	targetURL := flag.String("url", "http://localhost:8080", "Target URL of the load balancer")
	redisAddr := flag.String("redis", "localhost:6379", "Redis address for clearing state")
	flag.Parse()

	fmt.Println("=========================================================================")
	fmt.Println("              RelayAPI Rate Limiter Verification Harness                 ")
	fmt.Println("=========================================================================")
	fmt.Printf("Target Load Balancer:  %s\n", *targetURL)
	fmt.Printf("Redis Host Address:   %s\n\n", *redisAddr)

	// Try connecting to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:         *redisAddr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisOnline := true
	if err := rdb.Ping(ctx).Err(); err != nil {
		redisOnline = false
		fmt.Printf("Redis Status: OFFLINE (%v)\n", err)
		fmt.Println("Running Redis Outage (Fail-Closed) test scenario...")
	} else {
		fmt.Println("Redis Status: ONLINE. Preparing rate-limiting test scenarios...")
		defer rdb.Close()
	}

	results := make(map[string]*TestResult)

	if redisOnline {
		// Scenario 1: Starter Client (60 RPM Limit)
		results["1. Starter Client (60 RPM Limit)"] = runStarterTest(*targetURL, rdb)

		// Scenario 2: Enterprise Client Outside Window (300 RPM Limit)
		results["2. Enterprise Client Outside Window (300 RPM Limit)"] = runEnterpriseNormalTest(*targetURL, rdb)

		// Scenario 3: Enterprise Client Inside Window (1500 RPM capacity limit proof)
		results["3. Enterprise Client Inside Window (1500 RPM Limit Proof)"] = runEnterpriseWindowTest(*targetURL, rdb)

		// Scenario 4: Priya's Demo (Per-Customer Isolation)
		results["4. Priya's Demo (Per-Customer Isolation)"] = runPriyaIsolationTest(*targetURL, rdb)

		// Scenario 5a: Naive Fixed Window Boundary Rollover Test
		results["5a. Naive Fixed Window (Rollover Bursting allowed)"] = runFixedWindowBoundaryTest(*targetURL, rdb)

		// Scenario 5b: Sliding Window Log Boundary Rollover Test
		results["5b. Sliding Window Log (Rollover Bursting BLOCKED)"] = runSlidingWindowBoundaryTest(*targetURL, rdb)
	} else {
		// Scenario 6: Redis Outage (Fail-Closed)
		results["6. Redis Outage (Fail-Closed)"] = runRedisOutageTest(*targetURL)
	}

	printSummaryTable(results, redisOnline)
}

func runStarterTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("\nRunning Scenario 1: Starter Client (60 RPM)...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	// Send 75 requests. 60 should succeed, 15 should get 429.
	return executeBurst(url, "/api/v1/ping", "customer-starter", "", 75, 15, 60, 15)
}

func runEnterpriseNormalTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("Running Scenario 2: Enterprise Client Outside Window (300 RPM)...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	// Send 320 requests simulating 10:00 UTC. 300 should succeed, 20 should get 429.
	return executeBurst(url, "/api/v1/ping", "customer-enterprise", "2026-08-16T10:00:00Z", 320, 25, 300, 20)
}

func runEnterpriseWindowTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("Running Scenario 3: Enterprise Client Inside Window (1500 RPM Limit Proof)...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	// Send 1700 requests simulating 03:00 UTC. 1500 should succeed, 200 should get 429.
	return executeBurst(url, "/api/v1/ping", "customer-enterprise", "2026-08-16T03:00:00Z", 1700, 60, 1500, 200)
}

func runPriyaIsolationTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("Running Scenario 4: Priya's Demo (Per-Customer Isolation)...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	var wg sync.WaitGroup
	var res1, res2, res3 *TestResult

	wg.Add(3)
	go func() {
		defer wg.Done()
		res1 = executeBurst(url, "/api/v1/ping", "customer-priya-1", "", 120, 20, 100, 20)
	}()
	go func() {
		defer wg.Done()
		res2 = executeBurst(url, "/api/v1/ping", "customer-priya-2", "", 120, 20, 100, 20)
	}()
	go func() {
		defer wg.Done()
		res3 = executeBurst(url, "/api/v1/ping", "customer-priya-3", "", 80, 15, 80, 0)
	}()

	wg.Wait()

	// Combine results
	combinedFailed := res1.Failed || res2.Failed || res3.Failed
	combinedNodeHits := make(map[string]int)
	for k, v := range res1.NodeHits {
		combinedNodeHits[k] += v
	}
	for k, v := range res2.NodeHits {
		combinedNodeHits[k] += v
	}
	for k, v := range res3.NodeHits {
		combinedNodeHits[k] += v
	}

	details := fmt.Sprintf("Client 1 (120 reqs): %d OK / %d 429. Client 2 (120 reqs): %d OK / %d 429. Client 3 (80 reqs): %d OK / %d 429.",
		res1.SuccessCount, res1.BlockedCount,
		res2.SuccessCount, res2.BlockedCount,
		res3.SuccessCount, res3.BlockedCount)

	return &TestResult{
		TotalRequests: res1.TotalRequests + res2.TotalRequests + res3.TotalRequests,
		SuccessCount:  res1.SuccessCount + res2.SuccessCount + res3.SuccessCount,
		BlockedCount:  res1.BlockedCount + res2.BlockedCount + res3.BlockedCount,
		Failed:        combinedFailed,
		Details:       details,
		NodeHits:      combinedNodeHits,
	}
}

func runFixedWindowBoundaryTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("Running Scenario 5a: Naive Fixed Window Boundary Rollover Test...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	// Burst 1: 300 requests at 10:00:58 (end of minute window). Expected 300 succeed.
	res1 := executeBurst(url, "/api/v1/ping-fixed", "customer-growth", "2026-08-16T10:00:58Z", 300, 30, 300, 0)

	// Burst 2: 300 requests at 10:01:02 (just after rollover). In Fixed Window, this reset allows ALL 300 to pass!
	res2 := executeBurst(url, "/api/v1/ping-fixed", "customer-growth", "2026-08-16T10:01:02Z", 300, 30, 300, 0)

	combinedFailed := res1.Failed || res2.Failed
	combinedNodeHits := make(map[string]int)
	for k, v := range res1.NodeHits {
		combinedNodeHits[k] += v
	}
	for k, v := range res2.NodeHits {
		combinedNodeHits[k] += v
	}

	details := fmt.Sprintf("Naive Fixed Window allowed both bursts: Burst 1 (10:00:58): %d OK. Burst 2 (10:01:02): %d OK. Total allowed inside 4 seconds: %d (SHOULD BE 300 max).",
		res1.SuccessCount, res2.SuccessCount, res1.SuccessCount+res2.SuccessCount)

	return &TestResult{
		TotalRequests: res1.TotalRequests + res2.TotalRequests,
		SuccessCount:  res1.SuccessCount + res2.SuccessCount,
		BlockedCount:  res1.BlockedCount + res2.BlockedCount,
		Failed:        combinedFailed,
		Details:       details,
		NodeHits:      combinedNodeHits,
	}
}

func runSlidingWindowBoundaryTest(url string, rdb *redis.Client) *TestResult {
	fmt.Println("Running Scenario 5b: Sliding Window Log Boundary Rollover Test...")
	ctx := context.Background()
	_ = rdb.FlushDB(ctx)

	// Burst 1: 300 requests at 10:00:58. Expected 300 succeed.
	res1 := executeBurst(url, "/api/v1/ping", "customer-growth", "2026-08-16T10:00:58Z", 300, 30, 300, 0)

	// Burst 2: 300 requests at 10:01:02. Since it's only 4 seconds later, Sliding Window Log correctly remembers Burst 1 and BLOCKS all 300 requests!
	res2 := executeBurst(url, "/api/v1/ping", "customer-growth", "2026-08-16T10:01:02Z", 300, 30, 0, 300)

	combinedFailed := res1.Failed || res2.Failed
	combinedNodeHits := make(map[string]int)
	for k, v := range res1.NodeHits {
		combinedNodeHits[k] += v
	}
	for k, v := range res2.NodeHits {
		combinedNodeHits[k] += v
	}

	details := fmt.Sprintf("Sliding Window Log enforced limit: Burst 1 (10:00:58): %d OK. Burst 2 (10:01:02): %d OK / %d 429. Total allowed inside 4 seconds: %d.",
		res1.SuccessCount, res2.SuccessCount, res2.BlockedCount, res1.SuccessCount+res2.SuccessCount)

	return &TestResult{
		TotalRequests: res1.TotalRequests + res2.TotalRequests,
		SuccessCount:  res1.SuccessCount + res2.SuccessCount,
		BlockedCount:  res1.BlockedCount + res2.BlockedCount,
		Failed:        combinedFailed,
		Details:       details,
		NodeHits:      combinedNodeHits,
	}
}

func runRedisOutageTest(url string) *TestResult {
	fmt.Println("\nRunning Scenario 6: Redis Outage (Fail-Closed)...")
	totalReqs := 10
	var wg sync.WaitGroup
	resChan := make(chan reqResult, totalReqs)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < totalReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest("GET", url+"/api/v1/ping", nil)
			if err != nil {
				resChan <- reqResult{statusCode: 999}
				return
			}
			req.Header.Set("X-Customer-Id", "customer-enterprise")
			resp, err := client.Do(req)
			if err != nil {
				resChan <- reqResult{statusCode: 999}
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			node := resp.Header.Get("X-Node-Id")
			if node == "" {
				node = "unknown"
			}
			resChan <- reqResult{statusCode: resp.StatusCode, nodeID: node}
		}()
	}

	wg.Wait()
	close(resChan)

	success := 0
	blocked := 0
	unavailable := 0
	other := 0
	nodeHits := make(map[string]int)

	for res := range resChan {
		if res.nodeID != "" {
			nodeHits[res.nodeID]++
		}
		switch res.statusCode {
		case 200:
			success++
		case 429:
			blocked++
		case 503:
			unavailable++
		default:
			other++
		}
	}

	failed := false
	var details string
	if unavailable != totalReqs {
		failed = true
		details = fmt.Sprintf("Expected %d requests to fail with 503. Got %d.", totalReqs, unavailable)
	} else {
		details = fmt.Sprintf("Successfully failed closed: %d requests got 503 Service Unavailable.", unavailable)
	}

	return &TestResult{
		TotalRequests: totalReqs,
		SuccessCount:  success,
		BlockedCount:  blocked,
		ErrorCount:    unavailable,
		OtherCount:    other,
		Failed:        failed,
		Details:       details,
		NodeHits:      nodeHits,
	}
}

func executeBurst(url, path, customerID, fakeTime string, totalReqs, concurrency, expectedSuccess, expectedBlocked int) *TestResult {
	var wg sync.WaitGroup
	reqChan := make(chan int, totalReqs)
	for i := 0; i < totalReqs; i++ {
		reqChan <- i
	}
	close(reqChan)

	resChan := make(chan reqResult, totalReqs)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqChan {
				req, err := http.NewRequest("GET", url+path, nil)
				if err != nil {
					resChan <- reqResult{statusCode: 999}
					continue
				}
				req.Header.Set("X-Customer-Id", customerID)
				if fakeTime != "" {
					req.Header.Set("X-Fake-Time", fakeTime)
				}

				resp, err := client.Do(req)
				if err != nil {
					resChan <- reqResult{statusCode: 999}
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				node := resp.Header.Get("X-Node-Id")
				if node == "" {
					node = "unknown"
				}
				resChan <- reqResult{statusCode: resp.StatusCode, nodeID: node}
			}
		}()
	}

	wg.Wait()
	close(resChan)

	success := 0
	blocked := 0
	unavailable := 0
	other := 0
	nodeHits := make(map[string]int)

	for res := range resChan {
		if res.nodeID != "" {
			nodeHits[res.nodeID]++
		}
		switch res.statusCode {
		case 200:
			success++
		case 429:
			blocked++
		case 503:
			unavailable++
		default:
			other++
		}
	}

	failed := false
	var details string
	if success != expectedSuccess || blocked != expectedBlocked {
		failed = true
		details = fmt.Sprintf("Expected %d OK / %d 429. Got %d OK / %d 429.", expectedSuccess, expectedBlocked, success, blocked)
	} else {
		details = fmt.Sprintf("Successfully enforced: %d OK, %d 429.", success, blocked)
	}

	return &TestResult{
		TotalRequests: totalReqs,
		SuccessCount:  success,
		BlockedCount:  blocked,
		ErrorCount:    unavailable,
		OtherCount:    other,
		Failed:        failed,
		Details:       details,
		NodeHits:      nodeHits,
	}
}

func printSummaryTable(results map[string]*TestResult, redisOnline bool) {
	fmt.Println("\n=========================================================================")
	fmt.Println("                            VERIFICATION REPORT                          ")
	fmt.Println("=========================================================================")
	fmt.Printf("| %-52s | %-6s | %-6s | %-6s | %-20s | %-6s |\n", "Scenario Name", "Reqs", "200s", "429s", "Node Hits Distribution", "Status")
	fmt.Println("|------------------------------------------------------|--------|--------|--------|----------------------|--------|")
	
	// Ensure printed in ordered sequence
	keys := []string{
		"1. Starter Client (60 RPM Limit)",
		"2. Enterprise Client Outside Window (300 RPM Limit)",
		"3. Enterprise Client Inside Window (1500 RPM Limit Proof)",
		"4. Priya's Demo (Per-Customer Isolation)",
		"5a. Naive Fixed Window (Rollover Bursting allowed)",
		"5b. Sliding Window Log (Rollover Bursting BLOCKED)",
		"6. Redis Outage (Fail-Closed)",
	}

	for _, name := range keys {
		res, exists := results[name]
		if !exists {
			continue
		}
		status := "PASS"
		if res.Failed {
			status = "FAIL"
		}
		// Format node hits: n-1:X, n-2:Y, n-3:Z
		hitsStr := fmt.Sprintf("n-1:%d, n-2:%d, n-3:%d", res.NodeHits["node-1"], res.NodeHits["node-2"], res.NodeHits["node-3"])
		fmt.Printf("| %-52s | %-6d | %-6d | %-6d | %-20s | %-6s |\n", name, res.TotalRequests, res.SuccessCount, res.BlockedCount, hitsStr, status)
		fmt.Printf("  -> Details: %s\n\n", res.Details)
	}
	fmt.Println("=========================================================================")

	if redisOnline {
		fmt.Println("To verify Scenario 6 (Redis Outage Fail-Closed behavior):")
		fmt.Println("1. Stop the Redis container: 'docker stop rlt-redis'")
		fmt.Println("2. Re-run this harness:       'go run harness/main.go'")
	}
}
