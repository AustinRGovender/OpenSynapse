package crawler

import (
	"encoding/json"
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
