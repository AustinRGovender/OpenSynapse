// Package compiler transforms a plan JSON document into a valid k6 JavaScript file.
//
// The transformer walks the plan tree and emits k6 JavaScript. Each node type
// has a corresponding emit function. Controllers become if blocks or for loops.
// Transaction controllers become group() calls. Assertions become check() calls.
// Data sources become SharedArray imports. The transformer is stateless and
// deterministic: the same plan always produces the same script bytes.
//
// There are two output modes. Standalone mode (Compile) produces a portable
// script that runs with stock k6 exactly as the plan describes; it is what
// "Show Script" displays. Managed mode (CompileWithOptions with Managed set)
// produces the script OpenSynapse actually runs: the plan's VU-based scenario
// is converted to the externally-controlled executor so the engine can drive
// its load profile and accept live changes, and — when xk6-kv is available —
// an internal control scenario feeds RPS limits and the stop flag to VUs.
// See docs/decisions/0010-run-metrics-and-live-control.md.
package compiler

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// ControlScenario is the name of the internal live-control scenario; the
// engine excludes its samples from results.
const ControlScenario = "__opensynapse_control"

// Options controls how a plan is compiled.
type Options struct {
	// Managed produces the script OpenSynapse runs (see package doc).
	Managed bool
	// KV adds the xk6-kv control scenario and RPS pacing (managed mode only).
	KV bool
	// Overrides are the Run dialog's per-run parameters.
	Overrides *db.RunParameters
	// Instances is how many k6 instances share the run (cluster mode). The
	// control scenario gets one VU per instance so every instance, whatever
	// its execution segment, receives live-control state.
	Instances int
}

// ProfileStage is one step of a managed VU profile.
type ProfileStage struct {
	Duration time.Duration
	Target   int
}

// Profile is the VU schedule the engine drives for a managed scenario.
type Profile struct {
	StartVUs int
	Stages   []ProfileStage
}

// Duration is the profile's total length.
func (p *Profile) Duration() time.Duration {
	var d time.Duration
	for _, s := range p.Stages {
		d += s.Duration
	}
	return d
}

// Peak is the highest VU count in the profile.
func (p *Profile) Peak() int {
	peak := p.StartVUs
	for _, s := range p.Stages {
		if s.Target > peak {
			peak = s.Target
		}
	}
	return peak
}

// Result is the output of CompileWithOptions.
type Result struct {
	Script string
	// Profile is set when a VU-based scenario was converted for engine control.
	Profile *Profile
	// MaxVUs is the VU ceiling the managed scenario starts with.
	MaxVUs int
	// PlannedDuration is the run length the engine enforces (0: executors decide).
	PlannedDuration time.Duration
	// MaxDuration bounds live "extend" requests.
	MaxDuration time.Duration
	// Control reports whether the xk6-kv control scenario was included.
	Control bool
}

// Compile transforms a Plan into a standalone k6 JavaScript string.
func Compile(plan *db.Plan) (string, error) {
	res, err := CompileWithOptions(plan, Options{})
	if err != nil {
		return "", err
	}
	return res.Script, nil
}

// CompileWithOptions compiles a plan in standalone or managed mode.
func CompileWithOptions(plan *db.Plan, opts Options) (*Result, error) {
	c := newCompiler(opts)
	if err := c.compilePlan(plan.Root); err != nil {
		return nil, fmt.Errorf("compile plan: %w", err)
	}
	return c.result(), nil
}

// CompileNode compiles a single root node (useful for testing individual node types).
func CompileNode(node db.Node) (string, error) {
	c := newCompiler(Options{})
	if err := c.emitNode(node, 1); err != nil {
		return "", fmt.Errorf("compile node: %w", err)
	}
	return c.render(), nil
}

func newCompiler(opts Options) *compiler {
	main := &strings.Builder{}
	return &compiler{
		imports:     make(map[string][]string),
		dataSources: &strings.Builder{},
		mainFunc:    main,
		defaultFunc: main,
		opts:        opts,
	}
}

type compiler struct {
	imports     map[string][]string // module -> named imports
	dataSources *strings.Builder    // SharedArray declarations
	// defaultFunc is the function body currently being emitted into.
	defaultFunc *strings.Builder
	// mainFunc collects plan-level nodes that sit outside any scenario.
	mainFunc  *strings.Builder
	scenarios []*scenarioOut

	needsWS     bool
	needsShared bool
	needsExec   bool
	needsVars   bool

	opts       Options
	control    bool // emit the xk6-kv control scenario and pacing
	thresholds []threshold
	result_    Result
}

type scenarioOut struct {
	key     string // k6 scenario key
	fn      string // exported exec function name
	options []string
	body    *strings.Builder
}

// addImport registers a named import from a module, deduplicating.
func (c *compiler) addImport(module, name string) {
	for _, existing := range c.imports[module] {
		if existing == name {
			return
		}
	}
	c.imports[module] = append(c.imports[module], name)
}

// indent returns n levels of indentation.
func indent(n int) string {
	return strings.Repeat("  ", n)
}

type scenarioProps struct {
	Executor     string  `json:"executor"`
	VUs          int     `json:"vus"`
	StartVUs     *int    `json:"start_vus"`
	MaxVUs       int     `json:"max_vus"`
	Duration     string  `json:"duration"`
	Iterations   int     `json:"iterations"`
	Rate         int     `json:"rate"`
	TimeUnit     string  `json:"time_unit"`
	PreAllocVUs  int     `json:"pre_allocated_vus"`
	GracefulStop string  `json:"graceful_stop"`
	Stages       []stage `json:"stages"`
}

func isVUBased(executor string) bool {
	switch executor {
	case "constant-vus", "ramping-vus", "externally-controlled":
		return true
	}
	return false
}

