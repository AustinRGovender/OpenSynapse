---
name: add-plan-node-type
description: How to add a new test plan node type to the schema, compiler, and plan builder UI
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Adding a Plan Node Type

Five files must be touched when adding a new node type (e.g., a new controller or sampler).

## Step 1: JSON Schema

Create `packages/plan-schema/schemas/nodes/{type}.schema.json` with the node's properties schema. Follow existing schemas for the format. Include `$id`, `type`, `properties`, and `required` fields.

## Step 2: TypeScript type

Add the corresponding TypeScript type in `packages/plan-schema/src/types.ts`. Export it and add it to the `NodeProperties` union type.

## Step 3: Transformer case

Add a case in `apps/control-plane/internal/compiler/compiler.go` that emits the k6 JavaScript for the new node type. Follow the pattern of existing cases:
- HTTP request nodes emit `http.get()` / `http.post()` etc.
- Controllers emit `if`/`for`/`group()` blocks
- Timers emit `sleep()` calls

If the node type cannot be fully expressed in k6 yet, emit a `// TODO: {type}` comment.

## Step 4: React components

Create two components in `apps/web/src/components/plan-builder/`:
- A form component for the properties panel (right pane)
- A React Flow node component for the canvas (centre pane)

Use design tokens only (no hex colours). Add Storybook entries for both.

## Step 5: Tests

- Unit test for the JSON schema validation (valid and invalid inputs)
- Round-trip test: plan → script → plan preserves the node's semantics
- React component render test

## Step 6: Register in palette

Add the new node type to `apps/web/src/config/nodes.ts` so it appears in the builder's add-node palette.

## Reference

- Feature spec: `docs/03-feature-spec.md` section F-02
- Data model: `docs/04-data-model-and-api.md` section 1.1
