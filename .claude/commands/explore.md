# /explore

Explore the codebase architecture.

Usage: `/explore <question>`

Delegate to the `architecture-explorer` subagent with the question. It will:
1. Read `CLAUDE.md` and relevant docs for orientation.
2. Search the codebase using Glob and Grep.
3. Read relevant code sections and follow imports.
4. Return a concise report with file:line references and an architectural summary.

Present the findings to the user.
