package compiler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// loadFixturePlan reads and parses the test-plan.json fixture.
func loadFixturePlan(t *testing.T) *db.Plan {
	t.Helper()

	data, err := os.ReadFile("../../fixtures/test-plan.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var fixture struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Root        db.Node  `json:"root"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	return &db.Plan{
		ID:          "test-plan-id",
		Name:        fixture.Name,
		Description: fixture.Description,
		Tags:        fixture.Tags,
		Root:        fixture.Root,
	}
}

// TestCompileFixturePlan compiles the full fixture plan and verifies
// the output contains expected k6 constructs.
func TestCompileFixturePlan(t *testing.T) {
	plan := loadFixturePlan(t)

	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// The script must be non-empty.
	if len(script) == 0 {
		t.Fatal("Compile() returned empty script")
	}

	// Verify imports.
	checks := []struct {
		name    string
		want    string
	}{
		{"http import", "import http from 'k6/http'"},
		{"check import", "check"},
		{"group import", "group"},
		{"sleep import", "sleep"},
		{"SharedArray import", "import { SharedArray } from 'k6/data'"},
		{"exec import", "import exec from 'k6/execution'"},
		{"ws import", "import ws from 'k6/ws'"},
	}
	for _, tc := range checks {
		if !strings.Contains(script, tc.want) {
			t.Errorf("%s: script should contain %q", tc.name, tc.want)
		}
	}

	// Verify options with externally-controlled executor.
	if !strings.Contains(script, "executor: 'externally-controlled'") {
		t.Error("script should contain externally-controlled executor")
	}
	if !strings.Contains(script, "vus: 10") {
		t.Error("script should contain vus: 10")
	}
	if !strings.Contains(script, "maxVUs: 100") {
		t.Error("script should contain maxVUs: 100")
	}
	if !strings.Contains(script, "duration: '10m'") {
		t.Error("script should contain duration: '10m'")
	}

	// Verify export default function.
	if !strings.Contains(script, "export default function ()") {
		t.Error("script should contain export default function")
	}

	// Verify token bucket helper (for externally-controlled).
	if !strings.Contains(script, "acquireToken") {
		t.Error("script should contain acquireToken for externally-controlled executor")
	}
	if !strings.Contains(script, "shouldStop") {
		t.Error("script should contain shouldStop watchdog")
	}

	// Verify HTTP requests.
	if !strings.Contains(script, "http.post(") {
		t.Error("script should contain http.post call")
	}
	if !strings.Contains(script, "http.get(") {
		t.Error("script should contain http.get call")
	}

	// Verify check() calls.
	if !strings.Contains(script, "check(") {
		t.Error("script should contain check() calls")
	}

	// Verify group() call for transaction controller.
	if !strings.Contains(script, "group('Browse Products'") {
		t.Error("script should contain group('Browse Products')")
	}

	// Verify once-only controller.
	if !strings.Contains(script, "if (__ITER === 0)") {
		t.Error("script should contain once-only __ITER === 0 check")
	}

	// Verify loop controller.
	if !strings.Contains(script, "for (let i = 0; i < 3; i++)") {
		t.Error("script should contain for loop with count 3")
	}

	// Verify if controller.
	if !strings.Contains(script, "if (response.json().items.length > 0)") {
		t.Error("script should contain if controller condition")
	}

	// Verify else controller.
	if !strings.Contains(script, "else {") {
		t.Error("script should contain else block")
	}

	// Verify code block.
	if !strings.Contains(script, "console.log('No products found');") {
		t.Error("script should contain code-block content")
	}

	// Verify sleep calls.
	if !strings.Contains(script, "sleep(") {
		t.Error("script should contain sleep() calls")
	}

	// Verify SharedArray for data source.
	if !strings.Contains(script, "new SharedArray(") {
		t.Error("script should contain SharedArray for data source")
	}

	// Verify WebSocket.
	if !strings.Contains(script, "ws.connect(") {
		t.Error("script should contain ws.connect for websocket node")
	}

	// Verify random controller.
	if !strings.Contains(script, "Math.random()") {
		t.Error("script should contain Math.random() for random controller")
	}
}

