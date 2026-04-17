package db

import "encoding/json"

// SeedBuiltInPlans inserts the shipped plans if they don't exist.
func (s *PlanStore) SeedBuiltInPlans() error {
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM plans WHERE built_in = 1").Scan(&count)
	if count > 0 {
		return nil
	}

	plans := builtInPlans()
	for _, p := range plans {
		s.Create(p.Name, p.Description, p.Tags, p.Root, nil, true)
	}
	return nil
}

func builtInPlans() []Plan {
	return []Plan{
		// Plan 1: Echo API — Smoke Test
		{
			Name:        "Echo API — Smoke Test",
			Description: "Quick verification that the test harness works. Hits echo-api health, echo, and delay endpoints with 1 VU for 30 seconds.",
			Tags:        []string{"smoke", "echo-api", "built-in"},
			Root: Node{
				ID: "plan-echo-smoke", Type: "plan", Name: "Echo API — Smoke Test", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-echo-smoke", Type: "scenario", Name: "Echo Smoke", Enabled: true,
					Properties: json.RawMessage(`{"executor":"constant-vus","vus":1,"duration":"30s"}`),
					Children: []Node{
						{ID: "tc-echo-smoke", Type: "transaction-controller", Name: "Echo Flow", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-health", Type: "http", Name: "GET /health", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://echo-api:9001/health","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-echo", Type: "http", Name: "POST /echo", Enabled: true,
									Properties: json.RawMessage(`{"method":"POST","url":"http://echo-api:9001/echo","headers":[{"key":"Content-Type","value":"application/json"}],"body":{"type":"json","content":"{\"message\":\"hello\"}"}}`),
									Children:   []Node{}},
								{ID: "timer-echo-1", Type: "timer", Name: "Think 500ms", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":500}`),
									Children:   []Node{}},
								{ID: "req-delay", Type: "http", Name: "GET /delay/100", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://echo-api:9001/delay/100","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
		// Plan 2: E-commerce — Browse & Buy Load Test
		{
			Name:        "E-commerce — Browse & Buy",
			Description: "Realistic multi-step user journey: browse products, view detail, place order, check order status. Ramps from 10 to 50 VUs over 14 minutes.",
			Tags:        []string{"load", "ecommerce", "built-in"},
			Root: Node{
				ID: "plan-ecom-load", Type: "plan", Name: "E-commerce — Browse & Buy", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-ecom", Type: "scenario", Name: "Browse and Buy", Enabled: true,
					Properties: json.RawMessage(`{"executor":"ramping-vus","stages":[{"duration":"2m","target":10},{"duration":"5m","target":30},{"duration":"5m","target":50},{"duration":"2m","target":0}]}`),
					Children: []Node{
						{ID: "tc-ecom-browse", Type: "transaction-controller", Name: "Browse & Buy", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-products", Type: "http", Name: "GET /products", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://mock-ecommerce:9002/products","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "timer-ecom-1", Type: "timer", Name: "Think 1s", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":1000}`),
									Children:   []Node{}},
								{ID: "req-product-detail", Type: "http", Name: "GET /products/p1", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://mock-ecommerce:9002/products/p1","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "timer-ecom-2", Type: "timer", Name: "Think 2s", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":2000}`),
									Children:   []Node{}},
								{ID: "req-order", Type: "http", Name: "POST /orders", Enabled: true,
									Properties: json.RawMessage(`{"method":"POST","url":"http://mock-ecommerce:9002/orders","headers":[{"key":"Content-Type","value":"application/json"}],"body":{"type":"json","content":"{\"product_id\":\"p1\",\"quantity\":1}"}}`),
									Children:   []Node{}},
								{ID: "timer-ecom-3", Type: "timer", Name: "Think 1s", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":1000}`),
									Children:   []Node{}},
								{ID: "req-order-status", Type: "http", Name: "GET /orders/latest", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://mock-ecommerce:9002/orders/latest","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
		// Plan 3: Slow API — Stress Test
		{
			Name:        "Slow API — Stress Test",
			Description: "Find the breaking point by ramping VUs against slow endpoints. Goes from 5 to 100 VUs over 8 minutes hitting /slow/1, /slow/2, and /flaky.",
			Tags:        []string{"stress", "slow-api", "built-in"},
			Root: Node{
				ID: "plan-slow-stress", Type: "plan", Name: "Slow API — Stress Test", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-slow", Type: "scenario", Name: "Stress Slow Endpoints", Enabled: true,
					Properties: json.RawMessage(`{"executor":"ramping-vus","stages":[{"duration":"1m","target":5},{"duration":"2m","target":20},{"duration":"2m","target":50},{"duration":"1m","target":100},{"duration":"2m","target":0}]}`),
					Children: []Node{
						{ID: "tc-slow", Type: "transaction-controller", Name: "Slow Requests", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-slow-1", Type: "http", Name: "GET /slow/1", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://slow-api:9003/slow/1","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-slow-2", Type: "http", Name: "GET /slow/2", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://slow-api:9003/slow/2","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-flaky", Type: "http", Name: "GET /flaky", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://slow-api:9003/flaky","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
		// Plan 4: Error API — Resilience Test
		{
			Name:        "Error API — Resilience Test",
			Description: "Observe error rates, rate limiting, and degradation behavior. 10 VUs for 2 minutes against error-producing endpoints.",
			Tags:        []string{"resilience", "error-api", "built-in"},
			Root: Node{
				ID: "plan-error-resilience", Type: "plan", Name: "Error API — Resilience Test", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-error", Type: "scenario", Name: "Error Resilience", Enabled: true,
					Properties: json.RawMessage(`{"executor":"constant-vus","vus":10,"duration":"2m"}`),
					Children: []Node{
						{ID: "tc-error", Type: "transaction-controller", Name: "Error Requests", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-random-error", Type: "http", Name: "GET /random-error", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://error-api:9004/random-error","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "timer-err-1", Type: "timer", Name: "Think 500ms", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":500}`),
									Children:   []Node{}},
								{ID: "req-rate-limit", Type: "http", Name: "GET /rate-limit", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://error-api:9004/rate-limit","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "timer-err-2", Type: "timer", Name: "Think 500ms", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":500}`),
									Children:   []Node{}},
								{ID: "req-degradation", Type: "http", Name: "GET /degradation", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://error-api:9004/degradation","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
		// Plan 5: Cross-API — Spike Test
		{
			Name:        "Cross-API — Spike Test",
			Description: "Sudden burst across all 4 test APIs to test system resilience. Spikes from 5 to 100 VUs and back down.",
			Tags:        []string{"spike", "cross-api", "built-in"},
			Root: Node{
				ID: "plan-spike", Type: "plan", Name: "Cross-API — Spike Test", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-spike", Type: "scenario", Name: "Spike Traffic", Enabled: true,
					Properties: json.RawMessage(`{"executor":"ramping-vus","stages":[{"duration":"30s","target":5},{"duration":"10s","target":100},{"duration":"1m","target":100},{"duration":"10s","target":5},{"duration":"1m","target":5}]}`),
					Children: []Node{
						{ID: "tc-spike", Type: "transaction-controller", Name: "Cross-API Requests", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-spike-echo", Type: "http", Name: "POST /echo", Enabled: true,
									Properties: json.RawMessage(`{"method":"POST","url":"http://echo-api:9001/echo","headers":[{"key":"Content-Type","value":"application/json"}],"body":{"type":"json","content":"{\"spike\":true}"}}`),
									Children:   []Node{}},
								{ID: "req-spike-products", Type: "http", Name: "GET /products", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://mock-ecommerce:9002/products","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-spike-slow", Type: "http", Name: "GET /slow/1", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://slow-api:9003/slow/1","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-spike-error", Type: "http", Name: "GET /random-error", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://error-api:9004/random-error","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
		// Plan 6: Echo API — Delay Validation
		{
			Name:        "Echo API — Delay Validation",
			Description: "Validate response time accuracy and status code handling. 5 VUs for 1 minute hitting delay and status endpoints.",
			Tags:        []string{"validation", "echo-api", "built-in"},
			Root: Node{
				ID: "plan-delay-validation", Type: "plan", Name: "Echo API — Delay Validation", Enabled: true,
				Properties: json.RawMessage(`{}`),
				Children: []Node{{
					ID: "scenario-delay", Type: "scenario", Name: "Delay Validation", Enabled: true,
					Properties: json.RawMessage(`{"executor":"constant-vus","vus":5,"duration":"1m"}`),
					Children: []Node{
						{ID: "tc-delay", Type: "transaction-controller", Name: "Delay Checks", Enabled: true,
							Properties: json.RawMessage(`{}`),
							Children: []Node{
								{ID: "req-delay-50", Type: "http", Name: "GET /delay/50", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://echo-api:9001/delay/50","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "req-delay-200", Type: "http", Name: "GET /delay/200", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://echo-api:9001/delay/200","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
								{ID: "timer-delay-1", Type: "timer", Name: "Think 300ms", Enabled: true,
									Properties: json.RawMessage(`{"timer_type":"constant","duration_ms":300}`),
									Children:   []Node{}},
								{ID: "req-status-418", Type: "http", Name: "GET /status/418", Enabled: true,
									Properties: json.RawMessage(`{"method":"GET","url":"http://echo-api:9001/status/418","headers":[],"body":{"type":"none"}}`),
									Children:   []Node{}},
							}},
					},
				}},
			},
		},
	}
}
