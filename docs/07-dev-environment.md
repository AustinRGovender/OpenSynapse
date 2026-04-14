# OpenSynapse — Development Environment PRD

**Version:** 1.0
**Audience:** Claude Code (to be executed before any product code is written)
**Related:** 01-PRD.md through 06-implementation-plan.md

---

## 1. Purpose

This document specifies the development environment that must be set up inside the OpenSynapse repository before product implementation begins. The goal is to give the project the best chance of being built correctly the first time by encoding standards, quality gates, workflows, and expertise into Claude Code's extensibility primitives (skills, subagents, hooks, slash commands, CLAUDE.md, and MCP servers) rather than relying on ad-hoc prompting.

The core insight driving this PRD is that CLAUDE.md and hooks are deterministic — they apply every time with no model discretion — while skills and subagents are probabilistic. Critical rules go in hooks. Durable policy goes in CLAUDE.md. Repeatable procedures go in skills. Heavy exploration and isolation-critical work goes in subagents. Getting this split right is the difference between a tool that "remembers" standards and one that drifts as context fills up.

## 2. Principles

Determinism over discretion for anything that matters. If a rule must always hold — no hex colours outside tokens, no commits of secrets, no skipping tests — it is enforced by a hook, not by a paragraph in CLAUDE.md.

Short context windows beat long ones. CLAUDE.md should be under 200 lines. Domain knowledge lives in skills that are loaded on demand, not in the always-on context.

Subagents for exploration. When Claude Code needs to understand a large file, trace a problem across many files, or consult external documentation, it spawns a subagent so the main thread keeps its context clean.

Quality gates at the right layer. Linting, type checking, formatting, and test execution run as hooks after file writes, not as reminders in a prompt.

Build the setup once. Every piece of this environment is a file in the repo. A new machine (or a new contributor, including another Claude Code session) checks out the repo and gets the same environment for free.

## 3. Extensibility primitives — what each is for

Before specifying what to build, a short reference for the primitives in play. Claude Code should use these definitions when deciding where to put a given rule or procedure.

**CLAUDE.md** is the top-of-context policy file. It is read automatically at the start of every session. Put durable project-wide rules here: tech stack, commands, hard constraints, where to find other documents. Keep it short.

**Skills** live in `.claude/skills/{skill-name}/SKILL.md`. Each skill has frontmatter with a description, trigger conditions, and tool permissions. Claude Code auto-invokes a skill when the conversation matches its description. Skills are the right home for procedural knowledge: how to add a new node type, how to run a database migration, how to write a Storybook entry. They can be manually invoked with a slash command or automatically loaded when their description matches.

**Subagents** live in `.claude/agents/{agent-name}.md`. Each subagent runs in its own isolated context window with its own system prompt, tools, and model. Subagents return a summary to the main thread, not their full conversation. Use subagents for code review, documentation lookup, architecture exploration, and any work that would otherwise pollute the main context. Subagents cannot spawn other subagents.

**Hooks** are shell commands defined in `.claude/settings.json` under a `hooks` key. They fire deterministically on lifecycle events: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, SessionEnd, Stop, SubagentStop, and others. Hooks can block actions (exit code 2) or just observe. Use hooks for anything that must always happen: lint on write, block secret commits, run tests after changes.

**Slash commands** live in `.claude/commands/{name}.md`. They are user-initiated shortcuts. Many slash commands are thin wrappers that invoke a skill or a subagent.

**MCP servers** connect Claude Code to external services (databases, GitHub, documentation sources). They live in `.claude/mcp.json`. Use sparingly; each adds context weight.

**Plugins** are not used in v1 of this setup. The repo itself is the distribution unit.

## 4. Deliverables

Claude Code must produce the following files and configurations in the repository. Section 5 onwards specifies each in detail.

Top-level:
- `CLAUDE.md` — project policy file
- `.claude/settings.json` — hook configuration and environment defaults
- `.claude/mcp.json` — MCP server configuration (empty in v1, structured for future)

