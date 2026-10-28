# API error codes

Every error from the OpenSynapse API has the same shape:

```json
{ "error": { "code": "VALIDATION_ERROR", "message": "worker_count must be between 1 and 64", "details": null } }
```

`code` is stable and safe to match on; `message` is written for people and may change.
`details` is present for some validation errors. The full API is described by
`GET /openapi.json`.

## Access and security

| Code | HTTP | Meaning |
| --- | --- | --- |
| `UNAUTHORIZED` | 401 | No valid token. Sign in, or send `Authorization: Bearer <token>` (the local install token, a session, or a personal API token). |
| `FORBIDDEN` | 403 | Your role can't do this (viewers can't change things; only admins manage users and settings), or a k6 run token was wrong. |
| `INVALID_CREDENTIALS` | 401 | Wrong email or password (or current password, when changing it). |
| `HOST_NOT_ALLOWED` | 403 | The request's Host isn't one the server answers to. Add it to `OPENSYNAPSE_ALLOWED_HOSTS`. |
| `ORIGIN_NOT_ALLOWED` | 403 | A browser page from another origin called the API. Add the origin to `OPENSYNAPSE_ALLOWED_ORIGINS`. |
| `NOT_TEAM_MODE` | 409 | Accounts, sign-in and personal tokens exist only with `TEAM_MODE=true`. |
| `ALREADY_SET_UP` | 409 | First-run setup was already done; sign in instead. |
| `EMAIL_TAKEN` | 409 | A user with that email exists. |
| `LAST_ADMIN` | 409 | The change would leave no admin. |
| `CANNOT_DELETE_SELF` | 409 | Admins can't delete their own account. |

## Requests and validation

| Code | HTTP | Meaning |
| --- | --- | --- |
| `INVALID_JSON` | 400 | The body isn't valid JSON for this endpoint. |
| `INVALID_BODY`, `INVALID_REQUEST` | 400 | The body or parameters are malformed. |
| `VALIDATION_ERROR` | 400 | A value is out of range or missing; the message says which. |
| `INVALID_FORMAT` | 400 | Unknown export format (use `json`, `csv`, `html` or `pdf`). |
| `INVALID_VERSION` | 400 | The plan version number isn't valid. |
| `INVALID_ENGINE` | 400 | Crawler engine must be `rod`, `colly` or `zap`. |
| `NO_FILE` | 400 | An import request had no file. |
| `IMPORT_FAILED` | 400 | The JMeter file couldn't be read. |
| `*_NOT_FOUND` | 404 | No such resource: `PLAN_`, `PLAN_VERSION_`, `RUN_`, `ENVIRONMENT_`, `REPORT_`, `COLLECTION_`, `CRAWL_`, `FRAGMENT_`, `USER_`, `TOKEN_` or `OAUTH_FLOW_NOT_FOUND`. |
| `CANNOT_MODIFY`, `CANNOT_DELETE_BUILTIN` | 403 | Built-in fragments can't be changed or deleted (built-in plans can be edited). |
| `INTERNAL_ERROR` | 500 | Something went wrong on the server; the server log has details. |

## Plans and runs

| Code | HTTP | Meaning |
| --- | --- | --- |
| `COMPILATION_FAILED`, `COMPILE_ERROR` | 422 | The plan can't be turned into a k6 script; the message names the node. |
| `ENGINE_NOT_AVAILABLE` | 503 | No k6 binary was found. Use the Docker image or set `OPENSYNAPSE_K6_PATH`. |
| `ENGINE_ERROR` | 500 | k6 (or, in cluster mode, the TestRun) couldn't be started. |
| `RUN_ACTIVE` | 409 | Stop the run before deleting it. |
| `RUN_NOT_ACTIVE` | 409 | Live control, stop or kill on a run that has finished. |
| `CONTROL_UNSUPPORTED` | 422 | The change isn't possible for this run, e.g. changing VUs of an arrival-rate scenario, or an RPS limit without xk6-kv. |
| `CONTROL_REJECTED` | 422 | k6 refused the change (the message gives k6's reason). |
| `PDF_UNAVAILABLE` | 503 | PDF export needs Chrome or Chromium on the server (bundled in the Docker image). |
| `PDF_FAILED` | 500 | Rendering the PDF failed. |

## Playground and crawler

| Code | HTTP | Meaning |
| --- | --- | --- |
| `REQUEST_FAILED` | 502 | The playground request couldn't reach the target (DNS, connection refused, TLS…). |
| `AUTH_ERROR` | 400 | The chosen auth method is missing something (e.g. an API key name, or no OAuth token yet). |
| `OAUTH_FAILED` | 502 | The OAuth provider refused the token request. |
| `MICROTEST_TIMEOUT` | 504 | The micro-test didn't finish in time. |
| `CRAWL_NOT_COMPLETE` | 400 | Generate a plan only after the crawl has completed. |

## AI analysis

| Code | HTTP | Meaning |
| --- | --- | --- |
| `AI_NOT_CONFIGURED`, `AI_NOT_ENABLED`, `AI_NO_KEY` | 400 | Set up a provider and key under Settings › AI and enable it. |
| `AI_INVALID_PROVIDER` | 400 | Provider must be `openai`, `anthropic`, `azure_openai` or `nvidia_nim`. |
| `AI_CAP_EXCEEDED` | 400 | This month's spending cap is reached. |
| `AI_RATE_LIMITED` | 429 | More than 20 analyses in an hour. |
| `AI_REQUEST_FAILED` | 502 | The AI provider returned an error. |
