# OpenSynapse — Feature Specification

**Version:** 1.0
**Companion to:** 01-PRD.md, 02-architecture.md

This document turns each feature from the PRD into concrete behaviour, inputs, outputs, edge cases, and acceptance criteria. Claude Code should be able to implement a feature by reading only the relevant section here plus the architecture document.

---

## F-01 Test template gallery

When the user clicks "New test" from the home view or the plans list, the template gallery opens as a modal with a grid of cards. The cards are laid out four per row on desktop widths, two per row on tablet, one per row on mobile.

Each card contains a name, a one-sentence description of what the test is trying to answer, an SVG animation of the load curve, and a "Use template" button. The SVG animation loops every 4 seconds. The loop is tasteful; no bounce or decorative motion.

The templates and their defaults:

Smoke. Verify the test plan works. 1 VU, 30 seconds, single iteration, fails loudly on any non-2xx response. The curve is a flat near-zero line.

Load. Typical expected traffic. Ramp to target over 2 minutes, hold for 10 minutes, ramp down over 2 minutes. Target defaults to 50 VUs. The curve is a trapezoid.

Stress. Find the knee. Staged ramp: 50, 100, 200, 400 VUs at 3 minutes each. The curve is a staircase.

Spike. Sudden burst. Hold at 10 VUs for 1 minute, jump to 500 VUs for 30 seconds, back to 10 VUs for 1 minute. The curve is a flat line with a tall pulse.

Soak. Long-duration stability. 50 VUs for 4 hours. Default is 4 hours with a warning that longer soaks should be configured explicitly. The curve is a flat line.

Breakpoint. Slowly ramp until failure. Start at 10 VUs, add 10 VUs per minute, abort on error rate above 5 percent. Unbounded top end. The curve is an upward ramp that ends at a red X.

Trickle feed. Low constant rate for endurance. 1 request per second for 1 hour using the constant-arrival-rate executor. The curve is a flat thin line.

Ramp-up. Linear increase only. 0 to 200 VUs over 10 minutes. The curve is a diagonal line.

Step load. Discrete plateaus. Increase by 50 VUs every 2 minutes, hold each plateau for 2 minutes, up to 500 VUs. The curve is a staircase with flat tops.

When the user picks a template, the gallery closes and a new plan is created in the builder with the template's settings as the root scenario node. The name defaults to "New {template name} test" and is editable.

Acceptance: all 9 templates render with correct animated SVGs; clicking "Use template" navigates to the builder with a pre-populated plan; pressing Escape dismisses the gallery; the gallery is keyboard-navigable with arrow keys and Enter.

---

## F-02 Visual test plan builder

The builder is a three-pane layout. Left pane: the node tree, hierarchical, collapsible, searchable. Centre pane: a canvas showing the selected branch as a flow diagram. Right pane: the properties form for the selected node.

### Node types

Plan. The root. Has name, description, tags, default environment reference.

Scenario. Defines an executor and its parameters. Children run under that executor. Multiple scenarios can coexist in one plan and run in parallel. Executor options: ramping-vus, constant-vus, constant-arrival-rate, ramping-arrival-rate, shared-iterations, per-vu-iterations, externally-controlled (required for live control; one per plan).

Thread group. Legacy alias for scenario. Provided for users migrating from JMeter to feel at home; translates 1:1 to scenario on save.

HTTP request (sampler). Method, URL, headers, body (none, raw, form, multipart, JSON, from-file), timeout, follow redirects, assertions. URL and body support variable interpolation with `${varName}` syntax.

WebSocket request (sampler). URL, connect timeout, messages to send (with think time between each), expected messages to receive, disconnect behaviour.

Code block (sampler). A JavaScript snippet that runs in the k6 VU context. The user can call `http.get()` and similar functions directly. Used for anything the builder cannot express visually.

If controller. Condition is a JavaScript expression evaluated against the previous response and shared state. Children run only if the condition is true.

Else controller. Must be a sibling immediately after an If controller. Children run if the If controller's condition was false.

Loop controller. Fixed count or while-condition. Children run repeatedly.

Transaction controller. Groups children and reports aggregated metrics. Translates to k6 `group()`.

Once-only controller. Children run once per VU, regardless of how many iterations the VU performs. Used for setup steps like logging in.

Random controller. Picks one child at random per iteration. Weights are optional per child.

Assertion. Attached to a sampler or a controller. Checks status code, body content (substring, regex, JSONPath, XPath), header presence, header value, response time below threshold.

Timer (constant, uniform random, Gaussian). Inserts sleep time after the parent sampler or controller.