// TestCompileDeterministic verifies the compiler produces identical output
// for the same input.
func TestCompileDeterministic(t *testing.T) {
	plan := loadFixturePlan(t)

	script1, err := Compile(plan)
	if err != nil {
		t.Fatalf("first Compile() error: %v", err)
	}

	script2, err := Compile(plan)
	if err != nil {
		t.Fatalf("second Compile() error: %v", err)
	}

	if script1 != script2 {
		t.Error("Compile() is not deterministic: two calls with the same plan produced different output")
	}

	// Run it a third time to be sure.
	script3, err := Compile(plan)
	if err != nil {
		t.Fatalf("third Compile() error: %v", err)
	}

	if script1 != script3 {
		t.Error("Compile() is not deterministic on third run")
	}
}

// TestCompileRootTypeMismatch ensures the compiler rejects non-plan root nodes.
func TestCompileRootTypeMismatch(t *testing.T) {
	plan := &db.Plan{
		Root: db.Node{
			ID:         "bad-root",
			Type:       "http",
			Name:       "Not a plan",
			Enabled:    true,
			Properties: json.RawMessage(`{"method":"GET","url":"http://example.com","body":{"type":"none"},"follow_redirects":true}`),
		},
	}

	_, err := Compile(plan)
	if err == nil {
		t.Fatal("expected error for non-plan root, got nil")
	}
	if !strings.Contains(err.Error(), "expected root node type 'plan'") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestEmitHTTPGet tests compilation of an HTTP GET node.
func TestEmitHTTPGet(t *testing.T) {
	node := db.Node{
		ID:      "http-get-1",
		Type:    "http",
		Name:    "GET Example",
		Enabled: true,
		Properties: json.RawMessage(`{
			"method": "GET",
			"url": "https://example.com/api/items",
			"headers": [],
			"body": {"type": "none"}
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "http.get('https://example.com/api/items')") {
		t.Error("expected http.get call with URL")
	}
	if !strings.Contains(script, "import http from 'k6/http'") {
		t.Error("expected http import")
	}
}

// TestEmitHTTPPost tests compilation of an HTTP POST node with body and headers.
func TestEmitHTTPPost(t *testing.T) {
	node := db.Node{
		ID:      "http-post-1",
		Type:    "http",
		Name:    "POST Login",
		Enabled: true,
		Properties: json.RawMessage(`{
			"method": "POST",
			"url": "https://example.com/api/login",
			"headers": [{"key": "Content-Type", "value": "application/json"}],
			"body": {"type": "json", "content": "{\"user\":\"admin\"}"}
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "http.post(") {
		t.Error("expected http.post call")
	}
	if !strings.Contains(script, "'Content-Type': 'application/json'") {
		t.Error("expected Content-Type header")
	}
}

// TestEmitHTTPWithVarRefs tests URL variable reference resolution.
func TestEmitHTTPWithVarRefs(t *testing.T) {
	node := db.Node{
		ID:      "http-var-1",
		Type:    "http",
		Name:    "GET with var",
		Enabled: true,
		Properties: json.RawMessage(`{
			"method": "GET",
			"url": "${BASE_URL}/api/items",
			"headers": [],
			"body": {"type": "none"}
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	// Variable references should produce template literals.
	if !strings.Contains(script, "`${BASE_URL}/api/items`") {
		t.Error("expected template literal for variable references")
	}
}

// TestEmitCodeBlock tests compilation of a code block node.
func TestEmitCodeBlock(t *testing.T) {
	node := db.Node{
		ID:      "code-1",
		Type:    "code-block",
		Name:    "Custom Code",
		Enabled: true,
		Properties: json.RawMessage(`{
			"code": "console.log('hello world');"
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "console.log('hello world');") {
		t.Error("expected code block content verbatim")
	}
}

// TestEmitIfElseController tests if/else controller compilation.
func TestEmitIfElseController(t *testing.T) {
	plan := &db.Plan{
		ID: "if-else-plan",
		Root: db.Node{
			ID:         "root",
			Type:       "plan",
			Name:       "Test Plan",
			Enabled:    true,
			Properties: json.RawMessage(`{}`),
			Children: []db.Node{
				{
					ID:      "scenario-1",
					Type:    "scenario",
					Name:    "Test",
					Enabled: true,
					Properties: json.RawMessage(`{
						"executor": "constant-vus",
						"vus": 1,
						"duration": "1m"
					}`),
					Children: []db.Node{
						{
							ID:         "if-1",
							Type:       "if-controller",
							Name:       "Check condition",
							Enabled:    true,
							Properties: json.RawMessage(`{"condition": "x > 0"}`),
							Children: []db.Node{
								{
									ID:         "code-1",
									Type:       "code-block",
									Name:       "True branch",
									Enabled:    true,
									Properties: json.RawMessage(`{"code": "console.log('yes');"}`),
								},
							},
						},
						{
							ID:         "else-1",
							Type:       "else-controller",
							Name:       "Otherwise",
							Enabled:    true,
							Properties: json.RawMessage(`{}`),
							Children: []db.Node{
								{
									ID:         "code-2",
									Type:       "code-block",
									Name:       "False branch",
									Enabled:    true,
									Properties: json.RawMessage(`{"code": "console.log('no');"}`),
								},
							},
						},
					},
				},
			},
		},
	}

	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !strings.Contains(script, "if (x > 0) {") {
		t.Error("expected if block")
	}
	if !strings.Contains(script, "} else {") {
		t.Error("expected else block attached to if")
	}
	if !strings.Contains(script, "console.log('yes');") {
		t.Error("expected true branch code")
	}
	if !strings.Contains(script, "console.log('no');") {
		t.Error("expected false branch code")
	}
}

// TestEmitLoopController tests fixed-count loop compilation.
func TestEmitLoopController(t *testing.T) {
	node := db.Node{
		ID:         "loop-1",
		Type:       "loop-controller",
		Name:       "Repeat 5 times",
		Enabled:    true,
		Properties: json.RawMessage(`{"count": 5}`),
		Children: []db.Node{
			{
				ID:         "code-1",
				Type:       "code-block",
				Name:       "Loop body",
				Enabled:    true,
				Properties: json.RawMessage(`{"code": "console.log('iteration');"}`),
			},
		},
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "for (let i = 0; i < 5; i++)") {
		t.Error("expected for loop with count 5")
	}
}

// TestEmitWhileLoop tests while-condition loop compilation.
func TestEmitWhileLoop(t *testing.T) {
	node := db.Node{
		ID:         "loop-w-1",
		Type:       "loop-controller",
		Name:       "While loop",
		Enabled:    true,
		Properties: json.RawMessage(`{"while_condition": "retries < 3"}`),
		Children:   []db.Node{},
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "while (retries < 3)") {
		t.Error("expected while loop with condition")
	}
}

// TestEmitTransactionController tests group() generation.
func TestEmitTransactionController(t *testing.T) {
	node := db.Node{
		ID:         "tx-1",
		Type:       "transaction-controller",
		Name:       "Checkout Flow",
		Enabled:    true,
		Properties: json.RawMessage(`{}`),
		Children:   []db.Node{},
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "group('Checkout Flow'") {
		t.Error("expected group() call with transaction name")
	}
	if !strings.Contains(script, "import { group } from 'k6'") {
		t.Error("expected group import")
	}
}

// TestEmitOnceOnlyController tests __ITER check generation.
func TestEmitOnceOnlyController(t *testing.T) {
	node := db.Node{
		ID:         "once-1",
		Type:       "once-only-controller",
		Name:       "Setup",
		Enabled:    true,
		Properties: json.RawMessage(`{}`),
		Children:   []db.Node{},
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "if (__ITER === 0)") {
		t.Error("expected __ITER === 0 check")
	}
}

// TestEmitRandomController tests weighted random selection.
func TestEmitRandomController(t *testing.T) {
	node := db.Node{
		ID:         "random-1",
		Type:       "random-controller",
		Name:       "Random Pick",
		Enabled:    true,
		Properties: json.RawMessage(`{"weights": [80, 20]}`),
		Children: []db.Node{
			{
				ID:         "code-a",
				Type:       "code-block",
				Name:       "Option A",
				Enabled:    true,
				Properties: json.RawMessage(`{"code": "console.log('A');"}`),
			},
			{
				ID:         "code-b",
				Type:       "code-block",
				Name:       "Option B",
				Enabled:    true,
				Properties: json.RawMessage(`{"code": "console.log('B');"}`),
			},
		},
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "Math.random() * 100") {
		t.Error("expected weighted random with total 100")
	}
	if !strings.Contains(script, "__rand < 80") {
		t.Error("expected first weight threshold of 80")
	}
}

// TestEmitAssertion tests check() call generation.
func TestEmitAssertion(t *testing.T) {
	node := db.Node{
		ID:      "assert-1",
		Type:    "assertion",
		Name:    "Status is 200",
		Enabled: true,
		Properties: json.RawMessage(`{
			"target": "status",
			"condition": "equals",
			"value": 200,
			"negate": false
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "check(") {
		t.Error("expected check() call")
	}
	if !strings.Contains(script, "r.status === 200") {
		t.Error("expected status === 200 check expression")
	}
	if !strings.Contains(script, "import { check } from 'k6'") {
		t.Error("expected check import")
	}
}

// TestEmitAssertionNegate tests negated check expressions.
func TestEmitAssertionNegate(t *testing.T) {
	node := db.Node{
		ID:      "assert-neg-1",
		Type:    "assertion",
		Name:    "Not 500",
		Enabled: true,
		Properties: json.RawMessage(`{
			"target": "status",
			"condition": "equals",
			"value": 500,
			"negate": true
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "!(r.status === 500)") {
		t.Error("expected negated check expression")
	}
}

// TestEmitTimerConstant tests constant sleep generation.
func TestEmitTimerConstant(t *testing.T) {
	node := db.Node{
		ID:      "timer-1",
		Type:    "timer",
		Name:    "Wait 2s",
		Enabled: true,
		Properties: json.RawMessage(`{
			"timer_type": "constant",
			"duration_ms": 2000
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "sleep(2.000)") {
		t.Error("expected sleep(2.000) call")
	}
}

// TestEmitTimerUniformRandom tests uniform random sleep generation.
func TestEmitTimerUniformRandom(t *testing.T) {
	node := db.Node{
		ID:      "timer-2",
		Type:    "timer",
		Name:    "Random Wait",
		Enabled: true,
		Properties: json.RawMessage(`{
			"timer_type": "uniform_random",
			"min_ms": 500,
			"max_ms": 2000
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "sleep(0.500 + Math.random() * 1.500)") {
		t.Error("expected uniform random sleep expression")
	}
}

// TestEmitDataSourceCSV tests SharedArray generation for CSV.
func TestEmitDataSourceCSV(t *testing.T) {
	plan := &db.Plan{
		ID: "ds-plan",
		Root: db.Node{
			ID:         "root",
			Type:       "plan",
			Name:       "Test Plan",
			Enabled:    true,
			Properties: json.RawMessage(`{}`),
			Children: []db.Node{
				{
					ID:      "ds-1",
					Type:    "data-source",
					Name:    "Users CSV",
					Enabled: true,
					Properties: json.RawMessage(`{
						"source_type": "csv",
						"path": "data/users.csv",
						"variable_name": "users",
						"first_row_is_header": true
					}`),
				},
			},
		},
	}

	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if !strings.Contains(script, "new SharedArray('Users CSV'") {
		t.Error("expected SharedArray with name 'Users CSV'")
	}
	if !strings.Contains(script, "const users = new SharedArray") {
		t.Error("expected variable named 'users'")
	}
	if !strings.Contains(script, "open('data/users.csv')") {
		t.Error("expected open() call with CSV path")
	}
}

// TestEmitDisabledNode tests that disabled nodes are skipped.
func TestEmitDisabledNode(t *testing.T) {
	plan := &db.Plan{
		ID: "disabled-plan",
		Root: db.Node{
			ID:         "root",
			Type:       "plan",
			Name:       "Test Plan",
			Enabled:    true,
			Properties: json.RawMessage(`{}`),
			Children: []db.Node{
				{
					ID:      "scenario-1",
					Type:    "scenario",
					Name:    "Test",
					Enabled: true,
					Properties: json.RawMessage(`{
						"executor": "constant-vus",
						"vus": 1,
						"duration": "1m"
					}`),
					Children: []db.Node{
						{
							ID:         "code-disabled",
							Type:       "code-block",
							Name:       "Disabled Code",
							Enabled:    false,
							Properties: json.RawMessage(`{"code": "console.log('SHOULD_NOT_APPEAR');"}`),
						},
						{
							ID:         "code-enabled",
							Type:       "code-block",
							Name:       "Enabled Code",
							Enabled:    true,
							Properties: json.RawMessage(`{"code": "console.log('visible');"}`),
						},
					},
				},
			},
		},
	}

	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	if strings.Contains(script, "SHOULD_NOT_APPEAR") {
		t.Error("disabled node should not appear in output")
	}
	if !strings.Contains(script, "console.log('visible');") {
		t.Error("enabled node should appear in output")
	}
}

// TestEmitWebSocket tests websocket node compilation.
func TestEmitWebSocket(t *testing.T) {
	node := db.Node{
		ID:      "ws-1",
		Type:    "websocket",
		Name:    "WS Connect",
		Enabled: true,
		Properties: json.RawMessage(`{
			"url": "wss://example.com/ws",
			"connect_timeout": "5s",
			"messages": [{"type": "text", "data": "{\"subscribe\":\"updates\"}"}],
			"expected_messages": [{"match_type": "contains", "value": "subscribed", "timeout": "10s"}]
		}`),
	}

	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("CompileNode() error: %v", err)
	}

	if !strings.Contains(script, "ws.connect('wss://example.com/ws'") {
		t.Error("expected ws.connect call with URL")
	}
	if !strings.Contains(script, "socket.send(") {
		t.Error("expected socket.send for messages")
	}
	if !strings.Contains(script, "socket.on('message'") {
		t.Error("expected message handler for expected_messages")
	}
}

// TestSanitizeVarName covers the identifier sanitiser used to turn node IDs
// into JS variable name fragments. The crawler (see internal/crawler) mints
// IDs like "req-GET-https://example.com/a" that embed full URLs; before this
// was tightened, those IDs produced scripts like
//
//	let params_req_GET_https://example_com/a = { ... }
//
// which k6 rejected with a parse error, causing every run to fail within
// ~200ms with empty summary stats. This test pins the sanitiser to emit a
// valid ECMAScript identifier for every node ID we have seen in the wild.
func TestSanitizeVarName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"crawler URL-bearing ID", "req-GET-https://dogapi.dog/", "req_GET_https___dogapi_dog_"},
		{"nested path", "req-GET-https://example.com/a/b", "req_GET_https___example_com_a_b"},
		{"leading digit", "1node", "_1node"},
		{"only digits", "123", "_123"},
		{"dollar and underscore preserved", "$foo_bar", "$foo_bar"},
		{"spaces replaced", "my node 42", "my_node_42"},
		{"dots and dashes replaced", "a.b-c", "a_b_c"},
		{"colon replaced", "req:GET:/path", "req_GET__path"},
		{"query and fragment chars replaced", "req?k=v&x=y#f", "req_k_v_x_y_f"},
		{"empty string is safe", "", "_"},
		{"already valid identifier untouched", "foo_Bar123", "foo_Bar123"},
	}

	// Shape check: every output must be a valid JS IdentifierName
	// (non-empty; first rune in [A-Za-z_$]; rest in [A-Za-z0-9_$]).
	validIdentifier := func(s string) bool {
		if s == "" {
			return false
		}
		for i, r := range s {
			switch {
			case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_', r == '$':
				// ok at any position
			case r >= '0' && r <= '9':
				if i == 0 {
					return false
				}
			default:
				return false
			}
		}
		return true
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeVarName(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeVarName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if !validIdentifier(got) {
				t.Errorf("sanitizeVarName(%q) = %q is not a valid JS identifier", tc.in, got)
			}
		})
	}
}

// TestCompileCrawlerPlanProducesParseableScript is an end-to-end regression
// test for the sanitiser: a minimal crawler-shaped plan (node IDs embed full
// URLs) must compile into a script whose `let params_…` / `let res_…` lines
// are valid JS. Before the fix in sanitizeVarName, the k6 subprocess exited
// with `Unexpected token :` on exactly this input.
func TestCompileCrawlerPlanProducesParseableScript(t *testing.T) {
	plan := &db.Plan{
		ID:   "test-crawler-plan",
		Name: "crawler",
		Root: db.Node{
			ID: "root", Type: "plan", Name: "root", Enabled: true,
			Children: []db.Node{
				{
					ID: "scenario-crawl", Type: "scenario", Name: "Crawl", Enabled: true,
					Properties: json.RawMessage(`{"executor":"constant-vus","vus":1,"duration":"5s"}`),
					Children: []db.Node{
						{
							ID: "req-GET-https://dogapi.dog/", Type: "http",
							Name: "GET https://dogapi.dog/", Enabled: true,
							Properties: json.RawMessage(`{"method":"GET","url":"https://dogapi.dog/","body":{"type":"none"},"follow_redirects":true}`),
						},
						{
							ID: "req-GET-https://dogapi.dog/docs/api-v2", Type: "http",
							Name: "GET https://dogapi.dog/docs/api-v2", Enabled: true,
							Properties: json.RawMessage(`{"method":"GET","url":"https://dogapi.dog/docs/api-v2","body":{"type":"none"},"follow_redirects":true}`),
						},
					},
				},
			},
		},
	}

	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}

	// The compiled output must not contain any of the stray characters that
	// used to leak through into identifier positions.
	badMarkers := []string{
		"params_req_GET_https://",
		"res_req_GET_https://",
		"params_req_GET_https_:",
		"= http.get('https://dogapi.dog/', params_req_GET_https://",
	}
	for _, m := range badMarkers {
		if strings.Contains(script, m) {
			t.Errorf("script still contains invalid identifier fragment %q", m)
		}
	}

	// And the positive shape: both params and res identifiers for each URL
	// are emitted as underscore-only sequences.
	mustContain := []string{
		"let params_req_GET_https___dogapi_dog_ =",
		"http.get('https://dogapi.dog/', params_req_GET_https___dogapi_dog_)",
		"let params_req_GET_https___dogapi_dog_docs_api_v2 =",
		"http.get('https://dogapi.dog/docs/api-v2', params_req_GET_https___dogapi_dog_docs_api_v2)",
	}
	for _, m := range mustContain {
		if !strings.Contains(script, m) {
			t.Errorf("expected script to contain %q\n---script---\n%s", m, script)
		}
	}
}
