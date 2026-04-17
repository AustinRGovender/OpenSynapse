package compiler

import (
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// TestAllBuiltInPlansCompile verifies that every seeded built-in plan
// compiles to a valid k6 script without errors.
func TestAllBuiltInPlansCompile(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, err := store.List(db.ListParams{Limit: 100})
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}

	if len(result.Items) != 6 {
		t.Fatalf("expected 6 built-in plans, got %d", len(result.Items))
	}

	for _, plan := range result.Items {
		t.Run(plan.Name, func(t *testing.T) {
			script, err := Compile(&plan)
			if err != nil {
				t.Fatalf("Compile(%q) error: %v", plan.Name, err)
			}

			if script == "" {
				t.Fatalf("Compile(%q) returned empty script", plan.Name)
			}

			// Every compiled script must have these k6 fundamentals
			if !strings.Contains(script, "import http from 'k6/http'") {
				t.Errorf("%q: missing http import", plan.Name)
			}
			if !strings.Contains(script, "export default function") {
				t.Errorf("%q: missing default function", plan.Name)
			}
			if !strings.Contains(script, "export let options") {
				t.Errorf("%q: missing options export", plan.Name)
			}
		})
	}
}

// TestBuiltInPlanSmokeCompileContent verifies the Echo Smoke plan
// compiles with the expected executor, URL, and group structure.
func TestBuiltInPlanSmokeCompileContent(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, _ := store.List(db.ListParams{Limit: 100})

	var smokePlan *db.Plan
	for i := range result.Items {
		if strings.Contains(result.Items[i].Name, "Smoke Test") {
			smokePlan = &result.Items[i]
			break
		}
	}
	if smokePlan == nil {
		t.Fatal("smoke test plan not found")
	}

	script, err := Compile(smokePlan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	checks := []string{
		"constant-vus",
		"vus: 1",
		"duration: '30s'",
		"'Echo-Smoke'",
		"http.get('http://echo-api:9001/health')",
		"http.post('http://echo-api:9001/echo'",
		"http.get('http://echo-api:9001/delay/100')",
		"group('Echo Flow'",
		"sleep(0.500)",
	}

	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("smoke plan script missing %q\n\nScript:\n%s", want, script)
		}
	}
}

// TestBuiltInPlanRampingCompileContent verifies ramping-vus plans
// emit stages correctly.
func TestBuiltInPlanRampingCompileContent(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, _ := store.List(db.ListParams{Limit: 100})

	var ecomPlan *db.Plan
	for i := range result.Items {
		if strings.Contains(result.Items[i].Name, "Browse") {
			ecomPlan = &result.Items[i]
			break
		}
	}
	if ecomPlan == nil {
		t.Fatal("ecom plan not found")
	}

	script, err := Compile(ecomPlan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	checks := []string{
		"ramping-vus",
		"stages:",
		"'Browse-and-Buy'",
		"target: 10",
		"target: 30",
		"target: 50",
		"target: 0",
		"http.get('http://mock-ecommerce:9002/products')",
		"http.post('http://mock-ecommerce:9002/orders'",
	}

	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("ecom plan script missing %q", want)
		}
	}
}

// TestBuiltInPlanSpikeCrossAPI verifies the spike plan hits all 4 APIs.
func TestBuiltInPlanSpikeCrossAPI(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, _ := store.List(db.ListParams{Limit: 100})

	var spikePlan *db.Plan
	for i := range result.Items {
		if strings.Contains(result.Items[i].Name, "Spike") {
			spikePlan = &result.Items[i]
			break
		}
	}
	if spikePlan == nil {
		t.Fatal("spike plan not found")
	}

	script, err := Compile(spikePlan)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Verify all 4 APIs are referenced
	apis := []string{
		"echo-api:9001",
		"mock-ecommerce:9002",
		"slow-api:9003",
		"error-api:9004",
	}
	for _, api := range apis {
		if !strings.Contains(script, api) {
			t.Errorf("spike plan missing reference to %q", api)
		}
	}
}
