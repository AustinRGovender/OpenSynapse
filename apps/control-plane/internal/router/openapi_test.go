package router_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/handlers"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/router"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/testutil"
)

// TestOpenAPICoversEveryRoute keeps the OpenAPI catalogue (and therefore the
// generated TypeScript client) in lockstep with the routes actually served.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	testutil.NewTestServer(t) // registers every route with full dependencies
	served := map[string]bool{}
	for _, r := range router.Routes {
		served[r] = true
	}
	documented := map[string]bool{}
	for _, op := range handlers.Catalogue {
		documented[op.Method+" "+op.Path] = true
	}
	var missing, stale []string
	for r := range served {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		if !served[d] {
			stale = append(stale, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("routes missing from handlers.Catalogue:\n  %s", strings.Join(missing, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("catalogue entries with no route:\n  %s", strings.Join(stale, "\n  "))
	}
	doc := handlers.Document()
	if len(doc["paths"].(map[string]any)) < 60 {
		t.Error("document looks incomplete")
	}
}
