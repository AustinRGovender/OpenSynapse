---
name: create-react-component
description: How to scaffold a new React component with Storybook entry, tests, and design-token-only colours
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
---

# Creating a React Component

## File layout

```
packages/ui/src/components/{ComponentName}/
  index.tsx              # Component implementation
  {ComponentName}.stories.tsx  # Storybook entries
  {ComponentName}.test.tsx     # Tests
```

For app-specific components (not shared), place them under `apps/web/src/components/` with the same structure.

## Component requirements

- TypeScript with explicit prop types (interface, not type alias)
- Accessible: ARIA attributes where relevant, keyboard handlers for interactive elements
- Design tokens only: use Tailwind classes from the project palette. **No hex colours.** If a colour is needed that doesn't exist, add it to `packages/ui/src/tokens.ts` first
- Use `forwardRef` for components that wrap native elements
- Follow the component style from `docs/05-ui-ux-spec.md`

## Storybook entry

Every component must have a `.stories.tsx` file with at minimum:
- `Default` story — component with required props only
- `AllProps` story — component with all props set to non-default values
- Additional stories for interactive states (hover, focus, disabled) where relevant

Use the `@storybook/react` CSF3 format.

## Test

Every component must have a `.test.tsx` file that covers:
- Renders without crashing
- One interaction test (click, input, etc.) if the component is interactive
- Accessibility: no violation from a basic axe-core check if applicable

Use Vitest + Testing Library.

## Reference

- Design language: `docs/05-ui-ux-spec.md`
- Colour tokens: `packages/ui/src/tokens.ts`
- Typography and spacing: `docs/05-ui-ux-spec.md` sections 3–4
