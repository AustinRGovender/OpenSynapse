# ADR-0013: NVIDIA NIM as a fourth AI analysis provider

**Date:** 2026-10-28
**Status:** accepted
**Authors:** Claude Code, with the project owner

## Context

The specs name three AI providers for v1: "OpenAI (chat completions), Anthropic
(messages), Azure OpenAI (chat completions with deployment routing). The
provider abstraction is a Go interface with one implementation per provider.
Adding a fourth is a file, not a refactor." (`docs/02-architecture.md`). The
project owner asked for NVIDIA NIM, whose hosted API catalog (`integrate.api.nvidia.com/v1`)
serves open models (GLM, Nemotron, DeepSeek, gpt-oss and others) behind an
OpenAI-compatible chat completions API with a Bearer key (`nvapi-…`).

## Decision

Add an `nvidia_nim` provider (`ai.NIMProvider`) that posts to
`{base}/chat/completions` with `Authorization: Bearer <key>`, where `base`
defaults to NVIDIA's hosted API. Models are catalog IDs such as
`z-ai/glm-5.3-flash`, the default in Settings.

OpenAI, Azure OpenAI and NIM now share one chat completions helper instead of
three copies. Two behaviours change for all three:

- An empty answer is an error (`… returned no answer (finish reason: length)`)
  rather than a cached blank analysis. Reasoning models on NIM can spend the
  whole token budget before answering.
- The provider HTTP timeout is two minutes instead of 30 seconds.

The same change fixes Azure OpenAI: `GetProvider` never passed the saved
endpoint to the provider, so every Azure call failed.

The API is unchanged: `provider` is already a free string, and no new
configuration field is needed.

## Alternatives considered

- **A generic "OpenAI-compatible" provider with a base URL.** It would cover
  NIM, self-hosted NIM, vLLM and Ollama in one go, but needs a new config
  field and migration, and the spec's model is one named provider per
  service. Left for later; `NIMProvider.BaseURL` is the hook for self-hosted
  NIM.
- **Reusing the OpenAI provider with a different URL.** Would show NIM as
  "OpenAI" in the prompt preview ("This exact text will be sent to …"), which
  must name where the data goes.

## Consequences

- Spend estimates for NIM models use the default rate ($5 / $15 per million
  tokens). NVIDIA's hosted catalog bills by credits, not per token, so the
  estimate is deliberately high, and the monthly cap trips early rather than
  never.
- Not yet exercised against the live NVIDIA API; tests use a local fake.
- `docs/02`, `docs/03` (F-09) and `docs/04` §1.8 still list three providers.