Data source. CSV file, JSON file, or inline array. Rows are distributed across VUs using k6's SharedArray semantics.

Environment binding. A named reference to an Environment entity. Scopes variables to the plan.

### Editing interactions

Adding a node: right-click on a parent to open a context menu, or use the "+" button in the tree, or drag from a palette. Dragging reorders. Cut, copy, paste, and duplicate work between plans.

Every property change is saved to the plan document immediately (debounced 300ms). There is no explicit save button. The document is versioned; the user can browse history and revert.

The tree supports search. Typing in the search box filters visible nodes to matches and their ancestors. Matches highlight.

Multi-select allows bulk operations (enable, disable, delete, copy).

### Code view

A "Show script" button toggles a read-only view of the generated k6 script. An advanced mode lets users edit the script and re-parse it back into the plan; nodes that cannot be reverse-engineered are shown as opaque code blocks.

Acceptance: round-tripping a plan through save, load, and compare produces byte-identical generated scripts; the builder handles plans with 1,000 nodes without lag; search finds nodes by name, type, and property values; undo/redo work across all editing operations.

---

## F-03 Endpoint playground

A dedicated tab in the UI with a Postman-style interface. The top strip is the request builder: method dropdown, URL field, send button. Below are tabs for params, headers, body, auth, tests. Below that, the response pane appears after sending: status line, time, size, body with syntax highlighting, headers, cookies, timing breakdown.

Timing breakdown. DNS lookup, TCP connect, TLS handshake, request sent, time to first byte, content download. Each is measured and displayed as a stacked bar.

Auth helpers. Basic auth (username, password). Bearer token (token). API key (key, value, add to header or query). OAuth 2.0 authorization code (opens a browser window to the auth URL, captures the callback, exchanges the code, stores the access token for reuse). Inherit from environment (uses variables from the active environment).

Environment selector. Dropdown in the top-right. Switching environment updates variable interpolation throughout the request.

Collections. Save requests into collections for later. Collections live alongside test plans in the database but are a distinct entity (they are not test plans themselves).

Save to plan. Button that opens a picker: "Add to which plan?" and "Add to which node?". On confirm, the request becomes an HTTP sampler child of the chosen node in the chosen plan.

Run as micro-test. Button that runs the request N times with a user-specified concurrency (up to 50 VUs, 1 minute max). Shows a mini-dashboard. Not stored as a run; for quick validation only.

Acceptance: all seven auth methods work; OAuth flow completes in under 30 seconds for a typical provider; saving to plan inserts a fully-configured sampler node; the timing breakdown numbers sum to the total time within 5ms.

---

## F-04 Crawler

The crawler feature opens from a "Crawl application" button on the home view. The user provides an entry URL, optional auth, crawl depth (default 3), same-origin toggle (default on), path blocklist (default includes `/logout`, `/delete`, and methods DELETE by default), and a "stop after N requests" safety limit (default 500).

### Crawl execution

The crawler spawns a headless Chromium instance via Playwright. It navigates to the entry URL. If auth is configured, it performs the auth flow: form login fills username and password fields (located by heuristics: input[type=text] or input[type=email] for username, input[type=password] for password, button[type=submit] or input[type=submit] for the submit), bearer auth injects the Authorization header, OAuth runs a headful flow once with user supervision to capture tokens.

After auth, the crawler discovers links by scanning the DOM for `<a href>`, visible buttons that trigger navigation, and form submissions. It maintains a frontier queue. For each page, it records every network request (XHR, fetch, document loads), including request method, URL, headers, request body, response status, response headers, and response body. Sensitive headers (Authorization, Cookie) are stored in a sanitised form that references the auth config rather than hard-coding values.

The crawler respects the depth limit measured from the entry URL, the same-origin restriction by URL origin, and the blocklist by substring match on URL path or by HTTP method.

### Graph and correlation

The output is a graph where nodes are pages and endpoints and edges are traversals. For each edge, the crawler records the trigger (which action on the source page produced the request).

Correlation runs as a post-processing step. For every value that appears in a response (parsed from JSON, HTML forms, HTTP headers), the correlator searches all subsequent requests for the same value. Matches are recorded as correlation hints: "this value in request B came from response A, field X". Common patterns (CSRF tokens, session IDs, JWTs, numeric IDs) are detected with heuristics and flagged with higher confidence.

### Plan generation

The plan generator takes a crawl result and produces a draft plan. It groups requests by endpoint prefix (same scheme, host, path minus the last segment) into logical flows. Each flow becomes a transaction controller. Correlated values become variable extractors and parameters. Form login becomes a once-only controller with an HTTP sampler. The generated plan opens in the builder for user review.