// compilePlan handles the root plan node.
func (c *compiler) compilePlan(root db.Node) error {
	if root.Type != "plan" {
		return fmt.Errorf("expected root node type 'plan', got %q", root.Type)
	}
	var planProps struct {
		Thresholds []threshold `json:"thresholds"`
	}
	if len(root.Properties) > 0 {
		json.Unmarshal(root.Properties, &planProps)
	}
	for _, t := range planProps.Thresholds {
		if thresholdMetricRe.MatchString(t.Metric) && strings.TrimSpace(t.Condition) != "" {
			c.thresholds = append(c.thresholds, t)
		}
	}

	// Pass 1: data sources and scenario discovery.
	var scenarios []db.Node
	var loose []db.Node
	for _, child := range root.Children {
		if !child.Enabled {
			continue
		}
		switch child.Type {
		case "data-source":
			if err := c.emitDataSource(child); err != nil {
				return err
			}
		case "environment-binding":
			// Environment bindings are informational; variables arrive via __ENV.
		case "scenario":
			scenarios = append(scenarios, child)
		default:
			loose = append(loose, child)
		}
	}

	// In managed mode exactly one VU-based scenario is handed to the engine.
	managedIdx := -1
	if c.opts.Managed {
		vuBased := 0
		for i, s := range scenarios {
			var p scenarioProps
			if err := json.Unmarshal(s.Properties, &p); err != nil {
				return fmt.Errorf("unmarshal scenario properties: %w", err)
			}
			if isVUBased(p.Executor) {
				vuBased++
				if managedIdx < 0 {
					managedIdx = i
				}
			}
		}
		if vuBased > 1 {
			managedIdx = -1 // k6 allows one externally-controlled scenario; keep native executors
		}
	}

	// A plan without scenarios runs its nodes in one synthesized scenario.
	if len(scenarios) == 0 {
		if c.opts.Managed {
			props, _ := json.Marshal(map[string]any{"executor": "constant-vus", "vus": 1, "duration": "30s"})
			scenarios = append(scenarios, db.Node{ID: "default", Type: "scenario", Name: "default", Enabled: true, Properties: props, Children: loose})
			managedIdx = 0
			loose = nil
		} else {
			for _, n := range loose {
				if err := c.emitNode(n, 1); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// The control scenario only makes sense when the engine owns the run's
	// end time; otherwise it would keep a native-executor test alive.
	c.control = c.opts.Managed && c.opts.KV && managedIdx >= 0

	for i, s := range scenarios {
		var children []db.Node
		if i == 0 {
			// Nodes outside any scenario run as part of the first scenario.
			children = append(children, loose...)
		}
		children = append(children, s.Children...)
		if err := c.emitScenario(s, children, i == managedIdx); err != nil {
			return err
		}
	}

	if c.control {
		dur := c.result_.MaxDuration
		c.scenarios = append(c.scenarios, &scenarioOut{
			key: ControlScenario,
			fn:  "__osControl",
			options: []string{
				"executor: 'constant-vus'",
				fmt.Sprintf("vus: %d", max(1, c.opts.Instances)),
				fmt.Sprintf("duration: '%ds'", int(dur.Seconds())),
				"gracefulStop: '0s'",
				"exec: '__osControl'",
			},
		})
		c.result_.Control = true
		c.addImport("k6/http", "")
		c.addImport("k6", "sleep")
		c.needsExec = true
	}
	return nil
}

// emitScenario records a scenario's options and emits its body into its own
// exec function.
func (c *compiler) emitScenario(node db.Node, children []db.Node, managed bool) error {
	var props scenarioProps
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal scenario properties: %w", err)
	}
	if ov := c.opts.Overrides; ov != nil && c.opts.Managed && !managed {
		applyNativeOverrides(&props, ov)
	}

	key := sanitizeScenarioName(node.Name)
	for _, s := range c.scenarios {
		if s.key == key {
			key = fmt.Sprintf("%s-%d", key, len(c.scenarios)+1)
		}
	}
	out := &scenarioOut{key: key, fn: "scenario_" + strings.ReplaceAll(sanitizeVarName(key), "-", "_"), body: &strings.Builder{}}
	opt := func(format string, a ...any) { out.options = append(out.options, fmt.Sprintf(format, a...)) }

	if managed {
		profile, err := profileFor(props, c.opts.Overrides)
		if err != nil {
			return err
		}
		planned := profile.Duration()
		maxDur := MaxDurationFor(planned)
		maxVUs := profile.Peak()
		if props.MaxVUs > maxVUs {
			maxVUs = props.MaxVUs
		}
		if maxVUs < 1 {
			maxVUs = 1
		}
		c.result_.Profile = profile
		c.result_.MaxVUs = maxVUs
		c.result_.PlannedDuration = planned
		c.result_.MaxDuration = maxDur
		opt("executor: 'externally-controlled'")
		opt("vus: %d", profile.StartVUs)
		opt("maxVUs: %d", maxVUs)
		opt("duration: '%ds'", int(math.Ceil(maxDur.Seconds())))
	} else {
		opt("executor: '%s'", props.Executor)
		switch props.Executor {
		case "externally-controlled":
			if props.VUs > 0 {
				opt("vus: %d", props.VUs)
			}
			if props.MaxVUs > 0 {
				opt("maxVUs: %d", props.MaxVUs)
			}
			if props.Duration != "" {
				opt("duration: '%s'", sanitizeJSString(props.Duration))
			}
		case "ramping-vus":
			if props.StartVUs != nil {
				opt("startVUs: %d", *props.StartVUs)
			}
			if props.Stages != nil {
				opt("stages: [%s]", renderStages(props.Stages))
			}
			if props.GracefulStop != "" {
				opt("gracefulStop: '%s'", sanitizeJSString(props.GracefulStop))
			}
		case "constant-vus":
			if props.VUs > 0 {
				opt("vus: %d", props.VUs)
			}
			if props.Duration != "" {
				opt("duration: '%s'", sanitizeJSString(props.Duration))
			}
		case "constant-arrival-rate":
			if props.Rate > 0 {
				opt("rate: %d", props.Rate)
			}
			if props.TimeUnit != "" {
				opt("timeUnit: '%s'", sanitizeJSString(props.TimeUnit))
			}
			if props.Duration != "" {
				opt("duration: '%s'", sanitizeJSString(props.Duration))
			}
			if props.PreAllocVUs > 0 {
				opt("preAllocatedVUs: %d", props.PreAllocVUs)
			}
			if props.MaxVUs > 0 {
				opt("maxVUs: %d", props.MaxVUs)
			}
		case "ramping-arrival-rate":
			if props.Stages != nil {
				opt("stages: [%s]", renderStages(props.Stages))
			}
			if props.TimeUnit != "" {
				opt("timeUnit: '%s'", sanitizeJSString(props.TimeUnit))
			}
			if props.PreAllocVUs > 0 {
				opt("preAllocatedVUs: %d", props.PreAllocVUs)
			}
			if props.MaxVUs > 0 {
				opt("maxVUs: %d", props.MaxVUs)
			}
		case "shared-iterations", "per-vu-iterations":
			if props.VUs > 0 {
				opt("vus: %d", props.VUs)
			}
			if props.Iterations > 0 {
				opt("iterations: %d", props.Iterations)
			}
		}
	}
	opt("exec: '%s'", out.fn)
	c.scenarios = append(c.scenarios, out)

	prev := c.defaultFunc
	c.defaultFunc = out.body
	defer func() { c.defaultFunc = prev }()
	for _, child := range children {
		if !child.Enabled {
			continue
		}
		if err := c.emitNode(child, 1); err != nil {
			return err
		}
	}
	return nil
}

// applyNativeOverrides maps Run-dialog parameters onto executors the engine
// does not drive (arrival-rate and iteration-based).
func applyNativeOverrides(p *scenarioProps, ov *db.RunParameters) {
	switch p.Executor {
	case "constant-arrival-rate":
		if ov.RPSTarget > 0 {
			p.Rate = ov.RPSTarget
			p.TimeUnit = "1s"
		}
		if ov.DurationSeconds > 0 {
			p.Duration = fmt.Sprintf("%ds", ov.DurationSeconds)
		}
		if ov.VUsTarget > 0 {
			p.PreAllocVUs = ov.VUsTarget
			if p.MaxVUs < ov.VUsTarget {
				p.MaxVUs = ov.VUsTarget
			}
		}
	case "ramping-arrival-rate":
		if len(ov.Stages) > 0 {
			p.Stages = nil
			for _, s := range ov.Stages {
				p.Stages = append(p.Stages, stage{Duration: s.Duration, Target: s.Target})
			}
		}
		if ov.VUsTarget > 0 {
			p.PreAllocVUs = ov.VUsTarget
			if p.MaxVUs < ov.VUsTarget {
				p.MaxVUs = ov.VUsTarget
			}
		}
	case "shared-iterations", "per-vu-iterations":
		if ov.VUsTarget > 0 {
			p.VUs = ov.VUsTarget
		}
	}
}

// profileFor builds the engine-driven VU profile for a VU-based scenario,
// applying the Run dialog's overrides when present.
func profileFor(p scenarioProps, ov *db.RunParameters) (*Profile, error) {
	if ov != nil && len(ov.Stages) > 0 {
		prof := &Profile{}
		for _, s := range ov.Stages {
			d, err := ParseDuration(s.Duration)
			if err != nil {
				return nil, fmt.Errorf("stage duration: %w", err)
			}
			prof.Stages = append(prof.Stages, ProfileStage{Duration: d, Target: s.Target})
		}
		return prof, nil
	}

	var prof *Profile
	switch p.Executor {
	case "ramping-vus":
		prof = &Profile{}
		if p.StartVUs != nil {
			prof.StartVUs = *p.StartVUs
		}
		for _, s := range p.Stages {
			d, err := ParseDuration(s.Duration)
			if err != nil {
				return nil, fmt.Errorf("stage duration: %w", err)
			}
			prof.Stages = append(prof.Stages, ProfileStage{Duration: d, Target: s.Target})
		}
	default: // constant-vus, externally-controlled
		vus := p.VUs
		if vus <= 0 {
			vus = 1
		}
		d := 30 * time.Second
		if p.Duration != "" {
			parsed, err := ParseDuration(p.Duration)
			if err != nil {
				return nil, fmt.Errorf("scenario duration: %w", err)
			}
			d = parsed
		}
		prof = &Profile{StartVUs: vus, Stages: []ProfileStage{{Duration: d, Target: vus}}}
	}

	if ov != nil && (ov.VUsTarget > 0 || ov.DurationSeconds > 0) {
		vus := ov.VUsTarget
		if vus <= 0 {
			vus = prof.Peak()
		}
		d := prof.Duration()
		if ov.DurationSeconds > 0 {
			d = time.Duration(ov.DurationSeconds) * time.Second
		}
		prof = &Profile{StartVUs: vus, Stages: []ProfileStage{{Duration: d, Target: vus}}}
	}
	if prof.Duration() <= 0 {
		return nil, fmt.Errorf("the scenario's load profile has no duration; set a duration or stages")
	}
	return prof, nil
}

func renderStages(stages []stage) string {
	parts := make([]string, 0, len(stages))
	for _, s := range stages {
		parts = append(parts, fmt.Sprintf("{ duration: '%s', target: %d }", sanitizeJSString(s.Duration), s.Target))
	}
	return strings.Join(parts, ", ")
}

// emitNode dispatches to the appropriate emit function based on node type.
func (c *compiler) emitNode(node db.Node, depth int) error {
	if !node.Enabled {
		return nil
	}

	switch node.Type {
	case "http":
		return c.emitHTTP(node, depth)
	case "websocket":
		return c.emitWebSocket(node, depth)
	case "code-block":
		return c.emitCodeBlock(node, depth)
	case "if-controller":
		return c.emitIfController(node, depth)
	case "else-controller":
		return c.emitElseController(node, depth)
	case "loop-controller":
		return c.emitLoopController(node, depth)
	case "transaction-controller":
		return c.emitTransactionController(node, depth)
	case "once-only-controller":
		return c.emitOnceOnlyController(node, depth)
	case "random-controller":
		return c.emitRandomController(node, depth)
	case "assertion":
		return c.emitAssertion(node, depth)
	case "timer":
		return c.emitTimer(node, depth)
	case "data-source":
		return c.emitDataSource(node)
	case "environment-binding":
		// No code emitted for environment bindings.
		return nil
	default:
		return fmt.Errorf("unknown node type %q", node.Type)
	}
}

// expectedStatuses returns non-2xx/3xx status codes that the request's own
// assertions say are expected, so k6 doesn't count them as failed requests.
func expectedStatuses(children []db.Node) []int {
	var out []int
	for _, ch := range children {
		if !ch.Enabled || ch.Type != "assertion" {
			continue
		}
		var a struct {
			Target    string `json:"target"`
			Condition string `json:"condition"`
			Value     any    `json:"value"`
			Negate    bool   `json:"negate"`
		}
		if json.Unmarshal(ch.Properties, &a) != nil || a.Target != "status" || a.Condition != "equals" || a.Negate {
			continue
		}
		var code int
		switch v := a.Value.(type) {
		case float64:
			code = int(v)
		case string:
			fmt.Sscanf(v, "%d", &code)
		}
		if code >= 400 && code <= 599 {
			out = append(out, code)
		}
	}
	return out
}

// emitHTTP generates an http.get/post/put/patch/delete/head/options call.
func (c *compiler) emitHTTP(node db.Node, depth int) error {
	var props struct {
		Method          string        `json:"method"`
		URL             string        `json:"url"`
		Headers         []headerEntry `json:"headers"`
		Body            httpBody      `json:"body"`
		Timeout         string        `json:"timeout"`
		FollowRedirects *bool         `json:"follow_redirects"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal http properties: %w", err)
	}

	c.addImport("k6/http", "")

	ind := indent(depth)
	df := c.defaultFunc
	id := sanitizeVarName(node.ID)

	// Comment with node name.
	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))

	expected := expectedStatuses(node.Children)

	// Build params object if we have headers, timeout, redirects config, or expected statuses.
	hasParams := len(props.Headers) > 0 || props.Timeout != "" || props.FollowRedirects != nil || len(expected) > 0
	if hasParams {
		df.WriteString(fmt.Sprintf("%slet params_%s = {\n", ind, id))
		if len(props.Headers) > 0 {
			df.WriteString(fmt.Sprintf("%s  headers: {\n", ind))
			for _, h := range props.Headers {
				if strings.TrimSpace(h.Key) == "" {
					continue
				}
				df.WriteString(fmt.Sprintf("%s    '%s': %s,\n", ind, sanitizeJSString(h.Key), c.resolveVarRefs(h.Value)))
			}
			df.WriteString(fmt.Sprintf("%s  },\n", ind))
		}
		if props.Timeout != "" {
			df.WriteString(fmt.Sprintf("%s  timeout: '%s',\n", ind, sanitizeJSString(props.Timeout)))
		}
		if props.FollowRedirects != nil {
			if *props.FollowRedirects {
				df.WriteString(fmt.Sprintf("%s  redirects: 10,\n", ind))
			} else {
				df.WriteString(fmt.Sprintf("%s  redirects: 0,\n", ind))
			}
		}
		if len(expected) > 0 {
			codes := make([]string, len(expected))
			for i, code := range expected {
				codes[i] = fmt.Sprint(code)
			}
			df.WriteString(fmt.Sprintf("%s  responseCallback: http.expectedStatuses({ min: 200, max: 399 }, %s),\n", ind, strings.Join(codes, ", ")))
		}
		df.WriteString(fmt.Sprintf("%s};\n", ind))
	}

	url := c.resolveVarRefs(props.URL)
	method := strings.ToLower(props.Method)
	if method == "" {
		method = "get"
	}

	if c.control {
		df.WriteString(fmt.Sprintf("%s__osPace();\n", ind))
	}

	paramsArg := ""
	if hasParams {
		paramsArg = ", params_" + id
	}
	switch method {
	case "get", "head", "options":
		df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s%s);\n", ind, id, method, url, paramsArg))
	case "delete":
		// http.del takes (url, body, params).
		if hasParams {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.del(%s, null%s);\n", ind, id, url, paramsArg))
		} else {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.del(%s);\n", ind, id, url))
		}
	case "post", "put", "patch":
		bodyStr := c.resolveHTTPBody(props.Body)
		df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s, %s%s);\n", ind, id, method, url, bodyStr, paramsArg))
	default:
		return fmt.Errorf("unsupported HTTP method %q", props.Method)
	}
	// `res` always refers to the most recent response, for If conditions,
	// code blocks, and assertions placed on controllers.
	df.WriteString(fmt.Sprintf("%sres = res_%s;\n", ind, id))

	// Emit children (assertions, timers) that reference the response.
	for _, child := range node.Children {
		if !child.Enabled {
			continue
		}
		if child.Type == "assertion" {
			if err := c.emitAssertionForResponse(child, depth, "res_"+id); err != nil {
				return err
			}
		} else {
			if err := c.emitNode(child, depth); err != nil {
				return err
			}
		}
	}

	return nil
}

// emitWebSocket generates a ws.connect() call.
func (c *compiler) emitWebSocket(node db.Node, depth int) error {
	var props struct {
		URL                string              `json:"url"`
		ConnectTimeout     string              `json:"connect_timeout"`
		Messages           []wsMessage         `json:"messages"`
		ExpectedMessages   []wsExpectedMessage `json:"expected_messages"`
		DisconnectBehavior string              `json:"disconnect_behavior"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal websocket properties: %w", err)
	}

	c.needsWS = true
	c.addImport("k6/ws", "")

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))

	url := c.resolveVarRefs(props.URL)

	paramsStr := "null"
	if props.ConnectTimeout != "" {
		paramsStr = fmt.Sprintf("{ timeout: '%s' }", sanitizeJSString(props.ConnectTimeout))
	}

	df.WriteString(fmt.Sprintf("%slet res_%s = ws.connect(%s, %s, function (socket) {\n",
		ind, sanitizeVarName(node.ID), url, paramsStr))

	innerInd := indent(depth + 1)

	// On open: send messages.
	if len(props.Messages) > 0 {
		df.WriteString(fmt.Sprintf("%ssocket.on('open', function () {\n", innerInd))
		for _, msg := range props.Messages {
			df.WriteString(fmt.Sprintf("%s  socket.send(%s);\n", innerInd, c.resolveVarRefs(msg.Data)))
		}
		df.WriteString(fmt.Sprintf("%s});\n", innerInd))
	}

	// On message: check expected messages.
	if len(props.ExpectedMessages) > 0 {
		df.WriteString(fmt.Sprintf("%ssocket.on('message', function (data) {\n", innerInd))
		for _, em := range props.ExpectedMessages {
			switch em.MatchType {
			case "contains":
				c.addImport("k6", "check")
				df.WriteString(fmt.Sprintf("%s  check(data, {\n", innerInd))
				df.WriteString(fmt.Sprintf("%s    'ws message contains %s': (d) => d.includes('%s'),\n",
					innerInd, sanitizeJSString(em.Value), sanitizeJSString(em.Value)))
				df.WriteString(fmt.Sprintf("%s  });\n", innerInd))
			case "exact":
				c.addImport("k6", "check")
				df.WriteString(fmt.Sprintf("%s  check(data, {\n", innerInd))
				df.WriteString(fmt.Sprintf("%s    'ws message equals %s': (d) => d === '%s',\n",
					innerInd, sanitizeJSString(em.Value), sanitizeJSString(em.Value)))
				df.WriteString(fmt.Sprintf("%s  });\n", innerInd))
			case "regex":
				c.addImport("k6", "check")
				df.WriteString(fmt.Sprintf("%s  check(data, {\n", innerInd))
				df.WriteString(fmt.Sprintf("%s    'ws message matches %s': (d) => new RegExp('%s').test(d),\n",
					innerInd, sanitizeJSString(em.Value), sanitizeJSString(em.Value)))
				df.WriteString(fmt.Sprintf("%s  });\n", innerInd))
			}
		}
		df.WriteString(fmt.Sprintf("%s});\n", innerInd))
	}

	// Set a timeout to close the socket.
	if len(props.ExpectedMessages) > 0 {
		df.WriteString(fmt.Sprintf("%ssocket.setTimeout(function () {\n", innerInd))
		df.WriteString(fmt.Sprintf("%s  socket.close();\n", innerInd))
		df.WriteString(fmt.Sprintf("%s}, 30000);\n", innerInd))
	} else {
		df.WriteString(fmt.Sprintf("%ssocket.close();\n", innerInd))
	}

	df.WriteString(fmt.Sprintf("%s});\n", ind))

	return nil
}

