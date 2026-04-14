# /adr

Create a new Architecture Decision Record.

Usage: `/adr [title]`

Invoke the `create-adr` skill to:
1. Find the next available ADR number in `docs/decisions/`.
2. Create a new file from the template at `docs/decisions/0000-template.md`.
3. Fill in the date and status.
4. If a title was provided, use it. Otherwise, ask the user for a short title.
5. Open the file for the user to fill in Context, Decision, Alternatives, and Consequences.
