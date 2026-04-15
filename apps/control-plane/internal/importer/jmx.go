// Package importer handles conversion of JMeter .jmx files to OpenSynapse plans.
package importer

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// JMXElement is the generic representation of a JMeter XML element.
type JMXElement struct {
	XMLName  xml.Name     `xml:""`
	Attrs    []xml.Attr   `xml:",any,attr"`
	Children []JMXElement `xml:",any"`
	Content  string       `xml:",chardata"`
}

// ImportLog tracks what was imported, approximated, or lost.
type ImportLog struct {
	Imported     []string `json:"imported"`
	Approximated []string `json:"approximated"`
	Unsupported  []string `json:"unsupported"`
}

// ImportResult holds the generated plan and the log.
type ImportResult struct {
	Plan *db.Plan  `json:"plan"`
	Log  ImportLog `json:"log"`
}

// ImportJMX parses a JMeter .jmx file and returns an OpenSynapse plan.
func ImportJMX(reader io.Reader) (*ImportResult, error) {
	var root JMXElement
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse JMX: %w", err)
	}

	log := &ImportLog{}
	children := convertChildren(root, log)

	planRoot := db.Node{
		ID:         uuid.New().String(),
		Type:       "plan",
		Name:       getAttr(root, "testname", "Imported JMeter Plan"),
		Enabled:    true,
		Properties: json.RawMessage(`{}`),
		Children:   children,
	}

	plan := &db.Plan{
		Name:        getAttr(root, "testname", "Imported JMeter Plan"),
		Description: "Imported from JMeter .jmx file",
		Tags:        []string{"imported", "jmeter"},
		Root:        planRoot,
	}

	return &ImportResult{Plan: plan, Log: *log}, nil
}

func convertChildren(parent JMXElement, log *ImportLog) []db.Node {
	var nodes []db.Node
	for _, child := range parent.Children {
		tag := child.XMLName.Local
		// Flatten hashTree, jmeterTestPlan, and TestPlan containers
		if tag == "hashTree" || tag == "jmeterTestPlan" || tag == "TestPlan" {
			nodes = append(nodes, convertChildren(child, log)...)
			continue
		}
		node := convertElement(child, log)
		if node != nil {
			nodes = append(nodes, *node)
		}
	}
	return nodes
}

func convertElement(el JMXElement, log *ImportLog) *db.Node {
	tag := el.XMLName.Local

	switch tag {
	case "ThreadGroup", "SetupThreadGroup", "PostThreadGroup":
		return convertThreadGroup(el, log)
	case "HTTPSamplerProxy":
		return convertHTTPSampler(el, log)
	case "IfController":
		return convertIfController(el, log)
	case "WhileController":
		return convertWhileController(el, log)
	case "LoopController":
		return convertLoopController(el, log)
	case "TransactionController":
		return convertTransactionController(el, log)
	case "OnceOnlyController":
		return convertOnceOnlyController(el, log)
	case "ConstantTimer":
		return convertConstantTimer(el, log)
	case "UniformRandomTimer":
		return convertUniformRandomTimer(el, log)
	case "ResponseAssertion":
		return convertResponseAssertion(el, log)
	case "JSONPathAssertion":
		return convertJSONPathAssertion(el, log)
	case "CSVDataSet":
		return convertCSVDataSet(el, log)
	case "HeaderManager":
		// Headers are attached to parent HTTP sampler; skip as standalone
		return nil
	case "hashTree":
		// hashTree is JMeter's container — collect all converted children
		children := convertChildren(el, log)
		if len(children) == 1 {
			return &children[0]
		}
		if len(children) > 1 {
			return &db.Node{
				ID: uuid.New().String(), Type: "transaction-controller",
				Name: "Group", Enabled: true, Properties: json.RawMessage(`{}`),
				Children: children,
			}
		}
		return nil
	case "TestPlan":
		// TestPlan is the JMeter root — just pass through to children
		return nil
	case "jmeterTestPlan":
		return nil
	default:
		// Check if it's a recognized element we can approximate
		if isUnsupported(tag) {
			name := getAttr(el, "testname", tag)
			log.Unsupported = append(log.Unsupported, fmt.Sprintf("%s (%s): preserved as code block", tag, name))
			return convertAsCodeBlock(el, tag)
		}
		// Recurse into unknown container elements
		children := convertChildren(el, log)
		if len(children) > 0 {
			// Wrap in transaction
			return &db.Node{
				ID:         uuid.New().String(),
				Type:       "transaction-controller",
				Name:       getAttr(el, "testname", tag),
				Enabled:    isEnabled(el),
				Properties: json.RawMessage(`{}`),
				Children:   children,
			}
		}
		return nil
	}
}

