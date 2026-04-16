package crawler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeneratePlanFromOpenAPI(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Pet Store", "version": "1.0"},
		"servers": [{"url": "https://petstore.example.com/api"}],
		"paths": {
			"/pets": {
				"get": {
					"operationId": "listPets",
					"summary": "List all pets",
					"parameters": [
						{"name": "limit", "in": "query", "required": false, "schema": {"type": "integer"}}
					]
				},
				"post": {
					"operationId": "createPet",
					"summary": "Create a pet",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {"type": "object"}
							}
						}
					}
				}
			},
			"/pets/{petId}": {
				"get": {
					"operationId": "getPet",
					"summary": "Get a pet by ID",
					"parameters": [
						{"name": "petId", "in": "path", "required": true, "schema": {"type": "string"}}
					]
				},
				"delete": {
					"operationId": "deletePet",
					"summary": "Delete a pet"
				}
			}
		}
	}`

	var spec OpenAPISpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	plan := GeneratePlanFromOpenAPI(&spec, "")
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}

	if plan.Name != "Pet Store Test Plan" {
		t.Fatalf("expected 'Pet Store Test Plan', got %q", plan.Name)
	}

	// Plan should have a root node with one scenario child
	if plan.Root.Type != "plan" {
		t.Fatalf("expected root type 'plan', got %q", plan.Root.Type)
	}

	if len(plan.Root.Children) != 1 {
		t.Fatalf("expected 1 child (scenario), got %d", len(plan.Root.Children))
	}

	scenario := plan.Root.Children[0]
	if scenario.Type != "scenario" {
		t.Fatalf("expected scenario type, got %q", scenario.Type)
	}

	// Should have 4 HTTP samplers (GET /pets, POST /pets, DELETE /pets/{petId}, GET /pets/{petId})
	if len(scenario.Children) != 4 {
		t.Fatalf("expected 4 HTTP samplers, got %d", len(scenario.Children))
	}

	// Check that path params are replaced with ${...} syntax
	foundPathParam := false
	for _, child := range scenario.Children {
		props := string(child.Properties)
		if strings.Contains(props, "${petId}") {
			foundPathParam = true
		}
	}
	if !foundPathParam {
		t.Fatal("expected path parameter ${petId} in at least one sampler URL")
	}

	// Check that the base URL from servers is used
	foundBaseURL := false
	for _, child := range scenario.Children {
		props := string(child.Properties)
		if strings.Contains(props, "https://petstore.example.com/api") {
			foundBaseURL = true
		}
	}
	if !foundBaseURL {
		t.Fatal("expected base URL from spec.servers in sampler URLs")
	}
}

func TestGeneratePlanDeterministic(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/a": {"get": {"summary": "A"}},
			"/b": {"post": {"summary": "B"}},
			"/c": {"get": {"summary": "C"}}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan1 := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	plan2 := GeneratePlanFromOpenAPI(&spec, "https://example.com")

	// Names should match
	if plan1.Root.Children[0].Children[0].Name != plan2.Root.Children[0].Children[0].Name {
		t.Fatal("expected deterministic output")
	}
}

// --- FetchAndParse tests ---

func TestFetchAndParseSuccess(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0"},
		"paths": {
			"/users": {
				"get": {"summary": "List users"}
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(specJSON))
	}))
	defer srv.Close()

	spec, err := FetchAndParse(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Info.Title != "Test API" {
		t.Fatalf("expected title 'Test API', got %q", spec.Info.Title)
	}
	if len(spec.Paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(spec.Paths))
	}
}

func TestFetchAndParseNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchAndParse(srv.URL)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("expected status 404 in error, got %q", err.Error())
	}
}

func TestFetchAndParseInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := FetchAndParse(srv.URL)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse openapi") {
		t.Fatalf("expected parse error, got %q", err.Error())
	}
}

func TestFetchAndParseNoPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"openapi":"3.0.0","info":{"title":"Empty"},"paths":{}}`))
	}))
	defer srv.Close()

	_, err := FetchAndParse(srv.URL)
	if err == nil {
		t.Fatal("expected error for empty paths")
	}
	if !strings.Contains(err.Error(), "no paths") {
		t.Fatalf("expected 'no paths' error, got %q", err.Error())
	}
}

func TestFetchAndParseUnreachable(t *testing.T) {
	_, err := FetchAndParse("http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "fetch openapi") {
		t.Fatalf("expected fetch error, got %q", err.Error())
	}
}

// --- ProbeOpenAPIEndpoints tests ---

func TestProbeOpenAPIEndpointsFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	url, err := ProbeOpenAPIEndpoints(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(url, "/openapi.json") {
		t.Fatalf("expected /openapi.json, got %q", url)
	}
}

func TestProbeOpenAPIEndpointsSwaggerFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/swagger.json" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	url, err := ProbeOpenAPIEndpoints(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(url, "/swagger.json") {
		t.Fatalf("expected /swagger.json, got %q", url)
	}
}

func TestProbeOpenAPIEndpointsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := ProbeOpenAPIEndpoints(srv.URL)
	if err == nil {
		t.Fatal("expected error when no spec found")
	}
	if !strings.Contains(err.Error(), "no OpenAPI spec found") {
		t.Fatalf("expected 'no OpenAPI spec found', got %q", err.Error())
	}
}

func TestProbeOpenAPIEndpointsTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/api-docs" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Pass URL with trailing slash
	url, err := ProbeOpenAPIEndpoints(srv.URL + "/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(url, "/v3/api-docs") {
		t.Fatalf("expected /v3/api-docs, got %q", url)
	}
}

// --- GeneratePlanFromOpenAPI edge case tests ---

