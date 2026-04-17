// Package compiler transforms a plan JSON document into a valid k6 JavaScript file.
//
// The transformer walks the plan tree and emits k6 JavaScript. Each node type
// has a corresponding emit function. Controllers become if blocks or for loops.
// Transaction controllers become group() calls. Assertions become check() calls.
// Data sources become SharedArray imports. The transformer is stateless and
// deterministic: the same plan always produces the same script bytes.
package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// Compile transforms a Plan into a k6 JavaScript string.
func Compile(plan *db.Plan) (string, error) {
	c := &compiler{
		imports:      make(map[string][]string),
		topLevel:     &strings.Builder{},
		options:      &strings.Builder{},
		defaultFunc:  &strings.Builder{},
		dataSources:  &strings.Builder{},
		needsWS:      false,
		needsShared:  false,
		needsExec:    false,
		extControlled: false,
	}

	if err := c.compilePlan(plan.Root); err != nil {
		return "", fmt.Errorf("compile plan: %w", err)
	}

	return c.render(), nil
}

// CompileNode compiles a single root node (useful for testing individual node types).
func CompileNode(node db.Node) (string, error) {
	c := &compiler{
		imports:      make(map[string][]string),
		topLevel:     &strings.Builder{},
		options:      &strings.Builder{},
		defaultFunc:  &strings.Builder{},
		dataSources:  &strings.Builder{},
		needsWS:      false,
		needsShared:  false,
		needsExec:    false,
		extControlled: false,
	}

	if err := c.emitNode(node, 1); err != nil {
		return "", fmt.Errorf("compile node: %w", err)
	}

	return c.render(), nil
}

type compiler struct {
	imports       map[string][]string // module -> named imports
	topLevel      *strings.Builder    // top-level code (SharedArray, etc.)
	options       *strings.Builder    // export let options = { ... }
	defaultFunc   *strings.Builder    // body of export default function()
	dataSources   *strings.Builder    // SharedArray declarations
	needsWS       bool
	needsShared   bool
	needsExec     bool
	extControlled bool
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

// indent returns n tabs of indentation.
func indent(n int) string {
	return strings.Repeat("  ", n)
}

// compilePlan handles the root plan node.
func (c *compiler) compilePlan(root db.Node) error {
	if root.Type != "plan" {
		return fmt.Errorf("expected root node type 'plan', got %q", root.Type)
	}

	// First pass: collect data sources and environment bindings at the plan level,
	// and identify scenarios.
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
			// Environment bindings are informational; no code emitted in the script.
		case "scenario":
			if err := c.emitScenario(child); err != nil {
				return err
			}
		default:
			// Other children at the plan level: emit into default function.
			if err := c.emitNode(child, 1); err != nil {
				return err
			}
		}
	}

	return nil
}