// emitCodeBlock emits raw JavaScript code verbatim.
func (c *compiler) emitCodeBlock(node db.Node, depth int) error {
	var props struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal code-block properties: %w", err)
	}

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))
	if strings.Contains(props.Code, "__vars") || strings.Contains(props.Code, "__v(") {
		c.needsVars = true
	}
	// Emit each line of the code block with proper indentation.
	lines := strings.Split(props.Code, "\n")
	for _, line := range lines {
		df.WriteString(fmt.Sprintf("%s%s\n", ind, line))
	}

	return nil
}

// emitIfController generates an if (condition) { ... } block.
func (c *compiler) emitIfController(node db.Node, depth int) error {
	var props struct {
		Condition string `json:"condition"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal if-controller properties: %w", err)
	}
	cond := strings.TrimSpace(props.Condition)
	if cond == "" {
		cond = "true"
	}

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))
	df.WriteString(fmt.Sprintf("%sif (%s) {\n", ind, cond))

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s}", ind))
	// Do not add newline here -- else-controller may follow.
	df.WriteString("\n")

	return nil
}

// emitElseController generates an else { ... } block.
func (c *compiler) emitElseController(node db.Node, depth int) error {
	ind := indent(depth)
	df := c.defaultFunc

	// Re-open the preceding if block: drop its trailing newline.
	s := df.String()
	if strings.HasSuffix(s, "}\n") {
		df.Reset()
		df.WriteString(strings.TrimSuffix(s, "\n"))
	}
	df.WriteString(" else {\n")

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s}\n", ind))

	return nil
}

// emitLoopController generates a for loop (fixed count) or while loop.
func (c *compiler) emitLoopController(node db.Node, depth int) error {
	var props struct {
		Count          int    `json:"count"`
		WhileCondition string `json:"while_condition"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal loop-controller properties: %w", err)
	}

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))

	if props.WhileCondition != "" {
		df.WriteString(fmt.Sprintf("%swhile (%s) {\n", ind, props.WhileCondition))
	} else {
		df.WriteString(fmt.Sprintf("%sfor (let i = 0; i < %d; i++) {\n", ind, props.Count))
	}

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s}\n", ind))

	return nil
}

