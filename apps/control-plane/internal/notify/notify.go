// Package notify sends run start/finish notifications to the webhook and
// Slack URLs configured in Settings > Integrations. Nothing is sent unless
// the user has configured a URL.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// Webhooks implements handlers.RunNotifier.
type Webhooks struct {
	Settings *db.SettingsStore
	// PublicURL, if set, is used to link to the run in messages.
	PublicURL string
	Client    *http.Client
}

// Payload is the JSON body posted to a generic webhook.
type Payload struct {
	Event   string         `json:"event"` // run.started | run.completed | run.failed | run.aborted
	RunID   string         `json:"run_id"`
	PlanID  string         `json:"plan_id"`
	Plan    string         `json:"plan_name"`
	Status  string         `json:"status"`
	URL     string         `json:"url,omitempty"`
	Summary *db.RunSummary `json:"summary,omitempty"`
	SentAt  time.Time      `json:"sent_at"`
}

func (n *Webhooks) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (n *Webhooks) RunStarted(run *db.Run)  { go n.send("run.started", run) }
func (n *Webhooks) RunFinished(run *db.Run) { go n.send("run."+run.Status, run) }

func (n *Webhooks) send(event string, run *db.Run) {
	st, err := n.Settings.Get()
	if err != nil {
		return
	}
	p := Payload{Event: event, RunID: run.ID, PlanID: run.PlanID, Plan: run.PlanName, Status: run.Status, Summary: run.Summary, SentAt: time.Now().UTC()}
	if n.PublicURL != "" {
		p.URL = n.PublicURL + "/#/runs/" + run.ID
	}
	if u := st.Integrations.WebhookURL; u != "" {
		n.post(u, p)
	}
	if u := st.Integrations.SlackWebhookURL; u != "" {
		n.post(u, map[string]string{"text": SlackText(p)})
	}
}

// SlackText renders a one-line run notification.
func SlackText(p Payload) string {
	verb := map[string]string{"run.started": "started", "run.completed": "completed", "run.failed": "failed", "run.aborted": "was aborted"}[p.Event]
	msg := fmt.Sprintf("OpenSynapse run of *%s* %s", p.Plan, verb)
	if s := p.Summary; s != nil && p.Event != "run.started" {
		msg += fmt.Sprintf(" — %d requests, %.2f%% errors, p95 %.0f ms, %.1f rps", s.TotalRequests, s.ErrorRate, s.P95MS, s.ThroughputRPS)
	}
	if p.URL != "" {
		msg += " <" + p.URL + "|open>"
	}
	return msg
}

func (n *Webhooks) post(url string, body any) {
	b, _ := json.Marshal(body)
	resp, err := n.client().Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("webhook delivery failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("webhook returned %d", resp.StatusCode)
	}
}