Skills (in `.claude/skills/`):
- `add-api-endpoint/` — how to add a new REST endpoint
- `add-plan-node-type/` — how to add a new test plan node type
- `create-react-component/` — how to scaffold a React component with Storybook
- `run-db-migration/` — how to create and apply a schema migration
- `update-openapi-client/` — how to regenerate the TypeScript client after API changes
- `write-integration-test/` — how to write an integration test for the control plane
- `verify-k6-script/` — how to validate a generated k6 script with `k6 inspect`
- `phase-acceptance/` — how to run the acceptance tests for the current implementation phase
- `create-adr/` — how to create an Architecture Decision Record

Subagents (in `.claude/agents/`):
- `architecture-explorer` — navigates the codebase and returns architectural summaries
- `code-reviewer` — reviews diffs against the project standards
- `spec-consultant` — looks up rules in the OpenSynapse spec documents
- `test-runner` — runs the full test suite and reports failures with context
- `security-auditor` — checks for secrets, unsafe patterns, and permission issues
- `perf-engineer-expert` — answers questions about k6, load testing patterns, and the live-control architecture

Hooks (configured in `settings.json`):
- Pre-commit secret scanner (blocks commits with API keys, tokens, .env files)
- Post-write linter (runs ESLint or gofmt based on file type)
- Post-write type checker (runs tsc or go vet)
- Post-write test runner (runs relevant unit tests for the changed files)
- UserPromptSubmit context loader (reminds Claude of the current phase from progress.md)
- Pre-bash dangerous command blocker (blocks rm -rf, force pushes, etc.)
- Hex colour guard (blocks writes to component files containing hex colours outside the tokens file)
- OpenAPI drift guard (after editing a control plane handler, flags the TypeScript client as stale)

Slash commands (in `.claude/commands/`):
- `/phase-start` — start a new implementation phase
- `/phase-done` — run acceptance tests and commit the phase
- `/review` — invoke the code-reviewer subagent on uncommitted changes
- `/spec` — invoke the spec-consultant subagent with a question
- `/adr` — create a new ADR from a template
- `/test` — run the test suite via the test-runner subagent
- `/explore` — invoke the architecture-explorer subagent

Supporting files:
- `docs/decisions/0000-template.md` — ADR template
- `docs/progress.md` — phase tracking file, updated at each phase boundary
- `scripts/dev/` — developer helper scripts invoked by hooks and skills
- `.vscode/` — VS Code workspace settings if the user develops in VS Code
- `.editorconfig` — baseline editor settings
- `.gitignore`, `.gitattributes` — version control setup

## 5. CLAUDE.md specification

The root CLAUDE.md is the always-loaded policy file. It must be concise (under 200 lines), authoritative, and focused on what Claude Code needs to know at the start of every session. Anything longer or more specific lives in skills or linked documents.

Content sections in order:

**Project overview.** Two sentences: what OpenSynapse is, why it exists. Reference the full PRD at `docs/01-PRD.md`.

**Tech stack.** A short table: control plane is Go 1.22+, web app is React 18 + TypeScript + Vite, desktop shell is Tauri 2, load engine is k6 with xk6-kv, database is SQLite + Parquet on desktop and Postgres + S3 on cluster, UI kit is shadcn/ui on Tailwind, charts are Recharts, plan graph is React Flow.

**Where to find things.** Pointers to each spec document: PRD, architecture, feature spec, data model, UI/UX spec, implementation plan, progress log, ADRs. One line each.

**Hard constraints.** A short list of rules that must never be broken: no hex colours outside the tokens file; no commits of `.env`, `.key`, `.pem`, or `creds*`; no phone-home code in the product; k6 is the load engine and must not be substituted; all endpoints need tests and all components need Storybook entries; live control works via externally-controlled executor plus xk6-kv.

**Current phase.** A line that says "Current phase: see `docs/progress.md`". This is explicit so Claude Code remembers to check progress before picking up work.

**How to work.** Two paragraphs. First: work phase by phase per `docs/06-implementation-plan.md`, stop at phase boundaries for confirmation, commit at the end of each phase with the convention "Phase N: <description>". Second: when making decisions not covered by specs, use the `/adr` command to record them; when unsure about a spec, use the `/spec` command to consult the spec-consultant subagent.

**Commands.** The 5 to 10 most frequently used shell commands for the project: `pnpm dev`, `pnpm test`, `pnpm lint`, `go test ./...`, `cargo tauri dev`, etc. This lets Claude Code invoke them directly without searching.

