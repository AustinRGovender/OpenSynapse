package crawler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// OpenAPISpec represents a minimal parsed OpenAPI 3.x or Swagger 2.x document.
type OpenAPISpec struct {
	OpenAPI string                        `json:"openapi"`
	Swagger string                        `json:"swagger"`
	Info    struct{ Title, Version string } `json:"info"`
	Paths   map[string]map[string]Operation `json:"paths"`
	Servers []struct{ URL string }         `json:"servers"`
	Host    string                         `json:"host"`    // Swagger 2
	BasePath string                        `json:"basePath"` // Swagger 2
}

type Operation struct {
	OperationID string      `json:"operationId"`
	Summary     string      `json:"summary"`
	Tags        []string    `json:"tags"`
	Parameters  []Parameter `json:"parameters"`
	RequestBody *struct {
		Content map[string]struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

type Parameter struct {
	Name     string `json:"name"`
	In       string `json:"in"` // query, path, header
	Required bool   `json:"required"`
	Schema   struct {
		Type    string `json:"type"`
		Default interface{} `json:"default"`
		Example interface{} `json:"example"`
	} `json:"schema"`
}

// FetchAndParse fetches an OpenAPI spec from the given URL and parses it.
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

	var spec OpenAPISpec
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("parse openapi: %w", err)
	}

	if spec.Paths == nil || len(spec.Paths) == 0 {
		return nil, fmt.Errorf("openapi spec has no paths")
	}

	return &spec, nil
}

// GeneratePlanFromOpenAPI creates a test plan from an OpenAPI spec.
func GeneratePlanFromOpenAPI(spec *OpenAPISpec, baseURL string) *db.Plan {
	title := "API Test Plan"
	if spec.Info.Title != "" {
		title = spec.Info.Title + " Test Plan"
	}

	// Determine base URL
	if baseURL == "" {
		if len(spec.Servers) > 0 {
			baseURL = spec.Servers[0].URL
		} else if spec.Host != "" {
			scheme := "https"
			baseURL = scheme + "://" + spec.Host + spec.BasePath
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
			var headers []map[string]string

			for _, param := range op.Parameters {
				switch param.In {
				case "path":
					placeholder := fmt.Sprintf("${%s}", param.Name)
					url = strings.Replace(url, "{"+param.Name+"}", placeholder, 1)
				case "query":
					if param.Required {
						sep := "?"
						if strings.Contains(url, "?") {
							sep = "&"
						}
						val := "example"
						if param.Schema.Example != nil {
							val = fmt.Sprintf("%v", param.Schema.Example)
						}
						url += sep + param.Name + "=" + val
					}
				case "header":
					headers = append(headers, map[string]string{
						"key": param.Name, "value": "example",
					})
				}
			}

			// Build body for POST/PUT/PATCH
			bodyType := "none"
			bodyContent := ""
			if op.RequestBody != nil && (m == "POST" || m == "PUT" || m == "PATCH") {
				if ct, ok := op.RequestBody.Content["application/json"]; ok {
					bodyType = "json"
					bodyContent = string(ct.Schema)
					if bodyContent == "" {
						bodyContent = "{}"
					}
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
				Properties: json.RawMessage(fmt.Sprintf(`{"method":"%s","url":"%s","headers":%s,"body":%s,"follow_redirects":true}`,
					m, url, string(headersJSON), string(bodyJSON))),
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