func convertThreadGroup(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Thread Group")
	log.Imported = append(log.Imported, "ThreadGroup: "+name)

	numThreads := getProp(el, "ThreadGroup.num_threads", "10")
	rampTime := getProp(el, "ThreadGroup.ramp_time", "1")
	duration := getProp(el, "ThreadGroup.duration", "60")

	props := map[string]interface{}{
		"executor": "ramping-vus",
		"vus":      parseInt(numThreads, 10),
		"duration": duration + "s",
		"stages": []map[string]interface{}{
			{"duration": rampTime + "s", "target": parseInt(numThreads, 10)},
			{"duration": duration + "s", "target": parseInt(numThreads, 10)},
		},
	}
	propsJSON, _ := json.Marshal(props)

	children := convertChildren(el, log)

	return &db.Node{
		ID:         uuid.New().String(),
		Type:       "scenario",
		Name:       name,
		Enabled:    isEnabled(el),
		Properties: propsJSON,
		Children:   children,
	}
}

func convertHTTPSampler(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "HTTP Request")
	log.Imported = append(log.Imported, "HTTPSamplerProxy: "+name)

	method := getProp(el, "HTTPSampler.method", "GET")
	domain := getProp(el, "HTTPSampler.domain", "${BASE_URL}")
	port := getProp(el, "HTTPSampler.port", "")
	protocol := getProp(el, "HTTPSampler.protocol", "https")
	path := getProp(el, "HTTPSampler.path", "/")

	url := protocol + "://" + domain
	if port != "" && port != "80" && port != "443" {
		url += ":" + port
	}
	url += path

	bodyType := "none"
	bodyContent := ""
	postBody := getProp(el, "HTTPSampler.postBodyRaw", "")
	if postBody != "" || method == "POST" || method == "PUT" || method == "PATCH" {
		bodyType = "raw"
		bodyContent = postBody
	}

	props := map[string]interface{}{
		"method":           method,
		"url":              url,
		"headers":          []interface{}{},
		"body":             map[string]interface{}{"type": bodyType, "content": bodyContent},
		"follow_redirects": getProp(el, "HTTPSampler.follow_redirects", "true") == "true",
	}
	propsJSON, _ := json.Marshal(props)

	children := convertChildren(el, log)

	return &db.Node{
		ID:         uuid.New().String(),
		Type:       "http",
		Name:       name,
		Enabled:    isEnabled(el),
		Properties: propsJSON,
		Children:   children,
	}
}

func convertIfController(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "If Controller")
	log.Imported = append(log.Imported, "IfController: "+name)

	condition := getProp(el, "IfController.condition", "true")
	props := map[string]interface{}{"condition": condition}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "if-controller", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: convertChildren(el, log),
	}
}

func convertWhileController(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "While Controller")
	log.Imported = append(log.Imported, "WhileController: "+name)
	condition := getProp(el, "WhileController.condition", "true")
	props := map[string]interface{}{"while_condition": condition}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "loop-controller", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: convertChildren(el, log),
	}
}

func convertLoopController(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Loop Controller")
	log.Imported = append(log.Imported, "LoopController: "+name)
	count := getProp(el, "LoopController.loops", "1")
	props := map[string]interface{}{"count": parseInt(count, 1)}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "loop-controller", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: convertChildren(el, log),
	}
}

func convertTransactionController(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Transaction Controller")
	log.Imported = append(log.Imported, "TransactionController: "+name)

	return &db.Node{
		ID: uuid.New().String(), Type: "transaction-controller", Name: name,
		Enabled: isEnabled(el), Properties: json.RawMessage(`{}`), Children: convertChildren(el, log),
	}
}

func convertOnceOnlyController(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Once Only Controller")
	log.Imported = append(log.Imported, "OnceOnlyController: "+name)

	return &db.Node{
		ID: uuid.New().String(), Type: "once-only-controller", Name: name,
		Enabled: isEnabled(el), Properties: json.RawMessage(`{}`), Children: convertChildren(el, log),
	}
}

func convertConstantTimer(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Constant Timer")
	log.Imported = append(log.Imported, "ConstantTimer: "+name)
	delay := getProp(el, "ConstantTimer.delay", "1000")
	props := map[string]interface{}{"timer_type": "constant", "duration_ms": parseInt(delay, 1000)}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "timer", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: []db.Node{},
	}
}