**Never do.** Explicitly listed: never commit secrets, never use `rm -rf`, never bypass hooks, never write hex colours in component files, never modify spec documents without asking.

That is the whole file. Do not add a glossary, architecture discussion, long examples, or per-feature notes. Those live elsewhere.

## 6. Skills specification

Each skill is a directory containing a `SKILL.md` file with YAML frontmatter. The frontmatter includes `name`, `description` (this is what Claude Code matches on to auto-invoke), `tools` (allowed tools), and optional `disable-model-invocation` (set to true for manual-only).

Skills are documented below with their purpose, when they should fire, and an outline of their content. Claude Code writes the full SKILL.md bodies by following the specifications.

### 6.1 `add-api-endpoint`

**Purpose.** Standardises how a new REST endpoint is added to the Go control plane and exposed through the TypeScript client.

**Auto-invoke when.** The conversation involves adding, modifying, or removing a REST API endpoint.

**Body outline.** Step 1: add or update the OpenAPI spec in `apps/control-plane/api/openapi.yaml`. Step 2: add the handler function in the relevant package under `apps/control-plane/internal/handlers/`. Step 3: register the route in `apps/control-plane/internal/router/router.go`. Step 4: add a unit test and an integration test following the patterns in `write-integration-test`. Step 5: run the OpenAPI client regeneration script. Step 6: update any web app code that consumes the endpoint. Step 7: confirm the new endpoint appears in `/openapi.json` at runtime.

### 6.2 `add-plan-node-type`

**Purpose.** Walks through the five files that must be touched when adding a new test plan node type (for example, a new type of controller).

**Auto-invoke when.** The conversation involves adding a new node type to the plan schema, plan builder, or plan-to-script transformer.

**Body outline.** Step 1: add the JSON schema in `packages/plan-schema/schemas/nodes/`. Step 2: add the TypeScript type in `packages/plan-schema/src/types.ts`. Step 3: add a transformer case in `apps/control-plane/internal/compiler/compiler.go` that emits k6 script for the new type. Step 4: add a React form component and a React Flow node component in `apps/web/src/components/plan-builder/`. Step 5: add a unit test for the schema validation and a round-trip test for the transformer. Step 6: update the builder palette in `apps/web/src/config/nodes.ts`. Include a link to feature spec section F-02.

### 6.3 `create-react-component`

**Purpose.** Scaffolds a new React component with a Storybook entry, tests, and design-token-only colours.

**Auto-invoke when.** The conversation involves creating a new UI component or view.

**Body outline.** File layout: `packages/ui/src/components/{Component}/index.tsx`, `{Component}.stories.tsx`, `{Component}.test.tsx`. The component is typed, accessible (ARIA attributes where relevant, keyboard handlers), and uses only Tailwind classes from the project palette. Storybook entry has at least a default story and a "with all props" story. Test covers rendering and one interaction. Include the rule: no hex colours; if a colour is needed, add it to `packages/ui/src/tokens.ts` first. Reference `docs/05-ui-ux-spec.md` for the design language.

### 6.4 `run-db-migration`

**Purpose.** How to create a new migration file, apply it locally, and handle both SQLite and Postgres.

**Auto-invoke when.** The conversation involves changing database schema or adding a migration.

**Body outline.** Migration files live in `apps/control-plane/internal/db/migrations/` named `NNNN_short_description.up.sql` and `NNNN_short_description.down.sql`. Every migration must be reversible. Use portable SQL (no SQLite-only or Postgres-only syntax unless necessary, in which case maintain two sets). Apply locally with `go run ./cmd/migrate up`. Add a fixture update if the migration changes a table referenced in tests. Commit migration and test update in the same commit.

### 6.5 `update-openapi-client`

**Purpose.** Ensures the TypeScript API client stays in sync with the Go handlers.

**Auto-invoke when.** A handler, route, or OpenAPI definition file is modified.

**Body outline.** Run `pnpm --filter api-client generate` (wraps openapi-typescript). Verify the generated file compiles. If any consumer in the web app breaks, fix it in the same commit. Do not commit a partial update.

### 6.6 `write-integration-test`

**Purpose.** Patterns for writing control plane integration tests against a seeded test database.

**Auto-invoke when.** The conversation involves writing or modifying tests for the control plane.

