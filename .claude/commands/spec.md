# /spec

Look up what the OpenSynapse specifications say about a topic.

Usage: `/spec <question>`

Delegate to the `spec-consultant` subagent with the user's question. The subagent will:
1. Search the spec documents (`docs/01-PRD.md` through `docs/07-dev-environment.md`).
2. Find relevant passages and quote them.
3. Return the answer with file and section references.
4. Flag if the specs are silent or ambiguous (recommend an ADR in that case).

Present the answer to the user.