func TestGeneratePlanSwagger2(t *testing.T) {
	specJSON := `{
		"swagger": "2.0",
		"info": {"title": "Legacy API", "version": "1.0"},
		"host": "api.example.com",
		"basePath": "/v1",
		"paths": {
			"/items": {
				"get": {"operationId": "listItems", "summary": "List items"}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "")
	if plan.Name != "Legacy API Test Plan" {
		t.Fatalf("expected 'Legacy API Test Plan', got %q", plan.Name)
	}

	// Should use host + basePath as base URL
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)
	if !strings.Contains(props, "https://api.example.com/v1/items") {
		t.Fatalf("expected Swagger 2 base URL, got %s", props)
	}
}

func TestGeneratePlanBaseURLOverride(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Override", "version": "1.0"},
		"servers": [{"url": "https://spec-server.com"}],
		"paths": {
			"/test": {"get": {"summary": "Test"}}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://custom-base.com")
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)
	if !strings.Contains(props, "https://custom-base.com/test") {
		t.Fatalf("expected custom base URL override, got %s", props)
	}
}

func TestGeneratePlanSkipsOptionsAndHead(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Skip Test", "version": "1.0"},
		"paths": {
			"/resource": {
				"get": {"summary": "Get resource"},
				"options": {"summary": "CORS preflight"},
				"head": {"summary": "Head check"},
				"post": {"summary": "Create resource"}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	samplers := plan.Root.Children[0].Children
	if len(samplers) != 2 {
		t.Fatalf("expected 2 samplers (GET + POST, skipping OPTIONS + HEAD), got %d", len(samplers))
	}
}

func TestGeneratePlanRequiredQueryParam(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Query", "version": "1.0"},
		"paths": {
			"/search": {
				"get": {
					"summary": "Search",
					"parameters": [
						{"name": "q", "in": "query", "required": true, "schema": {"type": "string", "example": "hello"}},
						{"name": "page", "in": "query", "required": false, "schema": {"type": "integer"}}
					]
				}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)

	// Required param 'q' should be in the URL with its example value
	if !strings.Contains(props, "q=hello") {
		t.Fatalf("expected required query param 'q=hello' in URL, got %s", props)
	}
	// Optional param 'page' should NOT be in the URL
	if strings.Contains(props, "page=") {
		t.Fatalf("optional query param 'page' should not be in URL, got %s", props)
	}
}

func TestGeneratePlanHeaderParam(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Headers", "version": "1.0"},
		"paths": {
			"/protected": {
				"get": {
					"summary": "Protected endpoint",
					"parameters": [
						{"name": "X-API-Key", "in": "header", "required": true, "schema": {"type": "string"}}
					]
				}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)
	if !strings.Contains(props, "X-API-Key") {
		t.Fatalf("expected header param 'X-API-Key' in properties, got %s", props)
	}
}

func TestGeneratePlanRequestBody(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Body", "version": "1.0"},
		"paths": {
			"/items": {
				"post": {
					"summary": "Create item",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {"type": "object"}
							}
						}
					}
				}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)
	if !strings.Contains(props, `"type":"json"`) {
		t.Fatalf("expected body type 'json' for POST with requestBody, got %s", props)
	}
}

func TestGeneratePlanFallbackNaming(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Fallback", "version": "1.0"},
		"paths": {
			"/no-summary": {
				"get": {"operationId": "myOperation"}
			},
			"/no-id-or-summary": {
				"put": {}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	samplers := plan.Root.Children[0].Children

	// Find each sampler by checking names
	names := make(map[string]bool)
	for _, s := range samplers {
		names[s.Name] = true
	}

	// No summary → falls back to operationId
	if !names["myOperation"] {
		t.Fatalf("expected fallback to operationId 'myOperation', got names: %v", names)
	}
	// No summary and no operationId → falls back to METHOD + path
	if !names["PUT /no-id-or-summary"] {
		t.Fatalf("expected fallback to 'PUT /no-id-or-summary', got names: %v", names)
	}
}

func TestGeneratePlanNoTitle(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"version": "1.0"},
		"paths": {
			"/x": {"get": {"summary": "X"}}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	if plan.Name != "API Test Plan" {
		t.Fatalf("expected default 'API Test Plan', got %q", plan.Name)
	}
}

func TestGeneratePlanTags(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Tagged", "version": "1.0"},
		"paths": {
			"/x": {"get": {"summary": "X"}}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	foundAutoGen := false
	foundOpenAPI := false
	for _, tag := range plan.Tags {
		if tag == "auto-generated" {
			foundAutoGen = true
		}
		if tag == "openapi" {
			foundOpenAPI = true
		}
	}
	if !foundAutoGen || !foundOpenAPI {
		t.Fatalf("expected tags [auto-generated, openapi], got %v", plan.Tags)
	}
}

func TestGeneratePlanMultipleRequiredQueryParams(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Multi Query", "version": "1.0"},
		"paths": {
			"/filter": {
				"get": {
					"summary": "Filter items",
					"parameters": [
						{"name": "status", "in": "query", "required": true, "schema": {"type": "string"}},
						{"name": "type", "in": "query", "required": true, "schema": {"type": "string"}}
					]
				}
			}
		}
	}`

	var spec OpenAPISpec
	json.Unmarshal([]byte(specJSON), &spec)

	plan := GeneratePlanFromOpenAPI(&spec, "https://example.com")
	sampler := plan.Root.Children[0].Children[0]
	props := string(sampler.Properties)

	// First param uses ?, second uses &
	if !strings.Contains(props, "?status=") {
		t.Fatalf("expected first query param with '?', got %s", props)
	}
	if !strings.Contains(props, "&type=") {
		t.Fatalf("expected second query param with '&', got %s", props)
	}
}