// emitTransactionController generates a group('name', function() { ... }) call.
func (c *compiler) emitTransactionController(node db.Node, depth int) error {
	c.addImport("k6", "group")

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))
	df.WriteString(fmt.Sprintf("%sgroup('%s', function () {\n", ind, sanitizeJSString(node.Name)))

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s});\n", ind))

	return nil
}

// emitOnceOnlyController generates an if (__ITER === 0) { ... } block.
func (c *compiler) emitOnceOnlyController(node db.Node, depth int) error {
	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))
	df.WriteString(fmt.Sprintf("%sif (__ITER === 0) {\n", ind))

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s}\n", ind))

	return nil
}

// emitRandomController generates weighted random selection logic.
func (c *compiler) emitRandomController(node db.Node, depth int) error {
	var props struct {
		Weights []int `json:"weights"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal random-controller properties: %w", err)
	}

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, sanitizeComment(node.Name)))

	enabledChildren := make([]db.Node, 0, len(node.Children))
	for _, child := range node.Children {
		if child.Enabled {
			enabledChildren = append(enabledChildren, child)
		}
	}

	if len(enabledChildren) == 0 {
		return nil
	}

	// If we have weights, use weighted random selection.
	if len(props.Weights) > 0 && len(props.Weights) == len(enabledChildren) {
		// Calculate total weight.
		totalWeight := 0
		for _, w := range props.Weights {
			totalWeight += w
		}

		df.WriteString(fmt.Sprintf("%s{\n", ind))
		df.WriteString(fmt.Sprintf("%s  let __rand = Math.random() * %d;\n", ind, totalWeight))

		cumulative := 0
		for i, child := range enabledChildren {
			cumulative += props.Weights[i]
			if i == 0 {
				df.WriteString(fmt.Sprintf("%s  if (__rand < %d) {\n", ind, cumulative))
			} else if i == len(enabledChildren)-1 {
				df.WriteString(fmt.Sprintf("%s  } else {\n", ind))
			} else {
				df.WriteString(fmt.Sprintf("%s  } else if (__rand < %d) {\n", ind, cumulative))
			}
			if err := c.emitNode(child, depth+2); err != nil {
				return err
			}
		}
		df.WriteString(fmt.Sprintf("%s  }\n", ind))
		df.WriteString(fmt.Sprintf("%s}\n", ind))
	} else {
		// Uniform random selection.
		df.WriteString(fmt.Sprintf("%s{\n", ind))
		df.WriteString(fmt.Sprintf("%s  let __randIdx = Math.floor(Math.random() * %d);\n", ind, len(enabledChildren)))

		for i, child := range enabledChildren {
			if i == 0 {
				df.WriteString(fmt.Sprintf("%s  if (__randIdx === %d) {\n", ind, i))
			} else if i == len(enabledChildren)-1 {
				df.WriteString(fmt.Sprintf("%s  } else {\n", ind))
			} else {
				df.WriteString(fmt.Sprintf("%s  } else if (__randIdx === %d) {\n", ind, i))
			}
			if err := c.emitNode(child, depth+2); err != nil {
				return err
			}
		}
		df.WriteString(fmt.Sprintf("%s  }\n", ind))
		df.WriteString(fmt.Sprintf("%s}\n", ind))
	}

	return nil
}

// emitAssertion generates a check() against the most recent response (`res`).
func (c *compiler) emitAssertion(node db.Node, depth int) error {
	return c.emitAssertionForResponse(node, depth, "res")
}

// emitAssertionForResponse generates a check() call referencing a specific response variable.
func (c *compiler) emitAssertionForResponse(node db.Node, depth int, resVar string) error {
	var props struct {
		Target     string      `json:"target"`
		Condition  string      `json:"condition"`
		Value      interface{} `json:"value"`
		Negate     bool        `json:"negate"`
		HeaderName string      `json:"header_name"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal assertion properties: %w", err)
	}

	c.addImport("k6", "check")

	ind := indent(depth)
	df := c.defaultFunc

	checkName := sanitizeJSString(node.Name)
	expr := buildCheckExpression(props.Target, props.Condition, props.Value, props.Negate, props.HeaderName)

	if resVar == "res" {
		// A standalone assertion with no preceding request in this function.
		df.WriteString(fmt.Sprintf("%sif (res) {\n", ind))
		df.WriteString(fmt.Sprintf("%s  check(res, {\n", ind))
		df.WriteString(fmt.Sprintf("%s    '%s': (r) => %s,\n", ind, checkName, expr))
		df.WriteString(fmt.Sprintf("%s  });\n", ind))
		df.WriteString(fmt.Sprintf("%s}\n", ind))
		return nil
	}
	df.WriteString(fmt.Sprintf("%scheck(%s, {\n", ind, resVar))
	df.WriteString(fmt.Sprintf("%s  '%s': (r) => %s,\n", ind, checkName, expr))
	df.WriteString(fmt.Sprintf("%s});\n", ind))

	return nil
}

