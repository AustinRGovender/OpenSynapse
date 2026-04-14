---
name: create-adr
description: Create a new Architecture Decision Record from the template
tools:
  - Read
  - Write
  - Glob
---

# Creating an ADR

Use this when a decision is made that is not covered by the specs, or when the specs are ambiguous and a choice must be documented.

## Procedure

1. **Find the next number** by listing `docs/decisions/` and finding the highest-numbered file. The new ADR is that number + 1, zero-padded to 4 digits.

2. **Copy the template** from `docs/decisions/0000-template.md` to `docs/decisions/NNNN-{short-title}.md`.

3. **Fill in the metadata**:
   - Date: today's date (YYYY-MM-DD)
   - Status: `accepted` (or `proposed` if seeking feedback)
   - Authors: `Claude Code` (or include the user if they participated in the decision)

4. **Fill in the sections**:
   - **Context**: What situation or requirement led to this decision? Quote the relevant spec passage if applicable.
   - **Decision**: What was decided and why?
   - **Alternatives considered**: What other options were evaluated? Why were they rejected?
   - **Consequences**: What becomes easier or harder because of this decision?

5. Keep it to one page. ADRs are meant to be scannable.

## Reference

- Template: `docs/decisions/0000-template.md`
- ADR convention: `docs/07-dev-environment.md` section 11.1
