package compiler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func planWith(scenarios ...db.Node) *db.Plan {
	return &db.Plan{ID: "p", Name: "p", Root: db.Node{ID: "root", Type: "plan", Name: "root", Enabled: true, Children: scenarios}}
}

func scenarioNode(name, props string, children ...db.Node) db.Node {
	return db.Node{ID: "s-" + name, Type: "scenario", Name: name, Enabled: true, Properties: json.RawMessage(props), Children: children}
}

func httpNode(id, url string, children ...db.Node) db.Node {
	return db.Node{ID: id, Type: "http", Name: id, Enabled: true,
		Properties: json.RawMessage(`{"method":"GET","url":"` + url + `","headers":[],"body":{"type":"none"}}`), Children: children}
}

func TestManagedRampingBecomesExternallyControlled(t *testing.T) {
	plan := planWith(scenarioNode("Ramp",
		`{"executor":"ramping-vus","stages":[{"duration":"1m","target":20},{"duration":"30s","target":0}]}`,
		httpNode("r1", "http://echo-api:9001/health")))

	res, err := CompileWithOptions(plan, Options{Managed: true})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(res.Script, "executor: 'externally-controlled'") {
		t.Fatalf("expected externally-controlled executor:\n%s", res.Script)
	}
	if res.Profile == nil || len(res.Profile.Stages) != 2 || res.Profile.Peak() != 20 {
		t.Fatalf("unexpected profile: %+v", res.Profile)
	}
	if res.PlannedDuration != 90*time.Second {
		t.Errorf("planned duration = %s, want 1m30s", res.PlannedDuration)
	}
	if res.MaxDuration < res.PlannedDuration+5*time.Minute {
		t.Errorf("max duration %s should leave at least 5m of headroom", res.MaxDuration)
	}
	if res.MaxVUs != 20 {
		t.Errorf("max VUs = %d, want 20", res.MaxVUs)
	}
	if res.Control || strings.Contains(res.Script, "k6/x/kv") {
		t.Error("control scenario must only be added when KV is available")
	}
}

func TestManagedWithKVAddsControlScenarioAndPacing(t *testing.T) {
	plan := planWith(scenarioNode("Load", `{"executor":"constant-vus","vus":5,"duration":"2m"}`,
		httpNode("r1", "http://echo-api:9001/health")))

	res, err := CompileWithOptions(plan, Options{Managed: true, KV: true})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{
		"import { openKv } from 'k6/x/kv'",
		"'" + ControlScenario + "': {",
		"exec: '__osControl'",
		"export async function __osControl()",
		"export async function scenario_Load ()",
		"if (!(await __osSync())) return;",
		"__osPace();",
		"import exec from 'k6/execution'",
	} {
		if !strings.Contains(res.Script, want) {
			t.Errorf("managed script missing %q\n%s", want, res.Script)
		}
	}
	if !res.Control {
		t.Error("Result.Control should be true")
	}
}

func TestManagedOverridesStages(t *testing.T) {
	plan := planWith(scenarioNode("S", `{"executor":"constant-vus","vus":1,"duration":"30s"}`, httpNode("r", "http://x/")))
	ov := &db.RunParameters{Stages: []db.RunStage{{Duration: "10s", Target: 10}, {Duration: "20s", Target: 50}}}
	res, err := CompileWithOptions(plan, Options{Managed: true, Overrides: ov})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Profile.Peak() != 50 || res.PlannedDuration != 30*time.Second {
		t.Fatalf("override not applied: peak=%d dur=%s", res.Profile.Peak(), res.PlannedDuration)
	}
}

func TestManagedOverridesConstant(t *testing.T) {
	plan := planWith(scenarioNode("S", `{"executor":"ramping-vus","stages":[{"duration":"5m","target":100}]}`, httpNode("r", "http://x/")))
	ov := &db.RunParameters{VUsTarget: 3, DurationSeconds: 45}
	res, err := CompileWithOptions(plan, Options{Managed: true, Overrides: ov})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Profile.StartVUs != 3 || res.Profile.Peak() != 3 || res.PlannedDuration != 45*time.Second {
		t.Fatalf("constant override not applied: %+v dur=%s", res.Profile, res.PlannedDuration)
	}
}

func TestManagedArrivalRateStaysNative(t *testing.T) {
	plan := planWith(scenarioNode("Trickle",
		`{"executor":"constant-arrival-rate","rate":1,"time_unit":"1s","duration":"1h","pre_allocated_vus":2}`,
		httpNode("r", "http://x/")))
	ov := &db.RunParameters{RPSTarget: 7, DurationSeconds: 60}
	res, err := CompileWithOptions(plan, Options{Managed: true, KV: true, Overrides: ov})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Profile != nil || res.PlannedDuration != 0 || res.Control {
		t.Fatalf("arrival-rate plans must stay native without a control scenario: %+v", res)
	}
	for _, want := range []string{"executor: 'constant-arrival-rate'", "rate: 7", "duration: '60s'"} {
		if !strings.Contains(res.Script, want) {
			t.Errorf("missing %q\n%s", want, res.Script)
		}
	}
}

