# Outbound data policy

U Workers stores business data locally by default. Models and integrations run
according to the user's configuration and existing authorization; their requests
can include task context. This is not an offline product or a promise that all
data stays on the machine.

## Boundary

- Product analytics, signup attribution, crash uploads, device/version reports,
  self-host telemetry and marketing requests are disabled in code. Old environment
  variables cannot turn them back on. Local logs and diagnostic exports remain.
- Remove official Multica cloud control-plane, login, billing, storage and update
  fallbacks. Missing configuration must not choose an upstream operational account.
- Model authentication, streaming, SDK retries and recovery belong to the model
  request. Do not add domain allowlists, force local models, block subprocess
  networking or require consent for each request.
- Task tools, MCP, Git, downloads and explicit links retain existing permissions.
  Clicking Run does not authorize telemetry or unrelated uploads.
- Configured OAuth, email, channels, Composio and external storage remain available.
  Without configuration, do not start probes or workers. Authorized subscriptions
  and scheduled work can continue in the background within their stored scope and
  lifecycle. Revocation stops that work.
- Explicit update/install actions use a real user-selected source or local package.
  Startup and timers must not discover or download releases. Detecting a binary
  that the user has already replaced on disk is a local operation.
- Fonts, product assets and diagnostics are local. Remote images, embeds and
  previews require an explicit loading action or an authorized integration.

Attribute requests to authenticated sessions, authorized task/run records,
persisted integration configuration and subscriptions. An arbitrary
`userInitiated` request flag is not evidence of permission. Keep existing
authentication, workspace authorization and external-link validation.

## Request inventory and ownership

| Entry point / trigger | Destination and possible data | Authorization / treatment |
| --- | --- | --- |
| `packages/core/analytics`, login/page/error callbacks | Product collector; identity, navigation, exceptions | No product reporting; compile-time gates and inert caller API |
| `server/internal/analytics`, handler events | Product collector; account/workspace/task metadata | No collector transport or event buffering |
| `server/internal/selfhosttelemetry`, startup/daily worker | Upstream telemetry; installation fingerprint and counts | No production worker assembly; reporting gate remains false |
| `server/cmd/server/router.go`, cloudruntime and cloud billing | Official cloud; PATs, fleet/workspace and billing state | No official cloud clients, workers or cloud API assembly |
| `server/pkg/llm`, chat title and follow-up generation | User-configured model; first chat message or bounded conversation tail | Explicit assist-layer configuration; no key/base URL means no request; configured SDK streaming/retries retained |
| `server/pkg/agent`, daemon task execution | Selected runtime/model and task tool destinations; task context | Existing task/run authorization; no new global network policy |
| Composio, Git and channel workers | Configured provider; selected tools, messages or repository data | Existing credentials, workspace permissions and enabled integration lifecycle |
| Storage adapters | Local files by default; configured S3 object data | External storage is opt-in; no implicit official bucket |
| Go daemon update hooks and CLI update | User-selected release source; release metadata and requested package | Explicit CLI/RPC action only; no automatic check/download or official release fallback |
| Web/Desktop config, auth, resources and updater | Self-host endpoint, configured authentication or selected external resource | Frontend companion work removes upstream defaults and automatic requests; does not change this policy |

This inventory covers the plan's known entry points. New outbound paths must
document their trigger, destination, data and existing authorization before they
are wired into production. Removing a cloud feature must not disable unrelated
user-configured integrations.

## Regression evidence

Use fake transports, test-created executables and local fake servers. Never use
real model accounts, agent sessions, email recipients, installers or deployment
databases for ordinary regression tests.

- Core analytics privacy tests exercise configured keys and old environment
  variables, cookie cleanup, fetch/XHR/beacon silence and unchanged user networking.
- Go privacy/analytics tests keep reporting disabled regardless of environment.
- `server/pkg/llm/privacy_network_test.go` runs the real SDK against a fake
  non-loopback destination: a retryable response followed by a successful stream.
  Existing LLM tests cover unconfigured zero requests and explicit retry budgets.
- Server assembly, configured integration/storage and manual update tests own
  their respective boundaries. Zero requests is required for unsolicited
  collectors, not for models or authorized work.
- The separate Task 8 acceptance issue observes browser, Node/Go and Electron
  traffic in an isolated deployment. Unit tests do not replace that observation,
  real-agent authorization or platform-specific release validation.

Retain only necessary destination, trigger and result metadata in verification
records; never capture tokens, task bodies, repository contents or production
traffic. Original deployments, credentials, databases and runtime profiles stay
untouched.