// buildCheckExpression builds the JavaScript expression for a check assertion.
func buildCheckExpression(target, condition string, value interface{}, negate bool, headerName string) string {
	var accessor string
	switch target {
	case "status":
		accessor = "r.status"
	case "body":
		accessor = "String(r.body)"
	case "header":
		accessor = fmt.Sprintf("r.headers['%s']", sanitizeJSString(headerName))
	case "response_time":
		accessor = "r.timings.duration"
	default:
		accessor = "r.status"
	}

	numeric := func(v interface{}) string {
		switch n := v.(type) {
		case float64:
			return fmt.Sprintf("%v", n)
		case string:
			var f float64
			if _, err := fmt.Sscanf(n, "%g", &f); err == nil {
				return fmt.Sprintf("%v", f)
			}
		}
		return "0"
	}

	var expr string
	switch condition {
	case "equals":
		switch v := value.(type) {
		case float64:
			expr = fmt.Sprintf("%s === %s", accessor, numeric(v))
		case string:
			if target == "status" {
				expr = fmt.Sprintf("%s === %s", accessor, numeric(v))
			} else {
				expr = fmt.Sprintf("%s === '%s'", accessor, sanitizeJSString(v))
			}
		case bool:
			expr = fmt.Sprintf("%s === %v", accessor, v)
		default:
			expr = fmt.Sprintf("%s === '%s'", accessor, sanitizeJSString(fmt.Sprintf("%v", v)))
		}
	case "contains":
		expr = fmt.Sprintf("String(%s).includes('%s')", accessor, sanitizeJSString(fmt.Sprintf("%v", value)))
	case "matches":
		expr = fmt.Sprintf("new RegExp('%s').test(%s)", sanitizeJSString(fmt.Sprintf("%v", value)), accessor)
	case "jsonpath":
		// For jsonpath, check that the path exists in the parsed JSON.
		expr = fmt.Sprintf("(() => { try { return JSON.parse(r.body)%s !== undefined; } catch (e) { return false; } })()", jsonPathToJS(fmt.Sprintf("%v", value)))
	case "less_than":
		expr = fmt.Sprintf("%s < %s", accessor, numeric(value))
	case "greater_than":
		expr = fmt.Sprintf("%s > %s", accessor, numeric(value))
	case "exists":
		expr = fmt.Sprintf("%s !== undefined && %s !== null", accessor, accessor)
	default:
		expr = fmt.Sprintf("%s === '%s'", accessor, sanitizeJSString(fmt.Sprintf("%v", value)))
	}

	if negate {
		expr = "!(" + expr + ")"
	}

	return expr
}