// emitScenario generates k6 options and populates the default function body.
func (c *compiler) emitScenario(node db.Node) error {
	var props struct {
		Executor       string  `json:"executor"`
		VUs            int     `json:"vus"`
		MaxVUs         int     `json:"max_vus"`
		Duration       string  `json:"duration"`
		Iterations     int     `json:"iterations"`
		Rate           int     `json:"rate"`
		TimeUnit       string  `json:"time_unit"`
		PreAllocVUs    int     `json:"pre_allocated_vus"`
		GracefulStop   string  `json:"graceful_stop"`
		Stages         []stage `json:"stages"`
	}
	if err := json.Unmarshal(node.Properties, &props); err != nil {
		return fmt.Errorf("unmarshal scenario properties: %w", err)
	}

	ob := c.options
	ob.WriteString("export let options = {\n")
	ob.WriteString("  scenarios: {\n")
	ob.WriteString(fmt.Sprintf("    '%s': {\n", sanitizeScenarioName(node.Name)))
	ob.WriteString(fmt.Sprintf("      executor: '%s',\n", props.Executor))

	switch props.Executor {
	case "externally-controlled":
		c.extControlled = true
		if props.VUs > 0 {
			ob.WriteString(fmt.Sprintf("      vus: %d,\n", props.VUs))
		}
		if props.MaxVUs > 0 {
			ob.WriteString(fmt.Sprintf("      maxVUs: %d,\n", props.MaxVUs))
		}
		if props.Duration != "" {
			ob.WriteString(fmt.Sprintf("      duration: '%s',\n", props.Duration))
		}
	case "ramping-vus":
		if props.Stages != nil {
			ob.WriteString("      stages: [\n")
			for _, s := range props.Stages {
				ob.WriteString(fmt.Sprintf("        { duration: '%s', target: %d },\n", s.Duration, s.Target))
			}
			ob.WriteString("      ],\n")
		}
		if props.GracefulStop != "" {
			ob.WriteString(fmt.Sprintf("      gracefulStop: '%s',\n", props.GracefulStop))
		}
	case "constant-vus":
		if props.VUs > 0 {
			ob.WriteString(fmt.Sprintf("      vus: %d,\n", props.VUs))
		}
		if props.Duration != "" {
			ob.WriteString(fmt.Sprintf("      duration: '%s',\n", props.Duration))
		}
	case "constant-arrival-rate":
		if props.Rate > 0 {
			ob.WriteString(fmt.Sprintf("      rate: %d,\n", props.Rate))
		}
		if props.TimeUnit != "" {
			ob.WriteString(fmt.Sprintf("      timeUnit: '%s',\n", props.TimeUnit))
		}
		if props.Duration != "" {
			ob.WriteString(fmt.Sprintf("      duration: '%s',\n", props.Duration))
		}
		if props.PreAllocVUs > 0 {
			ob.WriteString(fmt.Sprintf("      preAllocatedVUs: %d,\n", props.PreAllocVUs))
		}
		if props.MaxVUs > 0 {
			ob.WriteString(fmt.Sprintf("      maxVUs: %d,\n", props.MaxVUs))
		}
	case "ramping-arrival-rate":
		if props.Stages != nil {
			ob.WriteString("      stages: [\n")
			for _, s := range props.Stages {
				ob.WriteString(fmt.Sprintf("        { duration: '%s', target: %d },\n", s.Duration, s.Target))
			}
			ob.WriteString("      ],\n")
		}
		if props.TimeUnit != "" {
			ob.WriteString(fmt.Sprintf("      timeUnit: '%s',\n", props.TimeUnit))
		}
		if props.PreAllocVUs > 0 {
			ob.WriteString(fmt.Sprintf("      preAllocatedVUs: %d,\n", props.PreAllocVUs))
		}
		if props.MaxVUs > 0 {
			ob.WriteString(fmt.Sprintf("      maxVUs: %d,\n", props.MaxVUs))
		}
	case "shared-iterations":
		if props.VUs > 0 {
			ob.WriteString(fmt.Sprintf("      vus: %d,\n", props.VUs))
		}
		if props.Iterations > 0 {
			ob.WriteString(fmt.Sprintf("      iterations: %d,\n", props.Iterations))
		}
	case "per-vu-iterations":
		if props.VUs > 0 {
			ob.WriteString(fmt.Sprintf("      vus: %d,\n", props.VUs))
		}
		if props.Iterations > 0 {
			ob.WriteString(fmt.Sprintf("      iterations: %d,\n", props.Iterations))
		}
	}

	ob.WriteString("    },\n")
	ob.WriteString("  },\n")
	ob.WriteString("};\n")

	// Emit scenario children into the default function.
	for _, child := range node.Children {
		if !child.Enabled {
			continue
		}
		if err := c.emitNode(child, 1); err != nil {
			return err
		}
	}

	return nil
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

	// Comment with node name.
	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))

	// Build params object if we have headers, timeout, or redirects config.
	hasParams := len(props.Headers) > 0 || props.Timeout != "" || props.FollowRedirects != nil
	if hasParams {
		df.WriteString(fmt.Sprintf("%slet params_%s = {\n", ind, sanitizeVarName(node.ID)))
		if len(props.Headers) > 0 {
			df.WriteString(fmt.Sprintf("%s  headers: {\n", ind))
			for _, h := range props.Headers {
				df.WriteString(fmt.Sprintf("%s    '%s': '%s',\n", ind, sanitizeJSString(h.Key), sanitizeJSString(h.Value)))
			}
			df.WriteString(fmt.Sprintf("%s  },\n", ind))
		}
		if props.Timeout != "" {
			df.WriteString(fmt.Sprintf("%s  timeout: '%s',\n", ind, props.Timeout))
		}
		if props.FollowRedirects != nil {
			if *props.FollowRedirects {
				df.WriteString(fmt.Sprintf("%s  redirects: 10,\n", ind))
			} else {
				df.WriteString(fmt.Sprintf("%s  redirects: 0,\n", ind))
			}
		}
		df.WriteString(fmt.Sprintf("%s};\n", ind))
	}

	// Resolve the URL: replace ${VAR} with template literals.
	url := resolveVarRefs(props.URL)
	method := strings.ToLower(props.Method)

	// Build the call.
	switch method {
	case "get", "head", "options", "delete":
		if hasParams {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s, params_%s);\n",
				ind, sanitizeVarName(node.ID), method, url, sanitizeVarName(node.ID)))
		} else {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s);\n",
				ind, sanitizeVarName(node.ID), method, url))
		}
	case "post", "put", "patch":
		bodyStr := c.resolveHTTPBody(props.Body)
		if hasParams {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s, %s, params_%s);\n",
				ind, sanitizeVarName(node.ID), method, url, bodyStr, sanitizeVarName(node.ID)))
		} else {
			df.WriteString(fmt.Sprintf("%slet res_%s = http.%s(%s, %s);\n",
				ind, sanitizeVarName(node.ID), method, url, bodyStr))
		}
	default:
		return fmt.Errorf("unsupported HTTP method %q", props.Method)
	}

	// Emit children (assertions, timers) that reference the response.
	for _, child := range node.Children {
		if !child.Enabled {
			continue
		}
		if child.Type == "assertion" {
			if err := c.emitAssertionForResponse(child, depth, "res_"+sanitizeVarName(node.ID)); err != nil {
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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))

	url := resolveVarRefs(props.URL)

	paramsStr := "null"
	if props.ConnectTimeout != "" {
		paramsStr = fmt.Sprintf("{ timeout: '%s' }", props.ConnectTimeout)
	}

	df.WriteString(fmt.Sprintf("%slet res_%s = ws.connect(%s, %s, function (socket) {\n",
		ind, sanitizeVarName(node.ID), url, paramsStr))

	innerInd := indent(depth + 1)

	// On open: send messages.
	if len(props.Messages) > 0 {
		df.WriteString(fmt.Sprintf("%ssocket.on('open', function () {\n", innerInd))
		for _, msg := range props.Messages {
			df.WriteString(fmt.Sprintf("%s  socket.send('%s');\n", innerInd, sanitizeJSString(msg.Data)))
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
	if props.ExpectedMessages != nil && len(props.ExpectedMessages) > 0 {
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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))
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

	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))
	df.WriteString(fmt.Sprintf("%sif (%s) {\n", ind, props.Condition))

	for _, child := range node.Children {
		if err := c.emitNode(child, depth+1); err != nil {
			return err
		}
	}

	df.WriteString(fmt.Sprintf("%s}", ind))
	// Do not add newline here -- else-controller may follow.

	return nil
}