**Body outline.** Test file location convention, the test database setup helper, the HTTP client helper, the common fixtures, how to assert on structured error responses. Prefer table-driven tests. Clean up after every test. Do not share state between tests. Reference `docs/04-data-model-and-api.md` section 4 for the expected error response shape.

### 6.7 `verify-k6-script`

**Purpose.** After the plan-to-script transformer changes, validate the output by running `k6 inspect` and optionally `k6 run --iterations=1` on a throwaway target.

**Auto-invoke when.** The conversation involves changes to `apps/control-plane/internal/compiler/` or generating k6 scripts from plans.

**Body outline.** Steps: write the compiled script to a temp file, run `k6 inspect script.js`, check the exit code and output for errors, optionally run against `https://test.k6.io` with a low-volume override. If inspect fails, the compiler change is broken and must be fixed before commit.

### 6.8 `phase-acceptance`

**Purpose.** Runs the acceptance tests for the current implementation phase as defined in `docs/06-implementation-plan.md`.

**Auto-invoke when.** The user runs `/phase-done` or says they want to finish a phase.

**Body outline.** Read `docs/progress.md` to determine the current phase. Look up the phase's acceptance criteria in the implementation plan. Execute each criterion (build commands, test commands, verification steps). Report pass or fail per criterion. If all pass, update `docs/progress.md` with the completion entry and prompt the user for commit confirmation. If any fail, stop and report the failure with actionable next steps.

### 6.9 `create-adr`

**Purpose.** Creates a new ADR (Architecture Decision Record) from a template, numbered and titled.

**Auto-invoke when.** The user runs `/adr` or says they want to record a decision.

**Body outline.** Read `docs/decisions/` to find the next available number. Copy `0000-template.md` to `NNNN-{title}.md`. Fill in the frontmatter with date, status "proposed", and stub sections for Context, Decision, Alternatives Considered, and Consequences. Open the file for editing.

## 7. Subagents specification

Each subagent is a Markdown file in `.claude/agents/` with YAML frontmatter. Frontmatter includes name, description (used for auto-delegation), tools, model, and optional permissionMode. The body is the subagent's system prompt.

Subagents run in isolated contexts. The main thread sends them a task and receives a summary. Design each subagent's prompt assuming it starts with no context beyond what is in the task message.

### 7.1 `architecture-explorer`

**Purpose.** Given a question about how something works in the codebase, explores the relevant files and returns a concise explanation with file:line references. Prevents the main thread from having to read large swathes of code.

**When to delegate.** Main thread needs to understand how an existing feature works, where a given piece of logic lives, or how two subsystems connect.

**Tools.** Read, Glob, Grep, Bash (read-only commands).

**Prompt outline.** You are an architecture explorer for the OpenSynapse project. Your job is to answer "how does X work" questions by reading the code and returning a concise explanation. Always start by reading `CLAUDE.md` and any relevant files in `docs/`. Follow imports and function calls. Return your findings as a short report: the question you were given, the files and lines most relevant to the answer, a summary of how the system works, and any concerns you noticed. Do not make changes. Do not speculate; if you cannot find something, say so.

### 7.2 `code-reviewer`

**Purpose.** Reviews uncommitted changes against the project standards and returns an approval or a list of issues.

**When to delegate.** User runs `/review` or the main thread wants a second look before committing.

**Tools.** Read, Bash (for `git diff`, `git status`).

**Prompt outline.** You are a code reviewer for the OpenSynapse project. Your job is to review uncommitted changes for correctness, adherence to project standards, and consistency with the specs in `docs/`. Run `git diff` and `git diff --cached` to see the changes. Check against: the design token rule (no hex colours in components), test coverage (every new function has a test, every new component has a Storybook entry), the TypeScript client sync rule (if handlers change, the client must be regenerated), the ADR rule (if a non-obvious decision was made, there should be an ADR). Return a structured review: summary, pass/fail, list of issues with file and line, suggested fixes. Be strict but constructive. Do not edit files; the main thread will apply fixes.

### 7.3 `spec-consultant`

**Purpose.** Answers questions about what the OpenSynapse spec documents say. The main thread uses this instead of searching specs itself, keeping the main context lean.

**When to delegate.** Main thread is unsure what a spec requires for a given feature or behaviour.

**Tools.** Read, Glob, Grep.