// emitTimer generates a sleep() call.
func (c *compiler) emitTimer(node db.Node, depth int) error {
	var props struct {
		TimerType   string `json:"timer_type"`
		DurationMs  int    `json:"duration_ms"`
		MinMs       int    `json:"min_ms"`
		MaxMs       int    `json:"max_ms"`
		MeanMs      int    `json:"mean_ms"`
		DeviationMs int    `json:"deviation_ms"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal timer properties: %w", err)
	}

	c.addImport("k6", "sleep")

	ind := indent(depth)
	df := c.defaultFunc

	switch props.TimerType {
	case "constant":
		seconds := float64(props.DurationMs) / 1000.0
		df.WriteString(fmt.Sprintf("%ssleep(%.3f);\n", ind, seconds))
	case "uniform_random":
		minSec := float64(props.MinMs) / 1000.0
		maxSec := float64(props.MaxMs) / 1000.0
		df.WriteString(fmt.Sprintf("%ssleep(%.3f + Math.random() * %.3f);\n", ind, minSec, maxSec-minSec))
	case "gaussian":
		meanSec := float64(props.MeanMs) / 1000.0
		devSec := float64(props.DeviationMs) / 1000.0
		// Box-Muller transform for Gaussian random.
		df.WriteString(fmt.Sprintf("%ssleep(Math.max(0, %.3f + %.3f * Math.sqrt(-2 * Math.log(Math.random() || 1e-9)) * Math.cos(2 * Math.PI * Math.random())));\n",
			ind, meanSec, devSec))
	default:
		seconds := float64(props.DurationMs) / 1000.0
		df.WriteString(fmt.Sprintf("%ssleep(%.3f);\n", ind, seconds))
	}

	return nil
}

// emitDataSource generates a SharedArray declaration at the top of the file.
func (c *compiler) emitDataSource(node db.Node) error {
	var props struct {
		SourceType       string `json:"source_type"`
		Path             string `json:"path"`
		Delimiter        string `json:"delimiter"`
		Sharing          string `json:"sharing"`
		VariableName     string `json:"variable_name"`
		FirstRowIsHeader bool   `json:"first_row_is_header"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal data-source properties: %w", err)
	}

	c.needsShared = true

	ds := c.dataSources
	varName := props.VariableName
	if varName == "" {
		varName = sanitizeVarName(node.ID)
	} else {
		varName = sanitizeVarName(varName)
	}

	switch props.SourceType {
	case "csv":
		ds.WriteString(fmt.Sprintf("const %s = new SharedArray('%s', function () {\n", varName, sanitizeJSString(node.Name)))
		ds.WriteString(fmt.Sprintf("  return open('%s').split('\\n').slice(1).map(function (line) {\n", sanitizeJSString(props.Path)))
		ds.WriteString("    let fields = line.split(',');\n")
		ds.WriteString("    return { _raw: fields };\n")
		ds.WriteString("  }).filter(function (row) { return row._raw[0] !== ''; });\n")
		ds.WriteString("});\n\n")
	case "json":
		ds.WriteString(fmt.Sprintf("const %s = new SharedArray('%s', function () {\n", varName, sanitizeJSString(node.Name)))
		ds.WriteString(fmt.Sprintf("  return JSON.parse(open('%s'));\n", sanitizeJSString(props.Path)))
		ds.WriteString("});\n\n")
	case "inline":
		ds.WriteString(fmt.Sprintf("const %s = new SharedArray('%s', function () {\n", varName, sanitizeJSString(node.Name)))
		ds.WriteString("  return JSON.parse(open('./inline-data.json'));\n")
		ds.WriteString("});\n\n")
	}

	return nil
}