### OpenAPI path

If the target exposes an OpenAPI or Swagger document (the crawler probes common paths: `/swagger.json`, `/openapi.json`, `/v3/api-docs`, `/api-docs`), the crawler uses it directly. Each operation becomes an HTTP sampler. Example values from the schema populate request parameters. Security schemes map to auth helpers.

### Re-crawl and diff

The user can re-run the crawler on the same target and compare the new graph against the previous one. The diff view highlights added pages, removed pages, changed endpoints (URL or method), and changed response shapes. The user chooses which changes to merge into the existing plan.

Acceptance: crawling a vanilla WordPress site yields a working plan that can run without manual edits; crawling a SPA with client-side routing discovers all visible routes; the blocklist prevents destructive actions from being recorded; correlation detects CSRF tokens in at least three common framework patterns (Django, Rails, ASP.NET).

---

## F-05 Live test control

During a run, the live control panel appears in a fixed position in the run view. It shows three controls.

VUs slider. Current active VUs on the left, target VUs on the right, slider in between. Range is 0 to the pre-allocated maxVUs. Dragging the slider updates the target number; clicking "Apply" sends the change to the workers.

RPS slider. Current measured RPS on the left, target RPS on the right. Range is 0 to a sensible upper bound derived from the plan. Applying updates the token bucket in the kv store.

Duration. Current elapsed time and remaining time. A "+5 minutes" and "-5 minutes" button and a "set end time" picker. Applying updates the effective end time.

All changes log to the run's event stream. The run's charts show a vertical marker at the moment of each change with a tooltip describing what changed.

A "Pause" button pauses all workers via k6's REST API. A "Resume" button resumes. A "Stop" button gracefully stops the run (runs teardown, flushes metrics, marks the run as complete). A "Kill" button forcefully stops if graceful stop hangs.

Acceptance: VU changes take effect within 2 seconds; RPS changes take effect within the next full second of iteration; all changes are visible in the run timeline; pausing and resuming preserves in-flight iterations correctly.

---

## F-06 Real-time dashboard

The run view shows six default charts during and after a run.

RPS over time. Line chart. Y axis is requests per second. Shows total and per-endpoint as toggleable series.

Response time percentiles. Line chart with four series: p50, p90, p95, p99. Y axis is milliseconds.

Error rate. Area chart. Y axis is percentage. Coloured red above a configurable threshold.

Active VUs. Step chart. Shows the VU count as it changes over time.

Throughput. Two-line chart: bytes sent and bytes received per second.

Error log. Scrolling list of the last 50 errors with status code, URL, and response body excerpt. New errors highlight briefly.

Users can add custom charts via a "+" button. Each chart selects a metric (built-in or user-tagged), an aggregation (avg, percentile, rate), and optional filters (by endpoint, by tag).

Charts update every 1 second during a run. After a run, the same charts render from the persisted dataset at full fidelity.

Acceptance: charts remain responsive (no frame drops) with 10,000 samples per second incoming; switching between live and historical views is seamless; custom charts persist to the user's preferences.

---

## F-07 Run history and comparison

The runs list shows every run with columns for name, plan, start time, duration, status (running, passed, failed, aborted), peak VUs, and result summary (p95 response time, error rate). Filters: date range, plan, status, tag. Sort by any column.

Clicking a run opens the run view. Clicking the checkbox on multiple runs enables the "Compare" button.

The comparison view takes the selected runs and overlays their metrics. Each selected metric produces a chart with one line per run, distinguished by colour and labelled in a legend. The top of the view shows a summary block: which runs are being compared, which metrics improved, which degraded, and by how much (absolute and percentage).

Differences are computed per second of elapsed time within the shorter of the runs, or normalised to run progress (0 to 100 percent complete) if the user prefers.

AI analysis (if configured) produces a paragraph explaining the differences in plain language: "Run 2 shows a 34 percent improvement in p95 latency starting around the 50-VU mark, which coincides with the JVM warm-up period. Run 1 plateaus higher from 100 VUs onward, suggesting a fixed bottleneck at that concurrency."

Acceptance: up to 5 runs can be compared simultaneously without UI lag; comparison views export to PDF and HTML; the improvement/degradation summary uses statistical significance (a 2 percent change on a noisy metric is not flagged).

---

## F-08 Export and sharing

Every run and every comparison exports to four formats.

PDF. Formatted report with a cover page, summary statistics, each chart as a rendered image, and an appendix of key events. Generated via a headless Chromium render of a print stylesheet.