**Prompt outline.** You are the OpenSynapse spec consultant. You answer questions about what the specs say without interpretation beyond quoting and pointing to the exact location. Read the relevant files in `docs/` (the main ones are 01-PRD.md through 06-implementation-plan.md, plus this dev environment PRD at 07-dev-environment.md). When asked a question, find the relevant passages, quote them briefly, and give the file name and section. If the specs are silent or ambiguous, say so explicitly and recommend creating an ADR. Never invent rules.

### 7.4 `test-runner`

**Purpose.** Runs the test suite (unit, integration, or both) and returns a structured report.

**When to delegate.** User runs `/test` or the phase acceptance skill needs to check test results.

**Tools.** Bash, Read.

**Prompt outline.** You are the test runner for OpenSynapse. Given a scope (unit, integration, e2e, or all), run the appropriate command from `CLAUDE.md`, capture the output, and return a structured summary: total tests, passed, failed, skipped, duration, and for any failures, the test name, the failure message, and the file and line. If tests fail, do not attempt to fix them; that is the main thread's job. Report and return.

### 7.5 `security-auditor`

**Purpose.** Scans for secrets, unsafe patterns, and permission issues before commits.

**When to delegate.** Pre-commit hook triggers, or user explicitly asks for a security review.

**Tools.** Read, Bash (for `grep`, `git diff`).

**Prompt outline.** You are a security auditor for OpenSynapse. Scan the staged diff for: hard-coded API keys or tokens (high-entropy strings), `.env` files being committed, password fields with default values, SQL string concatenation that could indicate injection, use of `eval` or equivalent, shell commands constructed from user input, and any code that makes outgoing requests to domains other than the ones in the AI config allowlist. Report findings with file and line. Block commit (exit code 2) if any high-severity issues are found.

### 7.6 `perf-engineer-expert`

**Purpose.** Domain expert for k6, load testing patterns, and the OpenSynapse live-control architecture. Answers questions that would otherwise require a web search or a deep read of k6 docs.

**When to delegate.** Main thread needs guidance on k6 executor choice, metric semantics, xk6 extension behaviour, or how to implement a specific load pattern.

**Tools.** Read, WebFetch (scoped to grafana.com, github.com/grafana).

**Prompt outline.** You are a performance engineering expert specialising in k6. You know: every k6 executor and when to use it (shared-iterations, per-vu-iterations, constant-vus, ramping-vus, constant-arrival-rate, ramping-arrival-rate, externally-controlled); the exact semantics of `http_req_duration` and how k6 differs from JMeter's response time; how the externally-controlled executor exposes the REST API for runtime VU changes; how to build xk6 extensions and the xk6-kv pattern for shared state; how OpenSynapse specifically uses the externally-controlled executor plus an xk6-kv token bucket for runtime RPS and a should-stop flag for runtime duration (see `docs/02-architecture.md` section 3). Answer questions with concrete code examples where helpful. If you need to check current docs, use WebFetch on grafana.com.

## 8. Hooks specification

Hooks are configured in `.claude/settings.json`. The exact JSON structure follows Claude Code's current spec. Each hook below is described by its event, matcher, command, and blocking behaviour.

### 8.1 Secret scanner (pre-commit, blocking)

**Event.** PreToolUse on Bash commands that run `git commit` or `git push`.

**Command.** A shell script at `scripts/dev/hooks/check-secrets.sh` that scans staged files for known secret patterns (API key prefixes, high-entropy strings, `.env`, `.key`, `.pem`) using a short regex set. Exit 2 if any match.

**Why.** Deterministic protection against leaked credentials. CLAUDE.md says "do not commit secrets"; the hook enforces it.

### 8.2 Post-write linter (non-blocking)

**Event.** PostToolUse on Write and Edit tools.

**Command.** A script at `scripts/dev/hooks/lint-file.sh` that inspects the file extension and runs the appropriate linter: ESLint for `.ts`, `.tsx`, `.js`, `.jsx`; `gofmt -l` and `go vet` for `.go`; `rustfmt --check` for `.rs`. Output is informational; it does not block.

**Why.** Keeps code formatted without making Claude ask.

### 8.3 Post-write type checker (non-blocking)

**Event.** PostToolUse on Write and Edit tools, matching `.ts`, `.tsx`, or `.go` files.

