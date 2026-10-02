# Client version settings

Open `/admin/settings` and select **Client Versions / 版本管理**. Each of Grok CLI,
Claude Code CLI, and Claude TypeScript SDK has its own switch and custom pin.
All three automatic synchronization switches default to **off**. Fields are
populated from the running release's built-in pins, including any supported
existing custom/environment override. Click **Save version settings** to persist;
ordinary settings saves do not overwrite these separately managed choices.

| Client | Built-in pin in upstream 0.2.9.2 | Official stable metadata |
| --- | --- | --- |
| Grok CLI | 1.0.46 | https://x.ai/cli/stable |
| Claude Code CLI | 2.1.287 | https://api.github.com/repos/anthropics/claude-code/releases/latest |
| Claude TypeScript SDK | 0.127.0 | https://registry.npmjs.org/@anthropic-ai/sdk/latest |

SDK means `@anthropic-ai/sdk`, not the Agent SDK. Its GitHub monorepo's latest
release may refer to another package. Package-name validation prevents that
unrelated version from becoming the TypeScript SDK version.

Off selects the custom pin; absent/unsupported pins fall back to a supported
legacy environment override and then the built-in value. On selects the last
valid official stable release, falling back to the custom pin until a check
succeeds. Turning sync off preserves its history but stops using it. Resetting a
card selects its built-in pin and turns its switch off, pending an explicit save.
The UI shows built-in, minimum, effective version/source, last valid release,
last check time and failures. No official lookup happens merely by opening it.

An enabled client is checked within one minute, at most once per hour thereafter.
Check times, including errors and unchanged versions, survive restarts. Requests
use fixed HTTPS metadata URLs, no account credentials, a 10-second HTTP timeout,
a 64 KiB response limit, and no redirects. Unknown/draft/prerelease/malformed
responses and regressions retain the last valid release. Missing switches and
DB read failures never authorize a fetch. Disabling while a fetch is running
discards its result. Shutdown cancels in-flight checks. This cadence is per
service instance; the deployment uses one API replica.

Version settings affect gateway-generated headers and the OAuth SDK helper.
Grok uses one resolver for service and final transport headers. Claude SDK
runtime settings cover defaults and newly persisted identities explicitly marked
as originating from gateway defaults. Client-supplied SDK values and legacy
identity records retain their values. Claude's existing account CLI floor remains
monotonic, and native passthrough keeps its existing preservation policy. Changing
these settings does not install a CLI/SDK, change TLS behavior or reconstruct
telemetry. Independently synchronized latest releases do not establish that a
particular Claude binary bundles that SDK version.
