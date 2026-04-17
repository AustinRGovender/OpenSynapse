package db_test

import (
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func TestSeedBuiltInPlans(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)

	// First seed should create plans
	if err := store.SeedBuiltInPlans(); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	result, err := store.List(db.ListParams{Limit: 100})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(result.Items) != 6 {
		t.Fatalf("expected 6 built-in plans, got %d", len(result.Items))
	}

	// Verify all are marked built-in
	for _, p := range result.Items {
		if !p.BuiltIn {
			t.Fatalf("plan %q should be built-in", p.Name)
		}
	}

	// Second seed should be idempotent
	if err := store.SeedBuiltInPlans(); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	result2, _ := store.List(db.ListParams{Limit: 100})
	if len(result2.Items) != 6 {
		t.Fatalf("idempotent: expected 6, got %d", len(result2.Items))
	}
}

func TestBuiltInPlanCannotBeDeleted(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, _ := store.List(db.ListParams{Limit: 1})
	if len(result.Items) == 0 {
		t.Fatal("no plans found")
	}

	err = store.Delete(result.Items[0].ID)
	if err == nil {
		t.Fatal("expected error deleting built-in plan")
	}
	if err.Error() != "cannot delete built-in plan" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuiltInPlanCannotBeUpdated(t *testing.T) {
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := db.NewPlanStore(database)
	store.SeedBuiltInPlans()

	result, _ := store.List(db.ListParams{Limit: 1})
	if len(result.Items) == 0 {
		t.Fatal("no plans found")
	}

	p := result.Items[0]
	_, err = store.Update(p.ID, "Hacked", p.Description, p.Tags, p.Root, nil)
	if err == nil {
		t.Fatal("expected error updating built-in plan")
	}
	if err.Error() != "cannot modify built-in plan" {
		t.Fatalf("unexpected error: %v", err)
	}
}