func (c *compiler) result() *Result {
	r := c.result_
	r.Script = c.render()
	return &r
}

// render assembles the final k6 script from all collected parts.
func (c *compiler) render() string {
	var sb strings.Builder

	// Imports.
	if _, ok := c.imports["k6/http"]; ok {
		sb.WriteString("import http from 'k6/http';\n")
		delete(c.imports, "k6/http")
	}
	if c.needsWS {
		sb.WriteString("import ws from 'k6/ws';\n")
		delete(c.imports, "k6/ws")
	}
	if names, ok := c.imports["k6"]; ok && len(names) > 0 {
		sorted := append([]string(nil), dedup(names)...)
		sort.Strings(sorted)
		sb.WriteString(fmt.Sprintf("import { %s } from 'k6';\n", strings.Join(sorted, ", ")))
		delete(c.imports, "k6")
	}
	if c.needsShared {
		sb.WriteString("import { SharedArray } from 'k6/data';\n")
	}
	if c.needsExec || c.hasExternallyControlled() {
		sb.WriteString("import exec from 'k6/execution';\n")
	}
	if c.control {
		sb.WriteString("import { openKv } from 'k6/x/kv';\n")
	}
	remainingMods := make([]string, 0, len(c.imports))
	for mod := range c.imports {
		if len(c.imports[mod]) > 0 {
			remainingMods = append(remainingMods, mod)
		}
	}
	sort.Strings(remainingMods)
	for _, mod := range remainingMods {
		sb.WriteString(fmt.Sprintf("import { %s } from '%s';\n", strings.Join(dedup(c.imports[mod]), ", "), mod))
	}
	sb.WriteString("\n")

	if c.dataSources.Len() > 0 {
		sb.WriteString(c.dataSources.String())
	}

	if len(c.scenarios) > 0 || len(c.thresholds) > 0 {
		sb.WriteString("export const options = {\n")
		if len(c.thresholds) > 0 {
			sb.WriteString(renderThresholds(c.thresholds))
		}
		if len(c.scenarios) > 0 {
			sb.WriteString("  scenarios: {\n")
			for _, s := range c.scenarios {
				sb.WriteString(fmt.Sprintf("    '%s': {\n", s.key))
				for _, o := range s.options {
					sb.WriteString("      " + o + ",\n")
				}
				sb.WriteString("    },\n")
			}
			sb.WriteString("  },\n")
		}
		sb.WriteString("};\n\n")
	}

	if c.needsVars {
		sb.WriteString(varsHelper)
		sb.WriteString("\n")
	}
	if c.control {
		sb.WriteString(controlHelper)
		sb.WriteString("\n")
	}

	writeFunc := func(name string, body string) {
		prefix := "export function " + name + " () {\n"
		if name == "default" {
			prefix = "export default function () {\n"
		}
		if c.control {
			prefix = strings.Replace(prefix, "function", "async function", 1)
		}
		sb.WriteString(prefix)
		if c.control {
			sb.WriteString("  if (!(await __osSync())) return;\n")
		}
		sb.WriteString("  let res = null;\n")
		sb.WriteString(body)
		sb.WriteString("}\n")
	}

	wrote := false
	for _, s := range c.scenarios {
		if s.body == nil {
			continue // internal scenario with its own helper function
		}
		if wrote {
			sb.WriteString("\n")
		}
		writeFunc(s.fn, s.body.String())
		wrote = true
	}
	if !wrote || c.mainFunc.Len() > 0 {
		if wrote {
			sb.WriteString("\n")
		}
		writeFunc("default", c.mainFunc.String())
	}

	return sb.String()
}

func (c *compiler) hasExternallyControlled() bool {
	for _, s := range c.scenarios {
		for _, o := range s.options {
			if o == "executor: 'externally-controlled'" {
				return true
			}
		}
	}
	return false
}

// varsHelper resolves ${NAME} references: values set by code blocks
// (`__vars.NAME = ...`) win, then environment variables, then empty string.
const varsHelper = `// Variables: code blocks may set __vars.NAME; environments arrive as __ENV.
const __vars = {};
function __v(name) {
  if (__vars[name] !== undefined) return __vars[name];
  if (__ENV[name] !== undefined) return __ENV[name];
  return '';
}
`

// controlHelper is the live-control glue for managed runs. The control
// scenario polls the control plane once a second and publishes the RPS
// target and stop flag to an xk6-kv store; every iteration reads them, and
// __osPace() spaces this VU's requests so all VUs together stay at or under
// the target. See docs/decisions/0010-run-metrics-and-live-control.md.
const controlHelper = `// --- OpenSynapse live control (xk6-kv) ---
const __osKv = openKv({ backend: 'memory' });
// With several k6 instances (cluster mode) each paces its share of the limit.
const __osInstances = Math.max(1, Number(__ENV.OPENSYNAPSE_INSTANCES || 1));
let __osRps = Number(__ENV.OPENSYNAPSE_RPS_TARGET || 0) / __osInstances;
let __osNext = 0;

export async function __osControl() {
  const s = http.get(__ENV.OPENSYNAPSE_CONTROL_URL, {
    headers: { 'X-OpenSynapse-Run-Token': __ENV.OPENSYNAPSE_CONTROL_TOKEN },
    tags: { name: 'opensynapse-control' },
  });
  if (s.status === 200) {
    const state = s.json();
    await __osKv.set('rps_target', Number(state.rps_target) || 0);
    await __osKv.set('should_stop', !!state.should_stop);
    await __osKv.set('paused', !!state.paused);
  }
  sleep(1);
}

// Reads live-control state at the start of each iteration. Returns false
// while the run is paused or being stopped, so no new requests start.
async function __osSync() {
  try { __osRps = (Number(await __osKv.get('rps_target')) || 0) / __osInstances; } catch (e) { /* not published yet */ }
  let hold = false;
  try { hold = !!(await __osKv.get('should_stop')) || !!(await __osKv.get('paused')); } catch (e) { /* not published yet */ }
  if (hold) {
    sleep(0.5);
    return false;
  }
  return true;
}

// Spaces this VU's requests so all VUs together stay at or under the RPS target.
function __osPace() {
  if (!(__osRps > 0)) return;
  const vus = Math.max(1, exec.instance.vusActive - 1);
  const interval = (vus * 1000) / __osRps;
  const now = Date.now();
  if (__osNext > now) sleep((__osNext - now) / 1000);
  __osNext = Math.max(__osNext, now) + interval;
}
`