**Command.** A script at `scripts/dev/hooks/typecheck-file.sh` that runs `tsc --noEmit` scoped to the file's package for TypeScript or `go build ./...` scoped to the file's package for Go.

**Why.** Catches type errors immediately.

### 8.4 Post-write test runner (non-blocking)

**Event.** PostToolUse on Write and Edit tools.

**Command.** A script at `scripts/dev/hooks/test-related.sh` that finds and runs tests related to the changed file (same package for Go, same directory for TypeScript). Output is informational.

**Why.** Tight feedback loop on regressions.

### 8.5 User prompt context loader (non-blocking)

**Event.** UserPromptSubmit.

**Command.** A script that echoes the first 30 lines of `docs/progress.md` into Claude's context so it always knows which phase it is in. Claude Code's UserPromptSubmit hook output is injected as a system message.

**Why.** Prevents phase confusion after long sessions or context compactions.

### 8.6 Dangerous command blocker (blocking)

**Event.** PreToolUse on Bash.

**Command.** A script at `scripts/dev/hooks/block-dangerous.sh` that parses the command and blocks (exit 2) if it matches: `rm -rf` at root or under $HOME, `git push --force` to main, `git reset --hard` without confirmation, `sudo rm`, anything piping `curl` into `bash` from a non-allowlisted domain.

**Why.** Deterministic safety rail. No amount of prompting replaces this.

### 8.7 Hex colour guard (blocking)

**Event.** PostToolUse on Write and Edit in files under `packages/ui/src/components/` or `apps/web/src/`.

**Command.** A script at `scripts/dev/hooks/check-hex-colours.sh` that greps the file for hex colour patterns (`#[0-9a-fA-F]{3,8}`) and blocks if any are found, except in the tokens file. Prints a message telling Claude to add the colour to `packages/ui/src/tokens.ts` instead.

**Why.** Enforces the design system. CLAUDE.md says no hex colours in components; the hook makes it physical.

### 8.8 OpenAPI drift guard (non-blocking warning)

**Event.** PostToolUse on Write and Edit matching `apps/control-plane/internal/handlers/` or `apps/control-plane/api/openapi.yaml`.

**Command.** A script that checks whether `packages/api-client/src/generated.ts` has been regenerated more recently than the handler or spec file. If not, emits a warning: "The API client is stale. Run `pnpm --filter api-client generate`."

**Why.** Keeps the TypeScript client in sync without relying on memory.

## 9. Slash commands specification

Each slash command is a Markdown file in `.claude/commands/`. The file contains a short description and the prompt that runs when the command is invoked. Many commands simply delegate to a subagent or skill.

### 9.1 `/phase-start`

Reads `docs/progress.md` to find the next phase. Reads the relevant section of `docs/06-implementation-plan.md`. Presents a summary of the phase's objective, deliverables, and acceptance criteria. Asks the user to confirm before starting work.

### 9.2 `/phase-done`

Invokes the `phase-acceptance` skill to run acceptance tests for the current phase. On success, updates `docs/progress.md` and prepares a commit message in the format `Phase N: <description>`. Asks the user to approve the commit before executing.

### 9.3 `/review`

Delegates to the `code-reviewer` subagent. Passes it the current uncommitted diff. Returns the review to the main thread.

### 9.4 `/spec <question>`

Delegates to the `spec-consultant` subagent with the user's question. Returns the answer with spec references.

### 9.5 `/adr`

Invokes the `create-adr` skill to generate a new ADR from the template.

### 9.6 `/test [scope]`

Delegates to the `test-runner` subagent. Scope defaults to "all" and can be "unit", "integration", "e2e", or a file path.

### 9.7 `/explore <question>`

Delegates to the `architecture-explorer` subagent. Returns the architectural summary.

### 9.8 `/perf <question>`

Delegates to the `perf-engineer-expert` subagent. Used for k6 and load testing domain questions.

## 10. MCP servers

In v1 of the dev environment, no MCP servers are required. The project is self-contained and Claude Code has enough context from the spec documents and subagents. The `.claude/mcp.json` file is created empty (just `{}`) so that adding servers later is a one-line change.

If the user later wants GitHub integration for PR review, a Postgres MCP for cluster mode testing, or a documentation MCP for k6 and shadcn/ui, those are added at that time. They are explicitly not set up pre-emptively because each MCP server adds context weight whether or not it is used.