func TestPlanWithoutScenarioIsSynthesizedWhenManaged(t *testing.T) {
	plan := planWith(httpNode("loose", "http://x/"))
	res, err := CompileWithOptions(plan, Options{Managed: true, Overrides: &db.RunParameters{VUsTarget: 2, DurationSeconds: 10}})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res.Profile == nil || res.Profile.Peak() != 2 || !strings.Contains(res.Script, "http.get('http://x/')") {
		t.Fatalf("loose nodes should run in a synthesized managed scenario:\n%s", res.Script)
	}
}

func TestMultipleScenariosGetSeparateFunctionsAndOneOptionsBlock(t *testing.T) {
	plan := planWith(
		scenarioNode("Browse", `{"executor":"constant-vus","vus":2,"duration":"30s"}`, httpNode("a", "http://a/")),
		scenarioNode("Checkout", `{"executor":"constant-arrival-rate","rate":5,"time_unit":"1s","duration":"30s","pre_allocated_vus":5}`, httpNode("b", "http://b/")),
	)
	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if n := strings.Count(script, "export const options"); n != 1 {
		t.Fatalf("expected one options export, got %d\n%s", n, script)
	}
	for _, want := range []string{"exec: 'scenario_Browse'", "exec: 'scenario_Checkout'", "export function scenario_Browse ()", "export function scenario_Checkout ()"} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q\n%s", want, script)
		}
	}
	browse := script[strings.Index(script, "export function scenario_Browse"):strings.Index(script, "export function scenario_Checkout")]
	if strings.Contains(browse, "http://b/") {
		t.Error("Checkout's request leaked into Browse's function")
	}
}

func TestExpectedStatusesFromAssertions(t *testing.T) {
	assert418 := db.Node{ID: "a", Type: "assertion", Name: "is 418", Enabled: true,
		Properties: json.RawMessage(`{"target":"status","condition":"equals","value":418}`)}
	plan := planWith(scenarioNode("S", `{"executor":"constant-vus","vus":1,"duration":"5s"}`, httpNode("teapot", "http://x/status/418", assert418)))
	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(script, "responseCallback: http.expectedStatuses({ min: 200, max: 399 }, 418)") {
		t.Errorf("asserted 418 should be an expected status:\n%s", script)
	}
}

func TestHeaderValuesResolveVariables(t *testing.T) {
	node := db.Node{ID: "h", Type: "http", Name: "h", Enabled: true,
		Properties: json.RawMessage(`{"method":"GET","url":"http://x/","headers":[{"key":"Authorization","value":"Bearer ${TOKEN}"}],"body":{"type":"none"}}`)}
	script, err := CompileNode(node)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(script, "'Authorization': `Bearer ${__v('TOKEN')}`") {
		t.Errorf("header variable not resolved:\n%s", script)
	}
}

func TestVarRefsEscapeBackticks(t *testing.T) {
	c := newCompiler(Options{})
	got := c.resolveVarRefs("a`b${X}c\\d")
	want := "`a\\`b${__v('X')}c\\\\d`"
	if got != want {
		t.Errorf("resolveVarRefs = %s, want %s", got, want)
	}
	if c.resolveVarRefs("${__ITER}") != "`${__ITER}`" {
		t.Error("k6 globals should be referenced directly")
	}
}

func TestIfElseChainIsValid(t *testing.T) {
	ifNode := db.Node{ID: "if", Type: "if-controller", Name: "if", Enabled: true, Properties: json.RawMessage(`{"condition":"res && res.status === 200"}`),
		Children: []db.Node{{ID: "c1", Type: "code-block", Name: "yes", Enabled: true, Properties: json.RawMessage(`{"code":"console.log('yes');"}`)}}}
	elseNode := db.Node{ID: "else", Type: "else-controller", Name: "else", Enabled: true, Properties: json.RawMessage(`{}`),
		Children: []db.Node{{ID: "c2", Type: "code-block", Name: "no", Enabled: true, Properties: json.RawMessage(`{"code":"console.log('no');"}`)}}}
	plan := planWith(scenarioNode("S", `{"executor":"constant-vus","vus":1,"duration":"5s"}`, httpNode("r", "http://x/"), ifNode, elseNode))
	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(script, "} else {") {
		t.Errorf("else should attach to the if block:\n%s", script)
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{"30s": 30 * time.Second, "2m": 2 * time.Minute, "1h30m": 90 * time.Minute, "45": 45 * time.Second, "1.5": 1500 * time.Millisecond}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	if _, err := ParseDuration("soon"); err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestPlanThresholdsAreEmitted(t *testing.T) {
	plan := planWith(scenarioNode("S", `{"executor":"constant-vus","vus":1,"duration":"5s"}`, httpNode("r", "http://x/")))
	plan.Root.Properties = json.RawMessage(`{"thresholds":[
		{"metric":"http_req_failed","condition":"rate<0.05","abort_on_fail":true},
		{"metric":"http_req_duration","condition":"p(95)<500"},
		{"metric":"bad metric'); evil();//","condition":"x"}]}`)
	script, err := Compile(plan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{
		"thresholds: {",
		"'http_req_failed': [{ threshold: 'rate<0.05', abortOnFail: true }]",
		"'http_req_duration': [{ threshold: 'p(95)<500', abortOnFail: false }]",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q\n%s", want, script)
		}
	}
	if strings.Contains(script, "evil") {
		t.Error("invalid metric names must be dropped")
	}
}