func convertUniformRandomTimer(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Uniform Random Timer")
	log.Imported = append(log.Imported, "UniformRandomTimer: "+name)
	delay := getProp(el, "ConstantTimer.delay", "0")
	maxRandom := getProp(el, "RandomTimer.range", "1000")
	props := map[string]interface{}{
		"timer_type": "uniform_random",
		"min_ms":     parseInt(delay, 0),
		"max_ms":     parseInt(delay, 0) + parseInt(maxRandom, 1000),
	}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "timer", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: []db.Node{},
	}
}

func convertResponseAssertion(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "Response Assertion")
	log.Imported = append(log.Imported, "ResponseAssertion: "+name)

	testField := getProp(el, "Assertion.test_field", "Assertion.response_data")
	target := "body"
	if strings.Contains(testField, "response_code") {
		target = "status"
	} else if strings.Contains(testField, "response_headers") {
		target = "header"
	}

	condition := "contains"
	testType := getProp(el, "Assertion.test_type", "2")
	if testType == "1" || testType == "8" {
		condition = "matches"
	} else if testType == "16" {
		condition = "equals"
	}

	value := getProp(el, "Assertion.test_strings", "")

	props := map[string]interface{}{
		"target": target, "condition": condition, "value": value, "negate": false,
	}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "assertion", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: []db.Node{},
	}
}

func convertJSONPathAssertion(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "JSON Path Assertion")
	log.Imported = append(log.Imported, "JSONPathAssertion: "+name)
	jsonPath := getProp(el, "JSON_PATH", "$")
	expectedValue := getProp(el, "EXPECTED_VALUE", "")

	value := jsonPath
	if expectedValue != "" {
		value = jsonPath + "=" + expectedValue
	}

	props := map[string]interface{}{
		"target": "body", "condition": "jsonpath", "value": value, "negate": false,
	}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "assertion", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: []db.Node{},
	}
}

func convertCSVDataSet(el JMXElement, log *ImportLog) *db.Node {
	name := getAttr(el, "testname", "CSV Data Set")
	log.Imported = append(log.Imported, "CSVDataSet: "+name)
	filename := getProp(el, "filename", "data.csv")
	delimiter := getProp(el, "delimiter", ",")
	varNames := getProp(el, "variableNames", "data")

	props := map[string]interface{}{
		"source_type":         "csv",
		"path":                filename,
		"delimiter":           delimiter,
		"variable_name":       varNames,
		"sharing":             "shared",
		"first_row_is_header": true,
	}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "data-source", Name: name,
		Enabled: isEnabled(el), Properties: propsJSON, Children: []db.Node{},
	}
}

func convertAsCodeBlock(el JMXElement, tag string) *db.Node {
	name := getAttr(el, "testname", tag)
	comment := fmt.Sprintf("// Unsupported JMeter element: %s\n// Original name: %s\n// This element could not be automatically converted.\n// Review and replace with equivalent k6 code.", tag, name)

	props := map[string]interface{}{"code": comment}
	propsJSON, _ := json.Marshal(props)

	return &db.Node{
		ID: uuid.New().String(), Type: "code-block", Name: name + " (unsupported)",
		Enabled: false, Properties: propsJSON, Children: []db.Node{},
	}
}

// --- helpers ---

func isUnsupported(tag string) bool {
	unsupported := map[string]bool{
		"JSR223Sampler": true, "BeanShellSampler": true,
		"JDBCSampler": true, "SOAPSampler": true,
		"JMSSampler": true, "BeanShellPreProcessor": true,
		"BeanShellPostProcessor": true, "JSR223PreProcessor": true,
		"JSR223PostProcessor": true, "RegexExtractor": true,
		"JSONExtractor": true, "XPath2Extractor": true,
	}
	return unsupported[tag]
}

func getAttr(el JMXElement, name, fallback string) string {
	for _, a := range el.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return fallback
}

func isEnabled(el JMXElement) bool {
	v := getAttr(el, "enabled", "true")
	return v == "true"
}

func getProp(el JMXElement, name, fallback string) string {
	for _, child := range el.Children {
		if child.XMLName.Local == "stringProp" || child.XMLName.Local == "intProp" || child.XMLName.Local == "boolProp" {
			propName := getAttr(child, "name", "")
			if propName == name {
				return strings.TrimSpace(child.Content)
			}
		}
		// Recurse into elementProp and collectionProp
		if child.XMLName.Local == "elementProp" || child.XMLName.Local == "collectionProp" {
			v := getProp(child, name, "")
			if v != "" {
				return v
			}
		}
	}
	return fallback
}

func parseInt(s string, fallback int) int {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return fallback
	}
	return v
}
