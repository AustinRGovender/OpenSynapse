package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"go.yaml.in/yaml/v3"
)

// OpenAPISpec represents a minimal parsed OpenAPI 3.x or Swagger 2.x document.
type OpenAPISpec struct {
	OpenAPI  string                          `json:"openapi"`
	Swagger  string                          `json:"swagger"`
	Info     struct{ Title, Version string } `json:"info"`
	Paths    map[string]PathItem             `json:"paths"`
	Servers  []struct{ URL string }          `json:"servers"`
	Host     string                          `json:"host"`     // Swagger 2
	BasePath string                          `json:"basePath"` // Swagger 2
	Schemes  []string                        `json:"schemes"`  // Swagger 2
	// Component schemas, for resolving $ref when building example bodies.
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
	Definitions map[string]json.RawMessage `json:"definitions"` // Swagger 2
}

// PathItem maps lower-case HTTP methods to operations. Path-level
// parameters are merged into each operation; other path-level keys
// (summary, servers, $ref…) are ignored.
type PathItem map[string]Operation

var httpMethods = map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}

func (p *PathItem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var shared []Parameter
	if r, ok := raw["parameters"]; ok {
		json.Unmarshal(r, &shared)
	}
	item := PathItem{}
	for key, r := range raw {
		method := strings.ToLower(key)
		if !httpMethods[method] {
			continue
		}
		var op Operation
		if err := json.Unmarshal(r, &op); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		for _, sp := range shared {
			overridden := false
			for _, own := range op.Parameters {
				if own.Name == sp.Name && own.In == sp.In {
					overridden = true
					break
				}
			}
			if !overridden {
				op.Parameters = append(op.Parameters, sp)
			}
		}
		item[method] = op
	}
	*p = item
	return nil
}

type Operation struct {
	OperationID string      `json:"operationId"`
	Summary     string      `json:"summary"`
	Tags        []string    `json:"tags"`
	Parameters  []Parameter `json:"parameters"`
	RequestBody *struct {
		Content map[string]MediaType `json:"content"`
	} `json:"requestBody"`
}

type MediaType struct {
	Schema   json.RawMessage `json:"schema"`
	Example  json.RawMessage `json:"example"`
	Examples map[string]struct {
		Value json.RawMessage `json:"value"`
	} `json:"examples"`
}

type Parameter struct {
	Name     string      `json:"name"`
	In       string      `json:"in"` // query, path, header
	Required bool        `json:"required"`
	Example  interface{} `json:"example"`
	Schema   struct {
		Type    string      `json:"type"`
		Default interface{} `json:"default"`
		Example interface{} `json:"example"`
	} `json:"schema"`
}

// exampleValue returns the parameter's example, if the document gives one.
func (p Parameter) exampleValue() (string, bool) {
	for _, v := range []interface{}{p.Example, p.Schema.Example} {
		if v != nil {
			return fmt.Sprintf("%v", v), true
		}
	}
	return "", false
}

// FetchAndParse fetches an OpenAPI spec (JSON or YAML) from the given URL and parses it.
func FetchAndParse(url string) (*OpenAPISpec, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch openapi: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openapi returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read openapi body: %w", err)
	}
	return ParseSpec(body)
}

// ParseSpec parses an OpenAPI 3 or Swagger 2 document in JSON or YAML.
func ParseSpec(body []byte) (*OpenAPISpec, error) {
	data := bytes.TrimSpace(body)
	if len(data) > 0 && data[0] != '{' {
		var doc interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse openapi: the document is neither JSON nor valid YAML: %w", err)
		}
		converted, err := json.Marshal(jsonCompatible(doc))
		if err != nil {
			return nil, fmt.Errorf("parse openapi: %w", err)
		}
		data = converted
	}

	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}

	if len(spec.Paths) == 0 {
		return nil, fmt.Errorf("openapi spec has no paths")
	}

	return &spec, nil
}

// jsonCompatible turns YAML maps with non-string keys (such as response
// codes) into maps encoding/json can marshal.
func jsonCompatible(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			t[k] = jsonCompatible(val)
		}
		return t
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = jsonCompatible(val)
		}
		return m
	case []interface{}:
		for i, val := range t {
			t[i] = jsonCompatible(val)
		}
		return t
	default:
		return v
	}
}

// ResolveBaseURL picks the base URL generated requests go to. A server on
// the host the document was fetched from wins (specs often list both a
// localhost and an internal address); relative server URLs resolve against
// the document URL; with no servers, the document's own origin is used.
func ResolveBaseURL(spec *OpenAPISpec, specURL string) string {
	doc, err := neturl.Parse(specURL)
	if err != nil || doc.Host == "" {
		if len(spec.Servers) > 0 {
			return strings.TrimRight(spec.Servers[0].URL, "/")
		}
		return ""
	}
	first := ""
	for i, s := range spec.Servers {
		ref, err := neturl.Parse(s.URL)
		if err != nil {
			continue
		}
		abs := doc.ResolveReference(ref)
		if i == 0 {
			first = abs.String()
		}
		if abs.Host == doc.Host {
			return strings.TrimRight(abs.String(), "/")
		}
	}
	if first != "" {
		return strings.TrimRight(first, "/")
	}
	if spec.Swagger != "" {
		host, scheme := spec.Host, doc.Scheme
		if host == "" {
			host = doc.Host
		}
		if len(spec.Schemes) > 0 {
			scheme = spec.Schemes[0]
		}
		return strings.TrimRight(scheme+"://"+host+spec.BasePath, "/")
	}
	return doc.Scheme + "://" + doc.Host
}