HTML. Single-file standalone report. Charts are live (rendered in the browser from embedded JSON). The file can be opened offline in any modern browser. Useful for email and ticket attachments.

CSV. Raw metric samples. One row per sample. Columns: timestamp, metric, value, tags.

JSON. The full result document as stored in the database. Re-importable into another OpenSynapse instance.

Acceptance: PDF renders in under 30 seconds for a typical run; HTML is self-contained with no external dependencies; CSV handles runs with 10 million samples without OOM; JSON round-trips cleanly.

---

## F-09 AI analysis

AI is disabled by default. Settings has an AI tab where the user selects a provider (OpenAI, Anthropic, Azure OpenAI) and pastes an API key. The key is validated with a minimal test call and stored in the OS keychain. A model dropdown lets the user choose which model to use (sensible defaults per provider, override allowed).

When enabled, an "Analyse with AI" button appears on run and comparison views. Clicking it shows a modal with a preview of the prompt that will be sent: the run summary, the metrics extracted, and the specific question. The user confirms and the request is sent. The response is rendered as markdown in an insights panel beside the charts.

Three standard questions are pre-built. "What does this run tell me?" for single runs. "What changed between these runs?" for comparisons. "What test should I run next?" for follow-up suggestions.

Users can also type free-form questions. The context sent with each question is the run summary (not the full raw data, which would be huge and expensive).

Costs are visible. Each analysis shows the token count used and an estimated cost. A monthly spend cap can be set in settings.

Acceptance: no metrics are sent to any AI provider without explicit button click; the prompt preview accurately reflects what will be sent; analysis responses render as readable markdown; cost estimates are within 10 percent of actual provider billing.

---

## F-10 Reusable fragments

The fragments library is a sidebar accessible from the builder. It lists all saved fragments with name, tags, and a preview of the node types they contain.

Shipped fragments. Generic form login. CSRF token extraction. Pagination walker. Search-then-select. Cart checkout. File upload with multipart. File download with hash verification. OAuth authorization code flow. SAML login stub (requires user configuration of the IdP). Wait-for-condition polling.

User fragments. The user can select a subtree in any plan and click "Save as fragment". The fragment is stored with a name and optional tags. Fragments can include variable bindings that the user supplies when inserting.

Inserting a fragment: drag from the sidebar into the tree, or right-click a parent node and choose "Insert fragment". The variable binding dialog appears if the fragment has any.

Acceptance: all 10 shipped fragments run successfully against a provided demo target; user fragments preserve assertions and timers; fragments handle variable collisions by renaming with the user's confirmation.

---

## F-11 Settings and configuration

A settings view with tabs for general, appearance, AI, integrations, storage, and about.

General. Default environment for new plans. Default template for "New test". Keyboard shortcuts (viewable and customisable).

Appearance. Theme (system, light, dark). Density (compact, comfortable). Chart colour palette (the default accent palette or a high-contrast palette for accessibility).

AI. As described in F-09.

Integrations. Webhook URL for run events (fired on start, completion, failure). Slack webhook shortcut (just a URL and channel). Prometheus remote-write target for streaming OpenSynapse's own metrics outward. Git integration for storing plans as files in a repo (push on save, pull on open).

Storage. Database location (desktop mode). Retention policy for runs (keep all, keep last N, keep last N days). Manual cleanup button with a size preview.

About. Version, license, links to docs, log file location, diagnostic bundle export button.

Acceptance: all settings persist across restarts; keyboard shortcut customisation does not break default shortcuts for other users; diagnostic bundle contains logs and config but no secrets.

---

## F-12 Import from JMeter

For users migrating from JMeter, a one-click import of .jmx files. The importer parses the XML, maps JMeter elements to OpenSynapse node types, and produces a draft plan. Unsupported elements are preserved as code block nodes with a comment explaining what was lost. The user reviews and saves.

Supported elements: ThreadGroup, HTTPSamplerProxy, HeaderManager, CSVDataSet, IfController, WhileController, LoopController, TransactionController, OnceOnlyController, ConstantTimer, UniformRandomTimer, ResponseAssertion, JSONPathAssertion, RegexExtractor, JSONExtractor, XPath2Extractor.

Unsupported: JSR223 samplers with arbitrary Groovy (preserved as opaque code blocks), BeanShell, JDBC samplers (v1.1), SOAP samplers (v1.1), JMS samplers (not planned).

Acceptance: importing a typical JMeter plan produces a runnable OpenSynapse plan with at most minor manual cleanup; the import log clearly states what was lost or approximated.
