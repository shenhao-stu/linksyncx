# Telemetry and diagnostic privacy

The gateway acknowledges and discards `POST /api/event_logging/batch` and
`POST /api/event_logging/v2/batch`. These handlers return an empty HTTP 200 with
`Cache-Control: no-store`. Application handlers do not read, parse, persist, bind
to an account, or forward the payload. These exact method/path pairs also bypass
application access logging and request-ID collection. Unknown paths and other
methods retain normal routing and logging. The HTTP server may drain a discarded
body as part of connection management; this is not application processing.

This prevents a relay from attaching one caller's telemetry to a different
upstream account. No synthetic events, identity rewrites, periodic telemetry
worker, model initialization calls, or randomized process metrics are added.
Inference forwarding and its existing native-field preservation are unchanged.

## Structural diagnostics

Claude request diagnostics and the opt-in `SUB2API_DEBUG_GATEWAY_BODY` file now
share one pure summary function. They record byte/header counts, JSON validity,
the presence of known headers and selected fields, and session consistency.
Session consistency is `match`, `mismatch`, `ambiguous` (multiple header values),
or `unavailable`. No session ID is emitted.

Raw header values, arbitrary header/JSON field names, credentials, cookies,
URLs, account names, metadata identities, prompts and message content are not
included in these summaries. Unknown fields therefore require no new redaction
rule. The summary does not change the headers or body being forwarded. Normal
error diagnostics retain the internal numeric account ID for troubleshooting.

The existing debug environment variable remains opt-in, but now writes summaries
instead of request dumps. New directories use mode 0700; new and existing files
are restricted to 0600 before logging is enabled. Existing file contents are not
erased or retroactively sanitized. This change is scoped to these diagnostic
paths; it is not a claim that every product audit log is content-free.

## Client and infrastructure boundaries

Generated Claude Code setup instructions already set
`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`. The official client also documents
`DISABLE_TELEMETRY=1` and `DISABLE_ERROR_REPORTING=1`. The broader setting can affect
feature-flag-dependent functionality; it is not a switch for all networking.
See the official [data usage](https://code.claude.com/docs/en/data-usage) and
[environment variables](https://code.claude.com/docs/en/env-vars) documentation.

The gateway cannot control requests a client sends directly to another host.
Edge/reverse-proxy logs are separate from application access logs. A disabled
optional telemetry setting neither proves full client equivalence nor guarantees
any account outcome. There is no verified fixed catalog of 200 signals here.

Tests use synthetic values and in-process HTTP requests. They verify unknown-field
non-disclosure, request immutability, ambiguous session headers, private file
permissions, unread sink payloads and exact routing/logging boundaries. They do
not contact a model provider, refresh a subscription or probe a proxy node.