// emitElseController generates an else { ... } block.
func (c *compiler) emitElseController(node db.Node, depth int) error {
	ind := indent(depth)
	df := c.defaultFunc

	df.WriteString(fmt.Sprintf(" else {\n"))
	_ = ind // indentation is handled by the parent if-controller's closing brace

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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))

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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))
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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))
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

	df.WriteString(fmt.Sprintf("%s// %s\n", ind, node.Name))

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

// emitAssertion generates a check() call (standalone, not tied to a response variable).
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
		accessor = "r.body"
	case "header":
		accessor = fmt.Sprintf("r.headers['%s']", sanitizeJSString(headerName))
	case "response_time":
		accessor = "r.timings.duration"
	default:
		accessor = "r.status"
	}

	var expr string
	switch condition {
	case "equals":
		switch v := value.(type) {
		case float64:
			expr = fmt.Sprintf("%s === %d", accessor, int(v))
		case string:
			expr = fmt.Sprintf("%s === '%s'", accessor, sanitizeJSString(v))
		default:
			expr = fmt.Sprintf("%s === %v", accessor, v)
		}
	case "contains":
		expr = fmt.Sprintf("%s.includes('%s')", accessor, sanitizeJSString(fmt.Sprintf("%v", value)))
	case "matches":
		expr = fmt.Sprintf("new RegExp('%s').test(%s)", sanitizeJSString(fmt.Sprintf("%v", value)), accessor)
	case "jsonpath":
		// For jsonpath, check that the path exists in the parsed JSON.
		expr = fmt.Sprintf("JSON.parse(%s)%s !== undefined", accessor, jsonPathToJS(fmt.Sprintf("%v", value)))
	case "less_than":
		expr = fmt.Sprintf("%s < %v", accessor, value)
	case "greater_than":
		expr = fmt.Sprintf("%s > %v", accessor, value)
	case "exists":
		expr = fmt.Sprintf("%s !== undefined && %s !== null", accessor, accessor)
	default:
		expr = fmt.Sprintf("%s === %v", accessor, value)
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
		df.WriteString(fmt.Sprintf("%ssleep(Math.max(0, %.3f + %.3f * Math.sqrt(-2 * Math.log(Math.random())) * Math.cos(2 * Math.PI * Math.random())));\n",
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

// render assembles the final k6 script from all collected parts.
func (c *compiler) render() string {
	var sb strings.Builder

	// Imports.
	// Always import http if used (default import style).
	if _, ok := c.imports["k6/http"]; ok {
		sb.WriteString("import http from 'k6/http';\n")
		delete(c.imports, "k6/http")
	}

	// WebSocket import.
	if c.needsWS {
		sb.WriteString("import ws from 'k6/ws';\n")
		delete(c.imports, "k6/ws")
	}

	// k6 named imports.
	if names, ok := c.imports["k6"]; ok && len(names) > 0 {
		sb.WriteString(fmt.Sprintf("import { %s } from 'k6';\n", strings.Join(dedup(names), ", ")))
		delete(c.imports, "k6")
	}

	// SharedArray import.
	if c.needsShared {
		sb.WriteString("import { SharedArray } from 'k6/data';\n")
	}

	// exec import for externally-controlled.
	if c.extControlled {
		sb.WriteString("import exec from 'k6/execution';\n")
		c.needsExec = true
	}

	// Any remaining module imports (sorted for determinism).
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

	// Data sources (SharedArray declarations).
	if c.dataSources.Len() > 0 {
		sb.WriteString(c.dataSources.String())
	}

	// Options.
	if c.options.Len() > 0 {
		sb.WriteString(c.options.String())
		sb.WriteString("\n")
	}

	// Token bucket helper and should-stop watchdog for externally-controlled.
	if c.extControlled {
		sb.WriteString(tokenBucketHelper)
		sb.WriteString("\n")
	}

	// Default function.
	sb.WriteString("export default function () {\n")
	if c.extControlled {
		sb.WriteString("  // should-stop watchdog\n")
		sb.WriteString("  if (shouldStop()) {\n")
		sb.WriteString("    exec.test.abort('stop flag set');\n")
		sb.WriteString("  }\n\n")
		sb.WriteString("  // RPS token bucket\n")
		sb.WriteString("  acquireToken();\n\n")
	}
	sb.WriteString(c.defaultFunc.String())
	sb.WriteString("}\n")

	return sb.String()
}

// tokenBucketHelper is the xk6-kv token bucket and should-stop watchdog
// prepended to every externally-controlled script, per docs/02-architecture.md section 3.
const tokenBucketHelper = `// --- xk6-kv token bucket for RPS control ---
// The control plane updates the 'rps_target' key; VUs read it to throttle.
let __rpsTarget = __ENV.RPS_TARGET ? parseInt(__ENV.RPS_TARGET) : 0;

function acquireToken() {
  if (__rpsTarget <= 0) return;
  let interval = 1000.0 / __rpsTarget;
  sleep(interval / 1000.0);
}

// --- should-stop watchdog ---
// The control plane sets 'should_stop' to signal graceful shutdown.
function shouldStop() {
  return __ENV.SHOULD_STOP === 'true';
}
`

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

// --- String helpers ---

// sanitizeJSString escapes single quotes and backslashes for JS string literals.
func sanitizeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
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
		case r == ' ':
			b.WriteByte('-')
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// resolveVarRefs replaces ${VAR} references with JS template literal syntax.
// Input: "${BASE_URL}/api/login" -> Output: `${BASE_URL}/api/login`
func resolveVarRefs(s string) string {
	if strings.Contains(s, "${") {
		return "`" + s + "`"
	}
	return "'" + sanitizeJSString(s) + "'"
}

// resolveHTTPBody returns the JS expression for the request body.
func (c *compiler) resolveHTTPBody(body httpBody) string {
	switch body.Type {
	case "none":
		return "null"
	case "json", "raw":
		if strings.Contains(body.Content, "${") {
			return "`" + body.Content + "`"
		}
		return "'" + sanitizeJSString(body.Content) + "'"
	case "form":
		if len(body.FormFields) == 0 {
			return "{}"
		}
		var parts []string
		for _, f := range body.FormFields {
			parts = append(parts, fmt.Sprintf("'%s': '%s'", sanitizeJSString(f.Key), sanitizeJSString(f.Value)))
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