// threshold is a plan-level pass/fail criterion (k6 thresholds).
type threshold struct {
	Metric      string `json:"metric"`
	Condition   string `json:"condition"` // e.g. "rate<0.05", "p(95)<500"
	AbortOnFail bool   `json:"abort_on_fail"`
}

// thresholdMetricRe accepts metric names with optional tag filters, e.g.
// http_req_duration{group:::Checkout}.
var thresholdMetricRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\{[^{}']*\})?$`)

// renderThresholds emits options.thresholds; conditions for the same metric
// are grouped.
func renderThresholds(ts []threshold) string {
	var order []string
	byMetric := map[string][]string{}
	for _, t := range ts {
		if _, ok := byMetric[t.Metric]; !ok {
			order = append(order, t.Metric)
		}
		entry := fmt.Sprintf("{ threshold: '%s', abortOnFail: %v }", sanitizeJSString(strings.TrimSpace(t.Condition)), t.AbortOnFail)
		byMetric[t.Metric] = append(byMetric[t.Metric], entry)
	}
	var b strings.Builder
	b.WriteString("  thresholds: {\n")
	for _, m := range order {
		b.WriteString(fmt.Sprintf("    '%s': [%s],\n", m, strings.Join(byMetric[m], ", ")))
	}
	b.WriteString("  },\n")
	return b.String()
}

// --- Helper types for JSON unmarshaling ---

type stage struct {
	Duration string `json:"duration"`
	Target   int    `json:"target"`
}

type headerEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type httpBody struct {
	Type       string      `json:"type"`
	Content    string      `json:"content"`
	FormFields []formField `json:"form_fields"`
}

type formField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type wsMessage struct {
	Type        string `json:"type"`
	Data        string `json:"data"`
	DelayBefore string `json:"delay_before"`
}

type wsExpectedMessage struct {
	MatchType string `json:"match_type"`
	Value     string `json:"value"`
	Timeout   string `json:"timeout"`
}

// --- Durations ---

// ParseDuration parses k6 duration strings ("30s", "2m", "1h30m", bare
// seconds "45").
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err == nil && fmt.Sprint(f) == s {
		return time.Duration(f * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// MaxDurationFor returns the hard upper bound a managed run is started with:
// the planned duration plus a 50% buffer (at least five minutes, at most an
// extra 24 hours). Live "extend" requests cannot go past it.
func MaxDurationFor(planned time.Duration) time.Duration {
	buffer := planned / 2
	if buffer < 5*time.Minute {
		buffer = 5 * time.Minute
	}
	if buffer > 24*time.Hour {
		buffer = 24 * time.Hour
	}
	return planned + buffer
}

// --- String helpers ---

// sanitizeJSString escapes a value for a single-quoted JS string literal.
func sanitizeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, " ", "\\u2028")
	s = strings.ReplaceAll(s, " ", "\\u2029")
	return s
}

// sanitizeComment keeps node names from breaking out of a // comment.
func sanitizeComment(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

// sanitizeVarName converts a node ID to a valid JS variable name fragment.
//
// Node IDs in OpenSynapse are plan-author or generator controlled, and can
// contain arbitrary printable characters — crawler-generated IDs often embed
// full URLs (e.g. "req-GET-https://example.com/a/b"). The output is always
// substituted into a bare JS identifier position (`let params_<X>`,
// `let res_<X>`, `const <X>`), so any character outside the ECMAScript
// IdentifierPart grammar would produce an unparseable script and the k6
// subprocess would exit immediately. Everything non-[A-Za-z0-9_$] is mapped
// to '_', and a leading digit is prefixed with '_' to avoid identifiers like
// `123foo` that would also fail to parse.
//
// Regression coverage: see TestSanitizeVarName in compiler_test.go.
func sanitizeVarName(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_', r == '$':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				// Identifiers cannot start with a digit.
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// sanitizeScenarioName converts a scenario name to a k6-compatible identifier.
// k6 requires scenario names to contain only [a-zA-Z0-9_-].
func sanitizeScenarioName(s string) string {
	if s == "" {
		return "default"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

var varRefRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// k6Globals are referenced directly rather than through __v().
var k6Globals = map[string]bool{"__VU": true, "__ITER": true}

// resolveVarRefs turns a value containing ${NAME} references into a JS
// template literal that resolves them at runtime through __v(). Values
// without references become plain single-quoted strings.
// Input: "${BASE_URL}/api/login" -> Output: `${__v('BASE_URL')}/api/login`
func (c *compiler) resolveVarRefs(s string) string {
	locs := varRefRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return "'" + sanitizeJSString(s) + "'"
	}
	var b strings.Builder
	b.WriteByte('`')
	last := 0
	for _, m := range locs {
		b.WriteString(escapeTemplate(s[last:m[0]]))
		name := s[m[2]:m[3]]
		if k6Globals[name] {
			b.WriteString("${" + name + "}")
		} else {
			c.needsVars = true
			b.WriteString("${__v('" + name + "')}")
		}
		last = m[1]
	}
	b.WriteString(escapeTemplate(s[last:]))
	b.WriteByte('`')
	return b.String()
}

// escapeTemplate escapes literal text for a JS template literal.
func escapeTemplate(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	return strings.ReplaceAll(s, "${", "\\${")
}

// resolveHTTPBody returns the JS expression for the request body.
func (c *compiler) resolveHTTPBody(body httpBody) string {
	switch body.Type {
	case "none", "":
		return "null"
	case "json", "raw":
		return c.resolveVarRefs(body.Content)
	case "form":
		if len(body.FormFields) == 0 {
			if body.Content != "" {
				// urlencoded form supplied as raw text
				return c.resolveVarRefs(body.Content)
			}
			return "{}"
		}
		var parts []string
		for _, f := range body.FormFields {
			parts = append(parts, fmt.Sprintf("'%s': %s", sanitizeJSString(f.Key), c.resolveVarRefs(f.Value)))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	default:
		return "null"
	}
}

// jsonPathToJS converts a JSONPath like "$.items" to JS accessor ".items".
func jsonPathToJS(path string) string {
	if strings.HasPrefix(path, "$.") {
		return path[1:] // strip the "$" but keep the dot
	}
	if strings.HasPrefix(path, "$") {
		return path[1:]
	}
	return "." + path
}

// dedup returns a deduplicated copy of a string slice, preserving order.
func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
