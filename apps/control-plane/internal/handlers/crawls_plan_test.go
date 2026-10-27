package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

func TestGeneratePlanFromRequestsDedupesAndKeepsBodies(t *testing.T) {
	crawl := &db.Crawl{
		EntryURL: "http://shop.test/",
		Requests: []db.CapturedRequest{
			{Method: "GET", URL: "http://shop.test/products"},
			{Method: "GET", URL: "http://shop.test/products"},
			{Method: "POST", URL: "http://shop.test/login", Body: "user=alice&pass=s3cret", Headers: map[string]string{"content-type": "application/x-www-form-urlencoded"}},
			{Method: "POST", URL: "http://shop.test/orders", Body: `{"product_ids":["p1"]}`, Headers: map[string]string{"Content-Type": "application/json"}},
		},
	}
	plan := generatePlanFromRequests(crawl)
	nodes := plan.Root.Children[0].Children
	if len(nodes) != 3 {
		t.Fatalf("got %d request nodes, want 3 (duplicates removed)", len(nodes))
	}
	for _, n := range append([]db.Node{plan.Root, plan.Root.Children[0]}, nodes...) {
		if _, err := uuid.Parse(n.ID); err != nil {
			t.Errorf("node %q has non-UUID id %q", n.Name, n.ID)
		}
	}
	if nodes[0].Name != "GET /products" {
		t.Errorf("name = %q, want the path", nodes[0].Name)
	}

	var login struct {
		Body struct {
			Type       string
			FormFields []struct{ Key, Value string } `json:"form_fields"`
		}
	}
	json.Unmarshal(nodes[1].Properties, &login)
	if login.Body.Type != "form" || len(login.Body.FormFields) != 2 || login.Body.FormFields[0].Key != "pass" {
		t.Errorf("login body = %+v, want form fields pass and user", login.Body)
	}

	var order struct {
		Body struct{ Type, Content string }
	}
	json.Unmarshal(nodes[2].Properties, &order)
	if order.Body.Type != "json" || order.Body.Content != `{"product_ids":["p1"]}` {
		t.Errorf("order body = %+v, want the captured JSON", order.Body)
	}
}