## 11. Supporting files

### 11.1 `docs/decisions/0000-template.md`

The ADR template. Short format: metadata (number, title, date, status, authors), context, decision, alternatives considered, consequences. One page maximum.

### 11.2 `docs/progress.md`

The phase tracking file. Starts with a header and a table with columns: phase, status, started, completed, notes. Each phase has a row. At the start of Phase 0, Claude Code pre-populates the rows with statuses "not started" for all phases. At the start and end of each phase, the row is updated.

### 11.3 `scripts/dev/`

All helper scripts invoked by hooks and skills. Each script is POSIX-compatible and has a Windows batch equivalent if needed (use Git Bash as the default shell; Claude Code's Windows native install uses Git Bash internally anyway). Scripts are documented in a `scripts/dev/README.md`.

### 11.4 `.vscode/` (optional, created if user uses VS Code)

Workspace settings file with recommended extensions (ESLint, Prettier, Go, Tailwind IntelliSense, Claude Code) and consistent formatter settings. A `tasks.json` that exposes common commands from CLAUDE.md as tasks.

### 11.5 `.editorconfig`

Sets indent style (spaces), indent size (2 for web, 4 for Go), end-of-line (LF), charset (utf-8), trim trailing whitespace, and insert final newline. This normalises files across editors and operating systems.

### 11.6 `.gitignore` and `.gitattributes`

Standard ignores for Node, Go, Rust, IDE files, OS files, and any build outputs specific to OpenSynapse. `.gitattributes` sets `text=auto eol=lf` to avoid CRLF issues on Windows.

## 12. Phase 0 setup sequence

This is the order in which Claude Code must create the environment before touching any product code. Each step produces something testable.

**Step 1.** Create the repository skeleton with the monorepo directory layout from `docs/06-implementation-plan.md` Phase 0, but do not implement anything yet.

**Step 2.** Create `CLAUDE.md` at the root.

**Step 3.** Create `.claude/settings.json` with all hooks configured but with the actual hook scripts still as no-ops. Verify that Claude Code loads the settings without error.

**Step 4.** Create the hook scripts under `scripts/dev/hooks/`. Test each one manually by running it against a sample input.

**Step 5.** Create the skills under `.claude/skills/`. Test each one by invoking its slash command or by prompting the conditions that should auto-invoke it.

**Step 6.** Create the subagents under `.claude/agents/`. Test each one by running its slash command with a sample question.

**Step 7.** Create the slash commands under `.claude/commands/`.

**Step 8.** Create supporting files: `docs/decisions/0000-template.md`, `docs/progress.md`, `.editorconfig`, `.gitignore`, `.gitattributes`, `.vscode/settings.json` and `extensions.json`.

**Step 9.** Commit everything as "Phase -1: development environment setup". The negative number makes it obvious this precedes Phase 0 of product work.

**Step 10.** Run `/phase-start` to begin Phase 0 of the actual implementation plan.

Only after Step 10 does product code start being written.

## 13. Validation checklist

Before declaring the dev environment ready, Claude Code must verify all of the following:

- `CLAUDE.md` is under 200 lines.
- Every skill in `.claude/skills/` has a valid SKILL.md with frontmatter that parses.
- Every subagent in `.claude/agents/` has a valid frontmatter that parses.
- Every hook in `.claude/settings.json` has a script that exists at the configured path and is executable.
- Running the secret scanner hook against a file containing a fake API key returns exit code 2.
- Running the hex colour guard against a file containing `#ff0000` returns exit code 2.
- Running the dangerous command blocker against `rm -rf /` returns exit code 2.
- Invoking `/spec "What is the live control architecture"` via the spec-consultant subagent returns a reference to `docs/02-architecture.md` section 3.
- Invoking `/explore "Where is the plan-to-script compiler"` via the architecture-explorer subagent returns a file path (even if the file does not exist yet; the agent should say so).
- Invoking `/test all` runs the current test command (which will report "no tests" at this stage) and returns structured output.
- `docs/progress.md` exists with all phases listed as "not started" except Phase -1 (dev environment) which is marked "complete".

If all eleven checks pass, the environment is ready. If any fail, Claude Code fixes them and reruns the validation before proceeding.