// GeneratePlanFromOpenAPI creates a test plan from an OpenAPI spec.
func GeneratePlanFromOpenAPI(spec *OpenAPISpec, baseURL string) *db.Plan {
	title := "API Test Plan"
	if spec.Info.Title != "" {
		title = spec.Info.Title + " Test Plan"
	}

	// Determine base URL
	if baseURL == "" {
		baseURL = ResolveBaseURL(spec, "")
		if baseURL == "" && spec.Host != "" {
			baseURL = "https://" + spec.Host + spec.BasePath
		}
	}

	// Build scenario with one HTTP sampler per operation
	var samplerNodes []db.Node

	// Sort paths for deterministic output
	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		methods := spec.Paths[path]
		// Sort methods too
		methodList := make([]string, 0, len(methods))
		for m := range methods {
			methodList = append(methodList, m)
		}
		sort.Strings(methodList)

		for _, method := range methodList {
			op := methods[method]
			m := strings.ToUpper(method)
			if m == "OPTIONS" || m == "HEAD" {
				continue // Skip non-essential methods
			}

			name := op.Summary
			if name == "" {
				name = op.OperationID
			}
			if name == "" {
				name = m + " " + path
			}

			// Build URL with path params replaced by placeholders
			url := baseURL + path
			headers := []map[string]string{}

			for _, param := range op.Parameters {
				example, hasExample := param.exampleValue()
				switch param.In {
				case "path":
					// Without an example, leave a ${name} variable for an environment or extractor to fill.
					value := fmt.Sprintf("${%s}", param.Name)
					if hasExample {
						value = neturl.PathEscape(example)
					}
					url = strings.Replace(url, "{"+param.Name+"}", value, 1)
				case "query":
					if param.Required {
						sep := "?"
						if strings.Contains(url, "?") {
							sep = "&"
						}
						if !hasExample {
							example = "example"
						}
						url += sep + param.Name + "=" + neturl.QueryEscape(example)
					}
				case "header":
					if !hasExample {
						example = "example"
					}
					headers = append(headers, map[string]string{
						"key": param.Name, "value": example,
					})
				}
			}

			// Build body for POST/PUT/PATCH
			bodyType := "none"
			bodyContent := ""
			if op.RequestBody != nil && (m == "POST" || m == "PUT" || m == "PATCH") {
				if ct, ok := op.RequestBody.Content["application/json"]; ok {
					bodyType = "json"
					bodyContent = spec.exampleBody(ct)
				}
			}

			headersJSON, _ := json.Marshal(headers)
			bodyJSON, _ := json.Marshal(map[string]interface{}{
				"type":    bodyType,
				"content": bodyContent,
			})

			sampler := db.Node{
				ID:      uuid.New().String(),
				Type:    "http",
				Name:    name,
				Enabled: true,
				Properties: mustJSON(map[string]interface{}{
					"method":           m,
					"url":              url,
					"headers":          json.RawMessage(headersJSON),
					"body":             json.RawMessage(bodyJSON),
					"follow_redirects": true,
				}),
				Children: []db.Node{},
			}

			samplerNodes = append(samplerNodes, sampler)
		}
	}

	scenario := db.Node{
		ID:         uuid.New().String(),
		Type:       "scenario",
		Name:       "API Scenario",
		Enabled:    true,
		Properties: json.RawMessage(`{"executor":"constant-vus","vus":1,"duration":"30s"}`),
		Children:   samplerNodes,
	}

	root := db.Node{
		ID:         uuid.New().String(),
		Type:       "plan",
		Name:       title,
		Enabled:    true,
		Properties: json.RawMessage(`{}`),
		Children:   []db.Node{scenario},
	}

	now := time.Now().UTC()
	return &db.Plan{
		ID:          uuid.New().String(),
		Name:        title,
		Description: fmt.Sprintf("Auto-generated from OpenAPI spec: %s", spec.Info.Title),
		Tags:        []string{"auto-generated", "openapi"},
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
		Root:        root,
	}
}

// ProbeOpenAPIEndpoints tries common OpenAPI/Swagger paths and returns the first that works.
func ProbeOpenAPIEndpoints(baseURL string) (string, error) {
	probes := []string{
		"/openapi.json",
		"/swagger.json",
		"/v3/api-docs",
		"/api-docs",
		"/swagger/v1/swagger.json",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, path := range probes {
		url := strings.TrimRight(baseURL, "/") + path
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return url, nil
		}
	}

	return "", fmt.Errorf("no OpenAPI spec found at common paths")
}
